// Command server runs the course registration API.
//
// Configuration (environment variables):
//
//	DATABASE_URL   required, e.g. postgres://user:pass@localhost:5432/registration
//	STRATEGY       "atomic" (default) or "naive" (the E1 failure baseline)
//	PORT           listen port, default 8080
//	DB_MAX_CONNS   connection pool size, default 20
//
// Owner: Suyash.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/SuyashMuley/Course-Registration-System/internal/api"
	"github.com/SuyashMuley/Course-Registration-System/internal/db"
	"github.com/SuyashMuley/Course-Registration-System/internal/metrics"
	"github.com/SuyashMuley/Course-Registration-System/internal/strategy"
	"github.com/SuyashMuley/Course-Registration-System/internal/waitlist"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := db.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	pool, err := db.Open(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	name := os.Getenv("STRATEGY")
	if name == "" {
		name = "atomic"
	}
	strat, err := strategy.New(name, pool)
	if err != nil {
		log.Fatal(err)
	}

	addr := ":8080"
	if p := os.Getenv("PORT"); p != "" {
		addr = ":" + p
	}

	srv := &http.Server{
		Addr: addr,
		// TODO(sameep): metrics.Middleware is a pass-through until real metrics land;
		// /metrics is exposed there too.
		Handler:           metrics.Middleware(api.NewServer(strat, api.PGSections{Pool: pool}, waitlist.New(pool)).Routes()),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Printf("listening on %s (strategy=%s, max_conns=%d)", addr, strat.Name(), cfg.MaxConns)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
