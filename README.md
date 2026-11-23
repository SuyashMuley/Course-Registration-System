# Course Registration System

A scalable course registration system for BSDS: Go + PostgreSQL + Redis on AWS. We study how registration behaves when thousands of students enroll at once: correctness (no over-enrollment), scaling limits, failures, and multi-region behavior.

Design: seat writes are CP (single consistent writer); reads are AP-leaning (Redis cache). See [docs/architecture.md](docs/architecture.md) and [docs/proposal.md](docs/proposal.md).

## Layout and owners

| Path | Owner | Contents |
|---|---|---|
| `cmd/server/`, `internal/api/`, `internal/strategy/`, `migrations/` | Suyash | Go service, seat strategies, schema |
| `internal/metrics/`, `observability/`, `analysis/` | Sameep | Metrics, dashboards, alerts, invariant checker, analysis |
| `infra/`, `deploy/`, `loadtest/` | Santosh | Terraform, Docker, load tests |
| `docs/` | everyone | Proposal and architecture |

Ownership is enforced by `.github/CODEOWNERS`.

## Workflow

- Branch per task, named `<name>/<task>`. Open a pull request into `main`; the path owner approves.
- Keep PRs small and merge the same day. Rebase on `main` before opening a PR.
- Never force-push `main`. Never commit secrets, `.env`, or Terraform state.
- AI-assisted code gets a human review and must pass the over-enrollment test.

## Run (stub)

```
go run ./cmd/server      # serves GET /healthz on :8080
```
