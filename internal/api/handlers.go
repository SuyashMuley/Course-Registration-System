// Package api holds the HTTP handlers for the registration service.
//
// Owner: Suyash.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/SuyashMuley/Course-Registration-System/internal/waitlist"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SectionStore reads section data for the seat-count endpoint.
type SectionStore interface {
	Section(ctx context.Context, id int64) (Section, error)
}

// Section is what GET /sections/{id} returns.
type Section struct {
	ID        int64 `json:"id"`
	Capacity  int   `json:"capacity"`
	Enrolled  int   `json:"enrolled"`
	Available int   `json:"available"`
}

// WaitlistStore is the waitlist backend (internal/waitlist).
type WaitlistStore interface {
	Join(ctx context.Context, studentID, sectionID int64) (int, error)
	Leave(ctx context.Context, studentID, sectionID int64) error
	Position(ctx context.Context, studentID, sectionID int64) (int, error)
	Info(ctx context.Context, sectionID int64) (waitlist.Info, error)
}

// Server wires the seat strategy, section reads and the waitlist to HTTP routes.
type Server struct {
	strat    strategy.SeatStrategy
	sections SectionStore
	wl       WaitlistStore
}

func NewServer(s strategy.SeatStrategy, store SectionStore, wl WaitlistStore) *Server {
	return &Server{strat: s, sections: store, wl: wl}
}

// Routes returns the router. Go 1.22+ method and wildcard patterns are used.
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /register", s.handleRegister)
	mux.HandleFunc("POST /drop", s.handleDrop)
	mux.HandleFunc("GET /sections/{id}", s.handleSection)
	mux.HandleFunc("GET /sections/{id}/waitlist", s.handleWaitlistInfo)
	mux.HandleFunc("POST /waitlist/join", s.handleWaitlistJoin)
	mux.HandleFunc("POST /waitlist/leave", s.handleWaitlistLeave)
	mux.HandleFunc("GET /waitlist/{section_id}/position", s.handleWaitlistPosition)
	return mux
}

type request struct {
	StudentID int64 `json:"student_id"`
	SectionID int64 `json:"section_id"`
}

type response struct {
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
	Position int    `json:"position,omitempty"` // waitlist position, 1 = next in line
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	err := s.strat.Register(r.Context(), req.StudentID, req.SectionID)
	writeResult(w, "enrolled", err)
}

func (s *Server) handleDrop(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	err := s.strat.Drop(r.Context(), req.StudentID, req.SectionID)
	writeResult(w, "dropped", err)
}

func (s *Server) handleWaitlistJoin(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	pos, err := s.wl.Join(r.Context(), req.StudentID, req.SectionID)
	if err != nil {
		writeResult(w, "", err)
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "waitlisted", Position: pos})
}

func (s *Server) handleWaitlistLeave(w http.ResponseWriter, r *http.Request) {
	req, ok := decode(w, r)
	if !ok {
		return
	}
	err := s.wl.Leave(r.Context(), req.StudentID, req.SectionID)
	writeResult(w, "left", err)
}

// GET /waitlist/{section_id}/position?student_id=N
func (s *Server) handleWaitlistPosition(w http.ResponseWriter, r *http.Request) {
	sectionID, err1 := strconv.ParseInt(r.PathValue("section_id"), 10, 64)
	studentID, err2 := strconv.ParseInt(r.URL.Query().Get("student_id"), 10, 64)
	if err1 != nil || err2 != nil || sectionID <= 0 || studentID <= 0 {
		writeJSON(w, http.StatusBadRequest, response{
			Status: "error", Error: "need a positive section id in the path and a positive student_id query parameter"})
		return
	}
	pos, err := s.wl.Position(r.Context(), studentID, sectionID)
	if errors.Is(err, waitlist.ErrNotWaitlisted) {
		writeJSON(w, http.StatusNotFound, response{Status: "not_waitlisted", Error: err.Error()})
		return
	}
	if err != nil {
		writeResult(w, "", err)
		return
	}
	writeJSON(w, http.StatusOK, response{Status: "waitlisted", Position: pos})
}

func (s *Server) handleWaitlistInfo(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, response{Status: "error", Error: "invalid section id"})
		return
	}
	info, err := s.wl.Info(r.Context(), id)
	if err != nil {
		writeResult(w, "", err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) handleSection(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, response{Status: "error", Error: "invalid section id"})
		return
	}
	sec, err := s.sections.Section(r.Context(), id)
	if errors.Is(err, strategy.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, response{Status: "error", Error: err.Error()})
		return
	}
	if err != nil {
		log.Printf("section %d: %v", id, err)
		writeJSON(w, http.StatusInternalServerError, response{Status: "error", Error: "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, sec)
}

// decode parses and validates a JSON body with positive student_id and section_id.
func decode(w http.ResponseWriter, r *http.Request) (request, bool) {
	var req request
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil || req.StudentID <= 0 || req.SectionID <= 0 {
		writeJSON(w, http.StatusBadRequest, response{
			Status: "error", Error: "body must be JSON with positive student_id and section_id"})
		return request{}, false
	}
	return req, true
}

// writeResult maps a strategy or waitlist result to an HTTP status.
func writeResult(w http.ResponseWriter, okStatus string, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, response{Status: okStatus})
	case errors.Is(err, strategy.ErrSectionFull):
		writeJSON(w, http.StatusConflict, response{Status: "full", Error: err.Error()})
	case errors.Is(err, strategy.ErrAlreadyEnrolled), errors.Is(err, waitlist.ErrAlreadyEnrolled):
		writeJSON(w, http.StatusConflict, response{Status: "already_enrolled", Error: err.Error()})
	case errors.Is(err, strategy.ErrNotEnrolled):
		writeJSON(w, http.StatusConflict, response{Status: "not_enrolled", Error: err.Error()})
	case errors.Is(err, waitlist.ErrAlreadyWaitlisted):
		writeJSON(w, http.StatusConflict, response{Status: "already_waitlisted", Error: err.Error()})
	case errors.Is(err, waitlist.ErrWaitlistFull):
		writeJSON(w, http.StatusConflict, response{Status: "waitlist_full", Error: err.Error()})
	case errors.Is(err, waitlist.ErrSeatsAvailable):
		writeJSON(w, http.StatusConflict, response{Status: "seats_available", Error: err.Error()})
	case errors.Is(err, waitlist.ErrNotWaitlisted):
		writeJSON(w, http.StatusConflict, response{Status: "not_waitlisted", Error: err.Error()})
	case errors.Is(err, strategy.ErrNotFound), errors.Is(err, waitlist.ErrNotFound):
		writeJSON(w, http.StatusNotFound, response{Status: "not_found", Error: err.Error()})
	default:
		log.Printf("request failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, response{Status: "error", Error: "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// PGSections reads sections straight from Postgres. (Redis caching is added later for E6.)
type PGSections struct{ Pool *pgxpool.Pool }

func (p PGSections) Section(ctx context.Context, id int64) (Section, error) {
	s := Section{ID: id}
	err := p.Pool.QueryRow(ctx,
		`SELECT capacity, enrolled FROM sections WHERE id = $1`, id,
	).Scan(&s.Capacity, &s.Enrolled)
	if errors.Is(err, pgx.ErrNoRows) {
		return Section{}, strategy.ErrNotFound
	}
	if err != nil {
		return Section{}, err
	}
	s.Available = s.Capacity - s.Enrolled
	return s, nil
}
