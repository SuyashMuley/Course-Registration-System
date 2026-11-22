package strategy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Naive is the failure baseline for E1. It reads the seat count, checks it in Go, and
// then writes, as separate statements with no transaction and no locking. Two requests
// can both read "1 seat left" and both succeed, so it is EXPECTED to over-enroll under
// contention. Do not use it for anything except the experiment.
//
// For E1 to show the over-enrollment, run it against a schema WITHOUT the
// enrolled_within_capacity constraint (see migrations/001_init.sql).
type Naive struct{ pool *pgxpool.Pool }

func NewNaive(pool *pgxpool.Pool) *Naive { return &Naive{pool: pool} }

func (n *Naive) Name() string { return "naive" }

func (n *Naive) Register(ctx context.Context, studentID, sectionID int64) error {
	var capacity, enrolled int
	err := n.pool.QueryRow(ctx,
		`SELECT capacity, enrolled FROM sections WHERE id = $1`, sectionID,
	).Scan(&capacity, &enrolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if enrolled >= capacity { // check ...
		return ErrSectionFull
	}
	// ... then act: another request can slip in between the check and these writes.
	if _, err := n.pool.Exec(ctx,
		`INSERT INTO enrollments (student_id, section_id) VALUES ($1, $2)`, studentID, sectionID,
	); err != nil {
		return mapPgError(err)
	}
	_, err = n.pool.Exec(ctx,
		`UPDATE sections SET enrolled = enrolled + 1 WHERE id = $1`, sectionID)
	return err
}

func (n *Naive) Drop(ctx context.Context, studentID, sectionID int64) error {
	tag, err := n.pool.Exec(ctx,
		`DELETE FROM enrollments WHERE student_id = $1 AND section_id = $2`, studentID, sectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotEnrolled
	}
	_, err = n.pool.Exec(ctx,
		`UPDATE sections SET enrolled = enrolled - 1 WHERE id = $1`, sectionID)
	return err
}

// mapPgError turns PostgreSQL constraint violations into the package's errors.
func mapPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation: (student_id, section_id) already present
			return ErrAlreadyEnrolled
		case "23503": // foreign_key_violation: unknown student or section
			return ErrNotFound
		}
	}
	return err
}
