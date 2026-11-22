// Package db opens the PostgreSQL connection pool.
//
// Owner: Suyash.
package db

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds connection settings. Everything comes from environment variables so the
// same binary runs locally (Docker Compose) and on AWS (RDS) without code changes.
type Config struct {
	URL      string // DATABASE_URL, e.g. postgres://user:pass@host:5432/registration
	MaxConns int32  // DB_MAX_CONNS (default 20); the connection pool is a scaling limit in E2
}

// ConfigFromEnv reads DATABASE_URL and DB_MAX_CONNS.
func ConfigFromEnv() (Config, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is not set")
	}
	cfg := Config{URL: url, MaxConns: 20}
	if v := os.Getenv("DB_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("DB_MAX_CONNS must be a positive integer, got %q", v)
		}
		cfg.MaxConns = int32(n)
	}
	return cfg, nil
}

// Open creates the pool and verifies the database is reachable.
func Open(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	pc, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	pc.MaxConns = cfg.MaxConns

	pool, err := pgxpool.NewWithConfig(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}
