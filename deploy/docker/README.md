# deploy/docker

Owner: Santosh.

Dockerfile for the Go service and a `docker-compose.yml` for local development. Local runs are how we get E1 and baseline numbers before spending AWS credits.

| File | Purpose |
|---|---|
| `Dockerfile` | Multi-stage build: compiles `cmd/server` and copies the static binary into a minimal non-root image. Build context is the repository root. |
| `Dockerfile.dockerignore` | Keeps the build context to `go.mod`, `go.sum`, `cmd/` and `internal/`. |
| `docker-compose.yml` | Postgres 16 (schema and E1 seed loaded on first start) and the app. An optional `k6` service runs load tests in a container. |

Planned additions: Redis, nginx (local load balancer for multi-instance tests), Prometheus and Grafana.

## Usage (from the repository root)

```sh
docker compose -f deploy/docker/docker-compose.yml up -d --build    # start
curl localhost:8080/healthz                                           # check
curl localhost:8080/sections/1                                        # {"id":1,"capacity":10,...}
docker compose -f deploy/docker/docker-compose.yml logs -f app        # app logs
docker compose -f deploy/docker/docker-compose.yml down               # stop, keep data
docker compose -f deploy/docker/docker-compose.yml down -v            # stop and delete the database
```

Settings come from your shell:

| Variable | Default | Meaning |
|---|---|---|
| `STRATEGY` | `atomic` | Seat strategy: `atomic` or `naive` |
| `DB_MAX_CONNS` | `20` | App connection pool size |

Change one with, for example, `STRATEGY=naive docker compose -f deploy/docker/docker-compose.yml up -d app`.

The schema and seed run only when the Postgres volume is first created. After a schema change, run `down -v` and start again.

The Postgres user and password (`registration`) are for local use only and are never used on AWS.

For running E1, see [loadtest/e1-correctness/README.md](../../loadtest/e1-correctness/README.md).
