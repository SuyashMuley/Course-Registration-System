package strategy_test

// Integration tests against a real Postgres. They are skipped unless TEST_DATABASE_URL
// is set, e.g.
//
//	docker run -d --name pgtest -e POSTGRES_PASSWORD=pw -p 55432:5432 postgres:16
//	TEST_DATABASE_URL=postgres://postgres:pw@localhost:55432/postgres go test ./internal/strategy
//
// The tests create and drop their own tables in a throwaway schema, so use a scratch
// database, not a shared one.

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/SuyashMuley/Course-Registration-System/internal/db"
	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	capacity = 10
	students = 200
)

func setup(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	pool, err := db.Open(ctx, db.Config{URL: url, MaxConns: 50})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	// Same shape as migrations/001_init.sql, without the enrolled<=capacity constraint
	// (the E1 schema), so the naive strategy is allowed to over-enroll.
	stmts := []string{
		`DROP TABLE IF EXISTS enrollments, sections, students CASCADE`,
		`CREATE TABLE students (id BIGINT PRIMARY KEY)`,
		`CREATE TABLE sections (id BIGINT PRIMARY KEY, capacity INT NOT NULL, enrolled INT NOT NULL DEFAULT 0)`,
		`CREATE TABLE enrollments (
			student_id BIGINT NOT NULL REFERENCES students(id),
			section_id BIGINT NOT NULL REFERENCES sections(id),
			PRIMARY KEY (student_id, section_id))`,
		`INSERT INTO sections (id, capacity) VALUES (1, 10)`,
		`INSERT INTO students SELECT generate_series(1, 200)`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP TABLE IF EXISTS enrollments, sections, students CASCADE`)
	})
	return pool, ctx
}

// hammer registers `students` different students for section 1 at the same time.
func hammer(ctx context.Context, s strategy.SeatStrategy) (ok, full int64) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for id := int64(1); id <= students; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			switch err := s.Register(ctx, id, 1); {
			case err == nil:
				atomic.AddInt64(&ok, 1)
			case errors.Is(err, strategy.ErrSectionFull):
				atomic.AddInt64(&full, 1)
			}
		}(id)
	}
	close(start)
	wg.Wait()
	return ok, full
}

func counts(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (enrolledCol, enrollmentRows int) {
	t.Helper()
	if err := pool.QueryRow(ctx, `SELECT enrolled FROM sections WHERE id = 1`).Scan(&enrolledCol); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM enrollments WHERE section_id = 1`).Scan(&enrollmentRows); err != nil {
		t.Fatal(err)
	}
	return
}

// The atomic strategy must never exceed capacity, no matter how many requests race.
func TestAtomicNeverOverEnrolls(t *testing.T) {
	pool, ctx := setup(t)
	ok, _ := hammer(ctx, strategy.NewAtomic(pool))
	col, rows := counts(t, ctx, pool)
	t.Logf("atomic: %d succeeded, enrolled=%d, enrollment rows=%d (capacity %d)", ok, col, rows, capacity)
	if col != capacity || rows != capacity || ok != capacity {
		t.Fatalf("want exactly %d enrolled; got succeeded=%d enrolled=%d rows=%d", capacity, ok, col, rows)
	}
}

// The naive strategy is the failure baseline: this test documents (and logs) the
// over-enrollment E1 expects. It does not fail if the race is not hit on a given run,
// because a lucky run proves nothing either way.
func TestNaiveOverEnrollsUnderContention(t *testing.T) {
	pool, ctx := setup(t)
	ok, _ := hammer(ctx, strategy.NewNaive(pool))
	col, rows := counts(t, ctx, pool)
	t.Logf("naive: %d succeeded, enrolled=%d, enrollment rows=%d (capacity %d)", ok, col, rows, capacity)
	if rows > capacity {
		t.Logf("over-enrolled by %d (expected for the naive strategy)", rows-capacity)
	}
}

func TestAtomicDuplicateAndDrop(t *testing.T) {
	pool, ctx := setup(t)
	s := strategy.NewAtomic(pool)

	if err := s.Register(ctx, 1, 1); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := s.Register(ctx, 1, 1); !errors.Is(err, strategy.ErrAlreadyEnrolled) {
		t.Fatalf("duplicate register = %v, want ErrAlreadyEnrolled", err)
	}
	if col, _ := counts(t, ctx, pool); col != 1 { // the failed duplicate must not leak a seat
		t.Fatalf("enrolled = %d after duplicate, want 1", col)
	}
	if err := s.Register(ctx, 1, 999); !errors.Is(err, strategy.ErrNotFound) {
		t.Fatalf("unknown section = %v, want ErrNotFound", err)
	}
	if err := s.Register(ctx, 99999, 1); !errors.Is(err, strategy.ErrNotFound) {
		t.Fatalf("unknown student = %v, want ErrNotFound", err)
	}
	if err := s.Drop(ctx, 1, 1); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := s.Drop(ctx, 1, 1); !errors.Is(err, strategy.ErrNotEnrolled) {
		t.Fatalf("second drop = %v, want ErrNotEnrolled", err)
	}
	if col, rows := counts(t, ctx, pool); col != 0 || rows != 0 {
		t.Fatalf("after drop enrolled=%d rows=%d, want 0/0", col, rows)
	}
}
