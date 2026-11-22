package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
)

type fakeStrategy struct{ registerErr, dropErr error }

func (f fakeStrategy) Name() string { return "fake" }
func (f fakeStrategy) Register(context.Context, int64, int64) error {
	return f.registerErr
}
func (f fakeStrategy) Drop(context.Context, int64, int64) error { return f.dropErr }

type fakeSections struct {
	sec Section
	err error
}

func (f fakeSections) Section(context.Context, int64) (Section, error) { return f.sec, f.err }

func do(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestRegisterStatusCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"enrolled", nil, `{"student_id":1,"section_id":1}`, http.StatusOK},
		{"full", strategy.ErrSectionFull, `{"student_id":1,"section_id":1}`, http.StatusConflict},
		{"already enrolled", strategy.ErrAlreadyEnrolled, `{"student_id":1,"section_id":1}`, http.StatusConflict},
		{"not found", strategy.ErrNotFound, `{"student_id":1,"section_id":9}`, http.StatusNotFound},
		{"bad json", nil, `not json`, http.StatusBadRequest},
		{"missing field", nil, `{"student_id":1}`, http.StatusBadRequest},
		{"unknown field", nil, `{"student_id":1,"section_id":1,"x":2}`, http.StatusBadRequest},
		{"non-positive id", nil, `{"student_id":0,"section_id":1}`, http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(fakeStrategy{registerErr: tc.err}, fakeSections{})
			if got := do(t, s, "POST", "/register", tc.body).Code; got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestDropStatusCodes(t *testing.T) {
	s := NewServer(fakeStrategy{dropErr: strategy.ErrNotEnrolled}, fakeSections{})
	if got := do(t, s, "POST", "/drop", `{"student_id":1,"section_id":1}`).Code; got != http.StatusConflict {
		t.Fatalf("status = %d, want 409", got)
	}
	s = NewServer(fakeStrategy{}, fakeSections{})
	if got := do(t, s, "POST", "/drop", `{"student_id":1,"section_id":1}`).Code; got != http.StatusOK {
		t.Fatalf("status = %d, want 200", got)
	}
}

func TestSection(t *testing.T) {
	s := NewServer(fakeStrategy{}, fakeSections{sec: Section{ID: 1, Capacity: 10, Enrolled: 4, Available: 6}})
	rec := do(t, s, "GET", "/sections/1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"available":6`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if got := do(t, s, "GET", "/sections/abc", "").Code; got != http.StatusBadRequest {
		t.Fatalf("bad id status = %d, want 400", got)
	}
	s = NewServer(fakeStrategy{}, fakeSections{err: strategy.ErrNotFound})
	if got := do(t, s, "GET", "/sections/7", "").Code; got != http.StatusNotFound {
		t.Fatalf("missing section status = %d, want 404", got)
	}
}

func TestHealth(t *testing.T) {
	s := NewServer(fakeStrategy{}, fakeSections{})
	if got := do(t, s, "GET", "/healthz", "").Code; got != http.StatusOK {
		t.Fatalf("healthz = %d", got)
	}
}
