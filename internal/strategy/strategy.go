// Package strategy defines how a seat is claimed. Each strategy lives in its own file
// so the experiments (E1, E3) can compare them without touching shared code.
//
// Owner: Suyash.
package strategy

import (
	"context"
	"errors"
)

var (
	// ErrSectionFull means no seats are left.
	ErrSectionFull = errors.New("section full")
	// ErrAlreadyEnrolled means the student already holds a seat.
	ErrAlreadyEnrolled = errors.New("already enrolled")
)

// SeatStrategy claims or releases a seat for a student in a section.
// Implementations planned (one file each):
//   - naive.go    read-then-write, no concurrency control (expected to over-enroll)
//   - locking.go  SELECT ... FOR UPDATE
//   - atomic.go   UPDATE ... SET enrolled = enrolled + 1 WHERE enrolled < capacity
//   - redis.go    Redis atomic admission in front of Postgres
type SeatStrategy interface {
	Name() string
	Register(ctx context.Context, studentID, sectionID int64) error
	Drop(ctx context.Context, studentID, sectionID int64) error
}
