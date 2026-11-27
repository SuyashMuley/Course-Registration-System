# Course Registration System

A scalable course registration system for BSDS: Go + PostgreSQL + Redis on AWS. We study how registration behaves when thousands of students enroll at once: correctness (no over-enrollment), scaling limits, failures, and multi-region behavior.

Design: seat writes are CP (single consistent writer); reads are AP-leaning (Redis cache). See [docs/architecture.md](docs/architecture.md) and [docs/proposal.md](docs/proposal.md).

## Layout and owners

| Path | Owner | Contents |
|---|---|---|
| `cmd/server/`, `internal/api/`, `internal/strategy/`, `migrations/` | Suyash | Go service, seat strategies, schema |
| `internal/metrics/`, `internal/waitlist/`, `observability/`, `analysis/` | Sameep | Metrics, waitlist backend, dashboards, alerts, invariant checker, analysis |
| `infra/`, `deploy/`, `loadtest/` | Santosh | Terraform, Docker, load tests |
| `docs/` | everyone | Proposal and architecture |

Ownership is enforced by `.github/CODEOWNERS`.

## Workflow

- Branch per task, named `<name>/<task>`. Open a pull request into `main`; the path owner approves.
- Keep PRs small and merge the same day. Rebase on `main` before opening a PR.
- Never force-push `main`. Never commit secrets, `.env`, or Terraform state.
- AI-assisted code gets a human review and must pass the over-enrollment test.

## Run

```
docker compose -f deploy/docker/docker-compose.yml up -d --build   # Postgres + app on :8080
```

## API

Request bodies are JSON `{"student_id": N, "section_id": N}`.

| Method | Route | Success | Errors |
|---|---|---|---|
| GET | `/healthz` | `ok` | |
| POST | `/register` | `enrolled` | 409 `full`, `already_enrolled`; 404 `not_found` |
| POST | `/drop` | `dropped` (and promotes the first waitlisted student into the seat, same transaction) | 409 `not_enrolled` |
| GET | `/sections/{id}` | `{id, capacity, enrolled, available}` | 404 |
| POST | `/waitlist/join` | `waitlisted` with `position` (1 = next) | 409 `seats_available`, `already_enrolled`, `already_waitlisted`, `waitlist_full`; 404 `not_found` |
| POST | `/waitlist/leave` | `left` | 409 `not_waitlisted` |
| GET | `/waitlist/{section_id}/position?student_id=N` | `waitlisted` with `position` | 404 `not_waitlisted` |
| GET | `/sections/{id}/waitlist` | `{section_id, length, cap}` | 404 |

Students join the waitlist only when a section is full, and promotion is first come, first served. Only the `atomic` strategy promotes on drop; `naive` is the E1 baseline.

## Tests

```
go test ./...                                            # unit tests
docker run -d --name pgtest -e POSTGRES_PASSWORD=pw -p 55432:5432 postgres:16
TEST_DATABASE_URL=postgres://postgres:pw@localhost:55432/postgres go test ./...   # + integration
```
