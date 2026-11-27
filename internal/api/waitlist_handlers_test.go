package api

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/SuyashMuley/Course-Registration-System/internal/waitlist"
)

type fakeWaitlist struct {
	pos  int
	info waitlist.Info
	err  error
}

func (f fakeWaitlist) Join(context.Context, int64, int64) (int, error)     { return f.pos, f.err }
func (f fakeWaitlist) Leave(context.Context, int64, int64) error           { return f.err }
func (f fakeWaitlist) Position(context.Context, int64, int64) (int, error) { return f.pos, f.err }
func (f fakeWaitlist) Info(context.Context, int64) (waitlist.Info, error)  { return f.info, f.err }

func TestWaitlistJoinStatusCodes(t *testing.T) {
	const body = `{"student_id":1,"section_id":1}`
	tests := []struct {
		name       string
		err        error
		body       string
		wantCode   int
		wantStatus string
	}{
		{"joined", nil, body, http.StatusOK, `"status":"waitlisted","position":3`},
		{"seats open", waitlist.ErrSeatsAvailable, body, http.StatusConflict, `"seats_available"`},
		{"list full", waitlist.ErrWaitlistFull, body, http.StatusConflict, `"waitlist_full"`},
		{"already waitlisted", waitlist.ErrAlreadyWaitlisted, body, http.StatusConflict, `"already_waitlisted"`},
		{"already enrolled", waitlist.ErrAlreadyEnrolled, body, http.StatusConflict, `"already_enrolled"`},
		{"not found", waitlist.ErrNotFound, body, http.StatusNotFound, `"not_found"`},
		{"bad body", nil, `{"student_id":1}`, http.StatusBadRequest, `"error"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{pos: 3, err: tc.err})
			rec := do(t, s, "POST", "/waitlist/join", tc.body)
			if rec.Code != tc.wantCode || !strings.Contains(rec.Body.String(), tc.wantStatus) {
				t.Fatalf("got %d %s, want %d containing %s", rec.Code, rec.Body.String(), tc.wantCode, tc.wantStatus)
			}
		})
	}
}

func TestWaitlistLeave(t *testing.T) {
	const body = `{"student_id":1,"section_id":1}`
	s := NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{})
	if rec := do(t, s, "POST", "/waitlist/leave", body); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"left"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	s = NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{err: waitlist.ErrNotWaitlisted})
	if got := do(t, s, "POST", "/waitlist/leave", body).Code; got != http.StatusConflict {
		t.Fatalf("not waitlisted = %d, want 409", got)
	}
}

func TestWaitlistPosition(t *testing.T) {
	s := NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{pos: 2})
	if rec := do(t, s, "GET", "/waitlist/1/position?student_id=5", ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"position":2`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/waitlist/1/position", "/waitlist/x/position?student_id=5", "/waitlist/1/position?student_id=0"} {
		if got := do(t, s, "GET", path, "").Code; got != http.StatusBadRequest {
			t.Fatalf("%s = %d, want 400", path, got)
		}
	}
	s = NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{err: waitlist.ErrNotWaitlisted})
	if got := do(t, s, "GET", "/waitlist/1/position?student_id=5", "").Code; got != http.StatusNotFound {
		t.Fatalf("not waitlisted = %d, want 404", got)
	}
}

func TestWaitlistInfo(t *testing.T) {
	s := NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{info: waitlist.Info{SectionID: 1, Length: 4, Cap: 5}})
	if rec := do(t, s, "GET", "/sections/1/waitlist", ""); rec.Code != http.StatusOK ||
		!strings.Contains(rec.Body.String(), `"length":4,"cap":5`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	s = NewServer(fakeStrategy{}, fakeSections{}, fakeWaitlist{err: waitlist.ErrNotFound})
	if got := do(t, s, "GET", "/sections/9/waitlist", "").Code; got != http.StatusNotFound {
		t.Fatalf("missing section = %d, want 404", got)
	}
}
