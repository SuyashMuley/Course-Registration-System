package waitlist_test

// Integration tests against a real Postgres. Skipped unless TEST_DATABASE_URL is set:
//
//	docker run -d --name pgtest -e POSTGRES_PASSWORD=pw -p 55432:5432 postgres:16
//	TEST_DATABASE_URL=postgres://postgres:pw@localhost:55432/postgres go test ./internal/waitlist
//
// The tests use their own schema (waitlist_test), created from migrations/001_init.sql
// plus the enrolled <= capacity constraint, so they can run alongside the strategy tests.

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/SuyashMuley/Course-Registration-System/internal/db"
	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/SuyashMuley/Course-Registration-System/internal/waitlist"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schema = "waitlist_test"

// setup creates a fresh schema with `students` students and one section (id 1).
func setup(t *testing.T, capacity, waitCap, students int) (*pgxpool.Pool, context.Context) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()

	admin, err := db.Open(ctx, db.Config{URL: url, MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = admin.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema)
	admin.Close()
	if err != nil {
		t.Fatal(err)
	}

	sep := "?"
	if strings.Contains(url, "?") {
		sep = "&"
	}
	pool, err := db.Open(ctx, db.Config{URL: url + sep + "search_path=" + schema, MaxConns: 50})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})

	ddl, err := os.ReadFile("../../migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		string(ddl),
		`ALTER TABLE sections ADD CONSTRAINT enrolled_within_capacity CHECK (enrolled <= capacity)`,
	} {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("schema: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO students SELECT generate_series(1, $1::int)`, students); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO sections (id, capacity, waitlist_cap) VALUES (1, $1, $2)`, capacity, waitCap); err != nil {
		t.Fatal(err)
	}
	return pool, ctx
}

func mustRegister(t *testing.T, ctx context.Context, s strategy.SeatStrategy, ids ...int64) {
	t.Helper()
	for _, id := range ids {
		if err := s.Register(ctx, id, 1); err != nil {
			t.Fatalf("register %d: %v", id, err)
		}
	}
}

func mustJoin(t *testing.T, ctx context.Context, wl *waitlist.Store, id int64, wantPos int) {
	t.Helper()
	pos, err := wl.Join(ctx, id, 1)
	if err != nil || pos != wantPos {
		t.Fatalf("join %d = (%d, %v), want (%d, nil)", id, pos, err, wantPos)
	}
}

func wantErr(t *testing.T, what string, got, want error) {
	t.Helper()
	if !errors.Is(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

// checkInvariants verifies the rules the invariant checker will watch in production.
func checkInvariants(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (enrolled, waiting int) {
	t.Helper()
	var capacity, rows, both int
	err := pool.QueryRow(ctx, `
		SELECT s.capacity, s.enrolled,
		       (SELECT count(*) FROM enrollments WHERE section_id = 1),
		       (SELECT count(*) FROM waitlist WHERE section_id = 1),
		       (SELECT count(*) FROM waitlist w JOIN enrollments e USING (student_id, section_id))
		  FROM sections s WHERE s.id = 1`,
	).Scan(&capacity, &enrolled, &rows, &waiting, &both)
	if err != nil {
		t.Fatal(err)
	}
	if enrolled != rows {
		t.Errorf("enrolled column %d != enrollment rows %d", enrolled, rows)
	}
	if enrolled > capacity {
		t.Errorf("over-enrolled: %d > %d", enrolled, capacity)
	}
	if both != 0 {
		t.Errorf("%d students are both enrolled and waitlisted", both)
	}
	if waiting > 0 && enrolled < capacity {
		t.Errorf("%d open seats while %d students wait", capacity-enrolled, waiting)
	}
	return enrolled, waiting
}

func TestJoinLeavePositionRules(t *testing.T) {
	pool, ctx := setup(t, 2, 2, 10)
	seats := strategy.NewAtomic(pool)
	wl := waitlist.New(pool)

	_, err := wl.Join(ctx, 1, 1)
	wantErr(t, "join with open seats", err, waitlist.ErrSeatsAvailable)
	_, err = wl.Join(ctx, 1, 99)
	wantErr(t, "join unknown section", err, waitlist.ErrNotFound)

	mustRegister(t, ctx, seats, 1, 2)
	_, err = wl.Join(ctx, 1, 1)
	wantErr(t, "join while enrolled", err, waitlist.ErrAlreadyEnrolled)

	mustJoin(t, ctx, wl, 3, 1)
	mustJoin(t, ctx, wl, 4, 2)
	_, err = wl.Join(ctx, 3, 1)
	wantErr(t, "join twice", err, waitlist.ErrAlreadyWaitlisted)
	_, err = wl.Join(ctx, 5, 1)
	wantErr(t, "join full list", err, waitlist.ErrWaitlistFull)

	if pos, err := wl.Position(ctx, 4, 1); err != nil || pos != 2 {
		t.Fatalf("position 4 = (%d, %v), want 2", pos, err)
	}
	if info, err := wl.Info(ctx, 1); err != nil || info != (waitlist.Info{SectionID: 1, Length: 2, Cap: 2}) {
		t.Fatalf("info = (%+v, %v)", info, err)
	}

	if err := wl.Leave(ctx, 3, 1); err != nil {
		t.Fatal(err)
	}
	wantErr(t, "leave twice", wl.Leave(ctx, 3, 1), waitlist.ErrNotWaitlisted)
	_, err = wl.Position(ctx, 3, 1)
	wantErr(t, "position after leave", err, waitlist.ErrNotWaitlisted)
	if pos, err := wl.Position(ctx, 4, 1); err != nil || pos != 1 {
		t.Fatalf("position 4 after leave = (%d, %v), want 1", pos, err)
	}
	_, err = wl.Join(ctx, 9999, 1)
	wantErr(t, "join unknown student", err, waitlist.ErrNotFound)
	_, err = wl.Info(ctx, 99)
	wantErr(t, "info unknown section", err, waitlist.ErrNotFound)

	checkInvariants(t, ctx, pool)
}

func TestDropPromotesFirstInLine(t *testing.T) {
	pool, ctx := setup(t, 2, 5, 10)
	seats := strategy.NewAtomic(pool)
	wl := waitlist.New(pool)

	mustRegister(t, ctx, seats, 1, 2)
	mustJoin(t, ctx, wl, 3, 1)
	mustJoin(t, ctx, wl, 4, 2)
	mustJoin(t, ctx, wl, 5, 3)

	if err := seats.Drop(ctx, 1, 1); err != nil {
		t.Fatal(err)
	}
	// 3 was first in line and now holds 1's seat; 4 moves to the front.
	if err := seats.Register(ctx, 3, 1); !errors.Is(err, strategy.ErrSectionFull) &&
		!errors.Is(err, strategy.ErrAlreadyEnrolled) {
		t.Fatalf("3 should already hold a seat, register = %v", err)
	}
	var promoted bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM enrollments WHERE student_id = 3 AND section_id = 1)`).Scan(&promoted)
	if !promoted {
		t.Fatal("student 3 was not promoted")
	}
	if pos, err := wl.Position(ctx, 4, 1); err != nil || pos != 1 {
		t.Fatalf("position 4 = (%d, %v), want 1", pos, err)
	}
	if enrolled, waiting := checkInvariants(t, ctx, pool); enrolled != 2 || waiting != 2 {
		t.Fatalf("enrolled=%d waiting=%d, want 2/2", enrolled, waiting)
	}

	// A failed drop must not promote anyone.
	wantErr(t, "drop not enrolled", seats.Drop(ctx, 9, 1), strategy.ErrNotEnrolled)
	if _, waiting := checkInvariants(t, ctx, pool); waiting != 2 {
		t.Fatalf("waiting=%d after failed drop, want 2", waiting)
	}
}

// Many students race to join a full section; exactly waitlist_cap get in, with
// positions 1..cap and no duplicates.
func TestConcurrentJoinsRespectCap(t *testing.T) {
	const waitCap, racers = 10, 200
	pool, ctx := setup(t, 1, waitCap, racers+1)
	mustRegister(t, ctx, strategy.NewAtomic(pool), 1)
	wl := waitlist.New(pool)

	var wg sync.WaitGroup
	var joined, full int64
	var mu sync.Mutex
	seen := map[int]bool{}
	start := make(chan struct{})
	for id := int64(2); id <= racers+1; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			pos, err := wl.Join(ctx, id, 1)
			switch {
			case err == nil:
				atomic.AddInt64(&joined, 1)
				mu.Lock()
				seen[pos] = true
				mu.Unlock()
			case errors.Is(err, waitlist.ErrWaitlistFull):
				atomic.AddInt64(&full, 1)
			default:
				t.Errorf("join %d: %v", id, err)
			}
		}(id)
	}
	close(start)
	wg.Wait()

	if joined != waitCap || full != racers-waitCap || len(seen) != waitCap {
		t.Fatalf("joined=%d full=%d distinct positions=%d, want %d/%d/%d",
			joined, full, len(seen), waitCap, racers-waitCap, waitCap)
	}
	if _, waiting := checkInvariants(t, ctx, pool); waiting != waitCap {
		t.Fatalf("waiting=%d, want %d", waiting, waitCap)
	}
}

// Every enrolled student drops while half the waitlist tries to leave at the same time.
// Each waiter ends up either promoted or gone, never both, and no seat is left open
// while someone waits.
func TestConcurrentDropsAndLeaves(t *testing.T) {
	const capacity = 20
	pool, ctx := setup(t, capacity, capacity, 2*capacity)
	seats := strategy.NewAtomic(pool)
	wl := waitlist.New(pool)
	for id := int64(1); id <= capacity; id++ {
		mustRegister(t, ctx, seats, id)
	}
	for id := int64(capacity + 1); id <= 2*capacity; id++ {
		mustJoin(t, ctx, wl, id, int(id-capacity))
	}

	var wg sync.WaitGroup
	var left int64
	start := make(chan struct{})
	for id := int64(1); id <= capacity; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			if err := seats.Drop(ctx, id, 1); err != nil {
				t.Errorf("drop %d: %v", id, err)
			}
		}(id)
	}
	for id := int64(capacity + 1); id <= capacity+capacity/2; id++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			<-start
			switch err := wl.Leave(ctx, id, 1); {
			case err == nil:
				atomic.AddInt64(&left, 1)
			case errors.Is(err, waitlist.ErrNotWaitlisted): // promoted first
			default:
				t.Errorf("leave %d: %v", id, err)
			}
		}(id)
	}
	close(start)
	wg.Wait()

	enrolled, waiting := checkInvariants(t, ctx, pool)
	t.Logf("enrolled=%d waiting=%d left=%d", enrolled, waiting, left)
	if waiting != 0 || int64(enrolled)+left != capacity {
		t.Fatalf("enrolled=%d + left=%d should be %d with nobody waiting (waiting=%d)",
			enrolled, left, capacity, waiting)
	}
	var originals int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM enrollments WHERE student_id <= $1`, capacity).Scan(&originals)
	if originals != 0 {
		t.Fatalf("%d original students still enrolled after dropping", originals)
	}
}
