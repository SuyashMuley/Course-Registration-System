package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sections/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("POST /register", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.WriteHeader(http.StatusTeapot) // superfluous: must not change the recorded code
	})
	mux.HandleFunc("GET /implicit", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok")) // no WriteHeader: 200
	})
	return mux
}

func serve(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestMiddlewareLabelsByRoutePattern(t *testing.T) {
	m := New()
	h := m.Middleware(testMux())

	for i := 1; i <= 3; i++ {
		serve(h, "GET", fmt.Sprintf("/sections/%d", i))
	}
	serve(h, "POST", "/register")
	serve(h, "GET", "/implicit")
	serve(h, "GET", "/nope")

	cases := []struct {
		route, method, code string
		want                float64
	}{
		{"/sections/{id}", "GET", "200", 3}, // one series, not one per section id
		{"/register", "POST", "409", 1},
		{"/implicit", "GET", "200", 1},
		{"unmatched", "GET", "404", 1},
	}
	for _, c := range cases {
		got := testutil.ToFloat64(m.requests.WithLabelValues(c.route, c.method, c.code))
		if got != c.want {
			t.Errorf("requests{%s %s %s} = %v, want %v", c.route, c.method, c.code, got, c.want)
		}
	}
	if n := testutil.CollectAndCount(m.requests); n != len(cases) {
		t.Errorf("%d request series, want %d", n, len(cases))
	}
	if n := testutil.CollectAndCount(m.duration); n != len(cases) {
		t.Errorf("%d duration series, want %d", n, len(cases))
	}
	if got := testutil.ToFloat64(m.inFlight); got != 0 {
		t.Errorf("in flight = %v after all requests finished, want 0", got)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	m := New()
	h := m.Middleware(testMux())
	serve(h, "GET", "/sections/1")

	rec := serve(h, "GET", Path)
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics = %d", rec.Code)
	}
	for _, want := range []string{
		`crs_http_requests_total{code="200",method="GET",route="/sections/{id}"} 1`,
		"crs_http_request_duration_seconds_bucket",
		"go_goroutines",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("/metrics output is missing %q", want)
		}
	}
	// Scrapes are not counted as API traffic.
	if strings.Contains(string(body), `route="/metrics"`) || strings.Contains(string(body), `route="unmatched"`) {
		t.Error("/metrics scrape was recorded as a request")
	}
}

type fakeStrategy struct{ err error }

func (f fakeStrategy) Name() string                                 { return "fake" }
func (f fakeStrategy) Register(context.Context, int64, int64) error { return f.err }
func (f fakeStrategy) Drop(context.Context, int64, int64) error     { return f.err }

func TestInstrumentStrategyOutcomes(t *testing.T) {
	m := New()
	cases := []struct {
		err  error
		want string
	}{
		{nil, "ok"},
		{strategy.ErrSectionFull, "full"},
		{fmt.Errorf("wrapped: %w", strategy.ErrAlreadyEnrolled), "already_enrolled"},
		{strategy.ErrNotEnrolled, "not_enrolled"},
		{strategy.ErrNotFound, "not_found"},
		{context.DeadlineExceeded, "canceled"},
		{errors.New("connection reset"), "error"},
	}
	for _, c := range cases {
		s := m.InstrumentStrategy(fakeStrategy{err: c.err})
		if s.Name() != "fake" {
			t.Fatalf("wrapper name = %q", s.Name())
		}
		if err := s.Register(context.Background(), 1, 1); !errors.Is(err, c.err) {
			t.Fatalf("wrapper changed the error: %v", err)
		}
		if got := testutil.ToFloat64(m.seatOps.WithLabelValues("fake", "register", c.want)); got != 1 {
			t.Errorf("outcome %q count = %v, want 1", c.want, got)
		}
	}
	_ = m.InstrumentStrategy(fakeStrategy{}).Drop(context.Background(), 1, 1)
	if got := testutil.ToFloat64(m.seatOps.WithLabelValues("fake", "drop", "ok")); got != 1 {
		t.Errorf("drop ok = %v, want 1", got)
	}
}

type fakeStats struct{}

func (fakeStats) MaxConns() int32                { return 20 }
func (fakeStats) TotalConns() int32              { return 12 }
func (fakeStats) AcquiredConns() int32           { return 9 }
func (fakeStats) IdleConns() int32               { return 3 }
func (fakeStats) ConstructingConns() int32       { return 0 }
func (fakeStats) AcquireCount() int64            { return 500 }
func (fakeStats) EmptyAcquireCount() int64       { return 40 }
func (fakeStats) CanceledAcquireCount() int64    { return 2 }
func (fakeStats) AcquireDuration() time.Duration { return 1500 * time.Millisecond }

func TestPoolCollector(t *testing.T) {
	c := newPoolCollector(func() PoolStats { return fakeStats{} })
	want := `
# HELP crs_db_pool_acquired_conns Connections in use by a request.
# TYPE crs_db_pool_acquired_conns gauge
crs_db_pool_acquired_conns 9
# HELP crs_db_pool_acquire_wait_seconds_total Total time spent waiting for a connection.
# TYPE crs_db_pool_acquire_wait_seconds_total counter
crs_db_pool_acquire_wait_seconds_total 1.5
# HELP crs_db_pool_max_conns Maximum pool size (DB_MAX_CONNS).
# TYPE crs_db_pool_max_conns gauge
crs_db_pool_max_conns 20
`
	err := testutil.CollectAndCompare(c, strings.NewReader(want),
		"crs_db_pool_acquired_conns", "crs_db_pool_acquire_wait_seconds_total", "crs_db_pool_max_conns")
	if err != nil {
		t.Fatal(err)
	}
	if n := testutil.CollectAndCount(c); n != 9 {
		t.Fatalf("%d pool metrics, want 9", n)
	}
	if problems, err := testutil.CollectAndLint(c); err != nil || len(problems) != 0 {
		t.Fatalf("lint: %v %v", problems, err)
	}
}
