// Package metrics exposes Prometheus metrics for the registration service at /metrics.
//
// Owner: Sameep.
//
// What is measured (all names start with crs_):
//
//	HTTP          requests by route/method/status code, latency histogram, in-flight requests
//	Seat ops      register/drop outcomes per strategy (ok, full, already_enrolled, ...) and
//	              their latency; "full" and "already_enrolled" are the seat conflicts
//	DB pool       pgxpool usage: connections in use / idle / max, acquires that had to
//	              wait for a free connection, and total wait time (the E2 scaling limit)
//
// Plus the standard Go runtime and process metrics. Prometheus adds the instance label,
// so per-instance numbers come for free when several app servers run behind the ALB.
package metrics

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Path is where the metrics are served. Requests to it are not counted themselves.
const Path = "/metrics"

// latencyBuckets cover a ~1 ms single-statement seat claim up to multi-second queueing
// under overload (seconds).
var latencyBuckets = []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// Metrics owns a registry and the service's collectors. Most code uses the package-level
// functions, which act on Default; tests create their own with New.
type Metrics struct {
	reg *prometheus.Registry

	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge

	seatOps      *prometheus.CounterVec
	seatDuration *prometheus.HistogramVec
}

// New creates a registry with the service metrics and the Go runtime/process collectors.
func New() *Metrics {
	m := &Metrics{
		reg: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crs_http_requests_total",
			Help: "HTTP requests by route pattern, method and status code.",
		}, []string{"route", "method", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "crs_http_request_duration_seconds",
			Help:    "HTTP request latency by route pattern and method.",
			Buckets: latencyBuckets,
		}, []string{"route", "method"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "crs_http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
		seatOps: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "crs_seat_operations_total",
			Help: "Seat operations by strategy, operation (register, drop) and outcome.",
		}, []string{"strategy", "op", "outcome"}),
		seatDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "crs_seat_operation_duration_seconds",
			Help:    "Time spent in the seat strategy (database work only, no HTTP).",
			Buckets: latencyBuckets,
		}, []string{"strategy", "op"}),
	}
	m.reg.MustRegister(
		m.requests, m.duration, m.inFlight, m.seatOps, m.seatDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// Default is the registry served by the package-level Middleware.
var Default = New()

// Middleware records HTTP metrics for next and serves /metrics from Default.
func Middleware(next http.Handler) http.Handler { return Default.Middleware(next) }

// InstrumentStrategy wraps s so Default records seat-operation outcomes and latency.
func InstrumentStrategy(s strategy.SeatStrategy) strategy.SeatStrategy {
	return Default.InstrumentStrategy(s)
}

// RegisterPool exports pool statistics through Default.
func RegisterPool(pool *pgxpool.Pool) error { return Default.RegisterPool(pool) }

// Handler serves the registry in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.reg, promhttp.HandlerOpts{Registry: m.reg})
}

// Middleware records request count, latency and in-flight requests, and answers
// GET /metrics itself so the API router does not need to know about it.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	metricsHandler := m.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == Path {
			metricsHandler.ServeHTTP(w, r)
			return
		}
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := routeLabel(r.Pattern)
		m.requests.WithLabelValues(route, r.Method, strconv.Itoa(rec.code)).Inc()
		m.duration.WithLabelValues(route, r.Method).Observe(time.Since(start).Seconds())
	})
}

// routeLabel turns the ServeMux pattern that matched ("POST /register",
// "GET /sections/{id}") into a bounded label ("/register", "/sections/{id}"). Raw paths
// are never used, so /sections/1 ... /sections/99999 do not each create a time series.
// The mux fills in r.Pattern on the request it is given, which is this one.
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if _, path, ok := strings.Cut(pattern, " "); ok {
		return path
	}
	return pattern
}

type statusRecorder struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteHeader {
		s.code, s.wroteHeader = code, true
	}
	s.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.ResponseController reach the underlying writer (flush, deadlines).
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// InstrumentStrategy wraps s; the wrapper has the same Name and behaviour.
func (m *Metrics) InstrumentStrategy(s strategy.SeatStrategy) strategy.SeatStrategy {
	return &instrumented{inner: s, m: m}
}

type instrumented struct {
	inner strategy.SeatStrategy
	m     *Metrics
}

func (i *instrumented) Name() string { return i.inner.Name() }

func (i *instrumented) Register(ctx context.Context, studentID, sectionID int64) error {
	start := time.Now()
	err := i.inner.Register(ctx, studentID, sectionID)
	i.observe("register", start, err)
	return err
}

func (i *instrumented) Drop(ctx context.Context, studentID, sectionID int64) error {
	start := time.Now()
	err := i.inner.Drop(ctx, studentID, sectionID)
	i.observe("drop", start, err)
	return err
}

func (i *instrumented) observe(op string, start time.Time, err error) {
	name := i.inner.Name()
	i.m.seatOps.WithLabelValues(name, op, outcome(err)).Inc()
	i.m.seatDuration.WithLabelValues(name, op).Observe(time.Since(start).Seconds())
}

func outcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, strategy.ErrSectionFull):
		return "full"
	case errors.Is(err, strategy.ErrAlreadyEnrolled):
		return "already_enrolled"
	case errors.Is(err, strategy.ErrNotEnrolled):
		return "not_enrolled"
	case errors.Is(err, strategy.ErrNotFound):
		return "not_found"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "error"
	}
}

// RegisterPool exports the pool's statistics, read fresh on every scrape.
func (m *Metrics) RegisterPool(pool *pgxpool.Pool) error {
	return m.reg.Register(newPoolCollector(func() PoolStats { return pool.Stat() }))
}

// PoolStats is the part of *pgxpool.Stat the collector reads (an interface so tests can
// fake it).
type PoolStats interface {
	MaxConns() int32
	TotalConns() int32
	AcquiredConns() int32
	IdleConns() int32
	ConstructingConns() int32
	AcquireCount() int64
	EmptyAcquireCount() int64
	CanceledAcquireCount() int64
	AcquireDuration() time.Duration
}

type poolCollector struct {
	stat func() PoolStats

	maxConns, totalConns, acquiredConns, idleConns, constructingConns *prometheus.Desc
	acquires, emptyAcquires, canceledAcquires, acquireWait            *prometheus.Desc
}

func newPoolCollector(stat func() PoolStats) *poolCollector {
	d := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc("crs_db_pool_"+name, help, nil, nil)
	}
	return &poolCollector{
		stat:              stat,
		maxConns:          d("max_conns", "Maximum pool size (DB_MAX_CONNS)."),
		totalConns:        d("total_conns", "Open connections."),
		acquiredConns:     d("acquired_conns", "Connections in use by a request."),
		idleConns:         d("idle_conns", "Open connections not in use."),
		constructingConns: d("constructing_conns", "Connections being opened."),
		acquires:          d("acquires_total", "Connections handed out."),
		emptyAcquires: d("empty_acquires_total",
			"Acquires that found no idle connection and had to wait or open one; climbs when the pool is the bottleneck."),
		canceledAcquires: d("canceled_acquires_total", "Acquires canceled before a connection was free."),
		acquireWait:      d("acquire_wait_seconds_total", "Total time spent waiting for a connection."),
	}
}

func (c *poolCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.maxConns, c.totalConns, c.acquiredConns, c.idleConns, c.constructingConns,
		c.acquires, c.emptyAcquires, c.canceledAcquires, c.acquireWait,
	} {
		ch <- d
	}
}

func (c *poolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.stat()
	gauge := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v)
	}
	counter := func(d *prometheus.Desc, v float64) {
		ch <- prometheus.MustNewConstMetric(d, prometheus.CounterValue, v)
	}
	gauge(c.maxConns, float64(s.MaxConns()))
	gauge(c.totalConns, float64(s.TotalConns()))
	gauge(c.acquiredConns, float64(s.AcquiredConns()))
	gauge(c.idleConns, float64(s.IdleConns()))
	gauge(c.constructingConns, float64(s.ConstructingConns()))
	counter(c.acquires, float64(s.AcquireCount()))
	counter(c.emptyAcquires, float64(s.EmptyAcquireCount()))
	counter(c.canceledAcquires, float64(s.CanceledAcquireCount()))
	counter(c.acquireWait, s.AcquireDuration().Seconds())
}
