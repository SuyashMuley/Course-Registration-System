// Command server runs the course registration API.
//
// Owner: Suyash. This is a stub: it only starts an HTTP server with a health check.
package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// TODO(suyash): register, drop, and seat-count handlers from internal/api.
	// TODO(sameep): wrap the mux with metrics.Middleware and expose /metrics.

	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
