// Package waitlist implements the capped, ordered waitlist for full sections.
//
// Owner: Sameep.
//
// Rules (checked by the integration tests and, later, the invariant checker):
//   - A student can join only when the section is full; if seats are open they register.
//   - At most waitlist_cap students wait per section.
//   - A student is never both enrolled and waitlisted in the same section.
//   - Promotion is first come, first served: order of joined_at, then id.
//
// Concurrency: every operation that can change who is enrolled or how long the list is
// (Join, PromoteNext) first locks the section row with SELECT ... FOR UPDATE. Seat claims
// in the atomic strategy update the same row, so they are serialized with joins and
// promotions for that section. Leave only deletes its own row and needs no section lock.
package waitlist

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrNotFound means the student or section does not exist.
	ErrNotFound = errors.New("student or section not found")
	// ErrSeatsAvailable means the section still has open seats; register instead.
	ErrSeatsAvailable = errors.New("section has open seats; register instead")
	// ErrAlreadyEnrolled means the student already holds a seat in the section.
	ErrAlreadyEnrolled = errors.New("already enrolled")
	// ErrAlreadyWaitlisted means the student is already on this section's waitlist.
	ErrAlreadyWaitlisted = errors.New("already waitlisted")
	// ErrWaitlistFull means the waitlist has reached waitlist_cap.
	ErrWaitlistFull = errors.New("waitlist full")
	// ErrNotWaitlisted means the student is not on this section's waitlist.
	ErrNotWaitlisted = errors.New("not waitlisted")
)

// Info is the waitlist summary for one section.
type Info struct {
	SectionID int64 `json:"section_id"`
	Length    int   `json:"length"`
	Cap       int   `json:"cap"`
}

// Store runs waitlist operations against Postgres.
type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Join puts the student at the back of the section's waitlist and returns their
// position (1 = next to be promoted).
func (s *Store) Join(ctx context.Context, studentID, sectionID int64) (int, error) {
	var position int
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var capacity, enrolled, waitCap int
		err := tx.QueryRow(ctx,
			`SELECT capacity, enrolled, waitlist_cap FROM sections WHERE id = $1 FOR UPDATE`,
			sectionID,
		).Scan(&capacity, &enrolled, &waitCap)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}

		var isEnrolled, isWaitlisted bool
		var length int
		err = tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM enrollments WHERE student_id = $1 AND section_id = $2),
			       EXISTS (SELECT 1 FROM waitlist    WHERE student_id = $1 AND section_id = $2),
			       (SELECT count(*) FROM waitlist WHERE section_id = $2)`,
			studentID, sectionID,
		).Scan(&isEnrolled, &isWaitlisted, &length)
		if err != nil {
			return err
		}
		switch {
		case isEnrolled:
			return ErrAlreadyEnrolled
		case isWaitlisted:
			return ErrAlreadyWaitlisted
		case enrolled < capacity:
			return ErrSeatsAvailable
		case length >= waitCap:
			return ErrWaitlistFull
		}

		// clock_timestamp(), not now(): now() is the transaction start time, which can be
		// earlier than that of a join that got the section lock first.
		_, err = tx.Exec(ctx,
			`INSERT INTO waitlist (section_id, student_id, joined_at) VALUES ($1, $2, clock_timestamp())`,
			sectionID, studentID)
		if err != nil {
			return mapPgError(err)
		}
		position = length + 1
		return nil
	})
	if err != nil {
		return 0, err
	}
	return position, nil
}

// Leave removes the student from the section's waitlist.
func (s *Store) Leave(ctx context.Context, studentID, sectionID int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM waitlist WHERE student_id = $1 AND section_id = $2`, studentID, sectionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotWaitlisted
	}
	return nil
}

const positionSQL = `
SELECT count(*)
  FROM waitlist w
  JOIN waitlist me ON me.section_id = w.section_id
 WHERE me.section_id = $2 AND me.student_id = $1
   AND (w.joined_at, w.id) <= (me.joined_at, me.id)`

// Position returns the student's place in line (1 = next to be promoted).
func (s *Store) Position(ctx context.Context, studentID, sectionID int64) (int, error) {
	var pos int
	if err := s.pool.QueryRow(ctx, positionSQL, studentID, sectionID).Scan(&pos); err != nil {
		return 0, err
	}
	if pos == 0 {
		return 0, ErrNotWaitlisted
	}
	return pos, nil
}

// Info returns the waitlist length and cap for a section.
func (s *Store) Info(ctx context.Context, sectionID int64) (Info, error) {
	info := Info{SectionID: sectionID}
	err := s.pool.QueryRow(ctx, `
		SELECT waitlist_cap, (SELECT count(*) FROM waitlist WHERE section_id = $1)
		  FROM sections WHERE id = $1`, sectionID,
	).Scan(&info.Cap, &info.Length)
	if errors.Is(err, pgx.ErrNoRows) {
		return Info{}, ErrNotFound
	}
	if err != nil {
		return Info{}, err
	}
	return info, nil
}

// PromoteNext fills open seats in the section from the front of its waitlist, inside the
// caller's transaction. Seat strategies call it from Drop, after releasing the seat, so
// the drop and the promotion commit or roll back together. It returns the promoted
// student IDs in promotion order (usually zero or one).
func PromoteNext(ctx context.Context, tx pgx.Tx, sectionID int64) ([]int64, error) {
	var promoted []int64
	for {
		// Lock the section and check for room. Holding this lock until commit keeps joins
		// and other promotions for the section out until we are done.
		var room bool
		err := tx.QueryRow(ctx,
			`SELECT enrolled < capacity FROM sections WHERE id = $1 FOR UPDATE`, sectionID,
		).Scan(&room)
		if errors.Is(err, pgx.ErrNoRows) {
			return promoted, ErrNotFound
		}
		if err != nil || !room {
			return promoted, err
		}

		var rowID, studentID int64
		err = tx.QueryRow(ctx, `
			SELECT id, student_id FROM waitlist
			 WHERE section_id = $1
			 ORDER BY joined_at, id
			 LIMIT 1`, sectionID,
		).Scan(&rowID, &studentID)
		if errors.Is(err, pgx.ErrNoRows) {
			return promoted, nil // nobody waiting
		}
		if err != nil {
			return promoted, err
		}

		// A concurrent Leave may delete this row first; then look at the new head.
		tag, err := tx.Exec(ctx, `DELETE FROM waitlist WHERE id = $1`, rowID)
		if err != nil {
			return promoted, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}

		// Should never conflict (Join refuses enrolled students), but if it does the
		// student already has a seat: drop the stale waitlist entry and move on.
		tag, err = tx.Exec(ctx,
			`INSERT INTO enrollments (student_id, section_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			studentID, sectionID)
		if err != nil {
			return promoted, err
		}
		if tag.RowsAffected() == 0 {
			continue
		}
		if _, err := tx.Exec(ctx,
			`UPDATE sections SET enrolled = enrolled + 1 WHERE id = $1`, sectionID); err != nil {
			return promoted, err
		}
		promoted = append(promoted, studentID)
	}
}

func mapPgError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique (section_id, student_id)
			return ErrAlreadyWaitlisted
		case "23503": // unknown student
			return ErrNotFound
		}
	}
	return err
}
