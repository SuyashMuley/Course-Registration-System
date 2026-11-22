// Package metrics holds request instrumentation for Prometheus.
//
// Owner: Sameep. This is a no-op stub so the service compiles before real metrics exist.
// Planned: latency histogram, requests by status, seat-conflict/retry counters,
// DB connection-pool usage, and a /metrics handler.
package metrics

import "net/http"

// Middleware wraps a handler with request metrics. The stub passes requests through.
func Middleware(next http.Handler) http.Handler {
	return next
}
