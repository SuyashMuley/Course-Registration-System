// Package strategy defines how a seat is claimed. Each strategy lives in its own file
// so the experiments (E1, E3) can compare them without touching shared code.
//
// Owner: Suyash.
package strategy

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrSectionFull means no seats are left.
	ErrSectionFull = errors.New("section full")
	// ErrAlreadyEnrolled means the student already holds a seat.
	ErrAlreadyEnrolled = errors.New("already enrolled")
	// ErrNotEnrolled means a drop was requested for a seat the student does not hold.
	ErrNotEnrolled = errors.New("not enrolled")
	// ErrNotFound means the student or section does not exist.
	ErrNotFound = errors.New("student or section not found")
)

// SeatStrategy claims or releases a seat for a student in a section.
// Implemented (one file each):
//   - naive.go    read-then-write, no concurrency control (expected to over-enroll)
//   - atomic.go   one SQL statement: UPDATE ... WHERE enrolled < capacity
//
// Planned:
//   - locking.go  SELECT ... FOR UPDATE
//   - redis.go    Redis admission filter in front of the atomic update (Redis may only say no)
type SeatStrategy interface {
	Name() string
	Register(ctx context.Context, studentID, sectionID int64) error
	Drop(ctx context.Context, studentID, sectionID int64) error
}

// New returns the strategy with the given name ("naive" or "atomic").
func New(name string, pool *pgxpool.Pool) (SeatStrategy, error) {
	switch name {
	case "naive":
		return NewNaive(pool), nil
	case "atomic":
		return NewAtomic(pool), nil
	default:
		return nil, fmt.Errorf("unknown strategy %q (want naive or atomic)", name)
	}
}
