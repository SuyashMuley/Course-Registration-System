package strategy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Atomic claims a seat with ONE SQL statement. The conditional UPDATE only succeeds
// while enrolled < capacity, and the INSERT happens in the same statement, so there is
// no window between "check" and "act". If the INSERT fails (the student is already
// enrolled) the whole statement rolls back, including the seat increment.
//
// The row lock is held only for the duration of this one statement (about 1 ms), which
// is also why it is safe to run behind a forwarded request in a second region.
type Atomic struct{ pool *pgxpool.Pool }

func NewAtomic(pool *pgxpool.Pool) *Atomic { return &Atomic{pool: pool} }

func (a *Atomic) Name() string { return "atomic" }

const registerSQL = `
WITH claimed AS (
    UPDATE sections
       SET enrolled = enrolled + 1
     WHERE id = $2 AND enrolled < capacity
 RETURNING id
)
INSERT INTO enrollments (student_id, section_id)
SELECT $1, id FROM claimed`

func (a *Atomic) Register(ctx context.Context, studentID, sectionID int64) error {
	tag, err := a.pool.Exec(ctx, registerSQL, studentID, sectionID)
	if err != nil {
		return mapPgError(err)
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	// Nothing was claimed: either the section is full or it does not exist.
	var one int
	err = a.pool.QueryRow(ctx, `SELECT 1 FROM sections WHERE id = $1`, sectionID).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrSectionFull
}

const dropSQL = `
WITH removed AS (
    DELETE FROM enrollments
     WHERE student_id = $1 AND section_id = $2
 RETURNING section_id
)
UPDATE sections
   SET enrolled = enrolled - 1
 WHERE id IN (SELECT section_id FROM removed)`

func (a *Atomic) Drop(ctx context.Context, studentID, sectionID int64) error {
	tag, err := a.pool.Exec(ctx, dropSQL, studentID, sectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotEnrolled
	}
	return nil
}
