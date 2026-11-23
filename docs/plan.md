# Build Plan and Task Order

Owners: **Suyash** (Go service), **Santosh** (infrastructure, load tests), **Sameep** (observability, analysis). Ownership of paths is enforced by `.github/CODEOWNERS`.

## Stage A: Proposal submission (now until the Canvas due date)

Check the due date in Canvas. One PDF per group is uploaded to Homework 9.

| # | Task | Who | Files / output |
|---|---|---|---|
| 1 | Team meeting: review `design_concerns_for_feedback.pdf` and confirm stack, roles, regions (us-east-1 + us-west-2), and scope. The waitlist is decided: it stays in the core. Check what the AWS Learner Lab allows (regions, RDS Multi-AZ, ElastiCache, cross-region replicas). | All | Decisions in chat |
| 2 | Fill the `[FILL IN]` items: bios, recent progress, chat platform, meeting time, course readings | Each person fills their own bio. Sameep: Section 4. Santosh: Sections 8–9. | `docs/proposal.md` |
| 3 | Fix the proposal for the Learner Lab limits (regions, services) | Santosh | Done (merged): second region is us-west-2 with no regional database; read replicas are blocked. |
| 4 | Build the E1 baseline code (table below) | Suyash, Santosh | Done and merged. The `analysis/e1_check.py` item was dropped: `check.sql` and the results CSV replace it. |
| 5 | Run E1 and a single-instance baseline. Write the results into Section 6. | Sameep collects; Suyash and Santosh review | `docs/proposal.md` Section 6. E1 is done: 3 runs per strategy through the HTTP service (k6) are in the Section 6 table. The single-instance baseline belongs to E2 and is not needed for submission. |
| 6 | Final read-through. Delete the "DRAFT NOTE", regenerate the `.docx`, export to PDF. | Sameep | `docs/proposal.pdf` |
| 7 | Upload one PDF to Canvas (Homework 9) | Suyash | Canvas |

### Step 4: minimum code for local E1 numbers (no AWS needed)

| Order | File | Owner | Status |
|---|---|---|---|
| 1 | `internal/db/db.go` (Postgres pool and config) | Suyash | Done (merged) |
| 2 | `internal/strategy/naive.go` and `atomic.go` (atomic: single statement for seat or waitlist) | Suyash | Done, verified against Postgres: atomic enrolled exactly 10 of 10 seats; naive enrolled 200 (200 concurrent requests). The waitlist branch is not written yet (see Stage B). |
| 3 | `internal/api/handlers.go`, wired into `cmd/server/main.go` | Suyash | Done (merged; handler unit tests pass) |
| 4 | `deploy/docker/Dockerfile` and `docker-compose.yml` (app + Postgres) | Santosh | Done (merged): Dockerfile, docker-compose.yml, README |
| 5 | `loadtest/e1-correctness/e1.js` (k6) | Santosh | Done (merged): `e1.js`, `seed.sql`, `reset.sql`, `check.sql`, 3 runs per strategy in `results/e1_summary.csv` |
| 6 | `analysis/e1_check.py` (final `enrolled` vs `capacity`, duplicates) | Sameep | Dropped: replaced by `loadtest/e1-correctness/check.sql` and the results CSV |

Orders 1–5 are done and merged as of Nov 22. Remaining before submission: the `[FILL IN]` items in `docs/proposal.md`, the final read-through, the PDF export, and the upload.

## Stage B: Build (after submission, following the proposal timeline)

### Week of Nov 23–29: single-instance service and metrics

| File | Owner |
|---|---|
| `internal/strategy/locking.go` | Suyash |
| Waitlist: schema, join/leave, promote-next in the drop transaction | Suyash |
| `internal/metrics/metrics.go` (real Prometheus metrics and `/metrics`) | Sameep |
| `observability/prometheus.yml`, first Grafana dashboard | Sameep |
| AWS Budgets alarm; trial launch of RDS and ElastiCache | Santosh |

### Week of Nov 30–Dec 6: deploy to AWS, multiple instances

| File | Owner |
|---|---|
| `infra/terraform/` (VPC, ALB, EC2, RDS Multi-AZ, ElastiCache, SNS) | Santosh |
| `internal/strategy/redis.go` (admission filter) | Suyash |
| `loadtest/e2-scaling/` | Santosh |
| `loadtest/e3-strategies/` | Suyash |
| `observability/alerts/`, `observability/invariant-checker/` (enrollment, waitlist cap, order) | Sameep |
| Prometheus EC2 service discovery config | Sameep |

### Week of Dec 7–13: experiments and failures (Phase 1), then design Phase 2 if time allows

| File | Owner |
|---|---|
| `loadtest/e4-spike/`, `e5-failure/`, `e6-caching/` | Sameep |
| Phase 2 only, after E1–E6 are done: `infra/terraform/` second region (us-west-2); `loadtest/e7-multiregion/` (direct vs. forwarded writes, cross-region latency, stale reads; no read replica, the Learner Lab blocks it) | Santosh |
| `analysis/` plots for all experiments | Sameep |
| Optional SQS variant (E4 stretch) | Whoever finishes first |

### Week of Dec 14 to the end: report and demo

| File | Owner |
|---|---|
| `docs/` final report, graphs, demo | All, each writing their own sections |

## Git order (to avoid conflicts)

1. Suyash pushes the skeleton. Add collaborators, then turn on branch protection for `main` (pull requests required, "Require review from Code Owners").
2. The Stage A step 4 files go in as small separate pull requests, each in its owner's directory.
3. The only overlap in Stage B is `cmd/server/main.go`, where Sameep's metrics middleware attaches. He sends that as a small PR after Suyash's handlers merge.
4. Branch per task (`<name>/<task>`), rebase on `main` before opening a PR, merge the same day, never force-push `main`.

## Scope priority

- **Core:** E1, E2, E3, E5, and the waitlist (required; it is not cut).
- **Stretch:** E4 (SQS variant), E6, E7 (Phase 2). If time runs short, cut E7 first, then the SQS variant and E6.
