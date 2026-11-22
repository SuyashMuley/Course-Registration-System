# Architecture: Course Registration System

Stack: **Go** (app), **PostgreSQL** (source of truth), **Redis** (admission filter and cache), on **AWS**. Local development uses Docker Compose.

![Architecture diagram](architecture.png)

Editable source: `architecture.svg`.

## Design principle

| Operation | Choice | Mechanism |
|---|---|---|
| Register / drop a seat | **CP** | One writer (RDS Postgres primary) with a synchronous Multi-AZ standby. During a partition or primary-region outage, writes are rejected or queued. |
| Seat-count and course-search reads | **AP-leaning** | Redis cache and read replicas. Brief staleness is accepted and measured (E6, E7). |
| Waitlist view | **AP-leaning** | Replica or cache read. The primary decides ordering. |

Redis is never the source of truth for seat counts. It only filters admissions and caches reads.

## Components

### Primary region (for example us-east-1)

| Component | Purpose |
|---|---|
| Load generator (k6 on EC2) | Simulates thousands of concurrent students. Runs in the same region so E1–E6 are not distorted by cross-region latency. |
| Application Load Balancer | Spreads requests across N app instances. |
| Go app instances (EC2, stateless) | Register, drop, and seat-count APIs. Expose `/metrics`. Scaled to 1, 2, 4, 8 instances in E2. |
| ElastiCache Redis | Admission filter and read cache (AP-leaning). |
| RDS Postgres primary | Single writer for seat counts (CP). Holds `sections(capacity, enrolled)` and `enrollments` with a unique `(student_id, section_id)`. |
| RDS Multi-AZ standby | Synchronous replica in a second availability zone. Takes over on failure without losing committed writes (tested in E5). |
| SQS queue + workers *(stretch)* | Optional virtual waiting room for E4. Workers apply registrations to Postgres at a steady rate. Must be idempotent because standard SQS is at-least-once. |

### Second region (for example ap-south-1), E7 only

| Component | Purpose |
|---|---|
| Remote load generator | Simulates students at another campus or time zone. |
| ALB + Go app instances | Regional entry point. Reads are served locally. Seat writes are forwarded to the primary region. |
| RDS read replica (async) | Local reads that may be stale. Replica lag is measured. |
| Regional Redis | Regional read cache. |

### Observability and alerting (both regions)

| Component | Purpose |
|---|---|
| Prometheus + Grafana | Application metrics (latency histograms, errors, seat conflicts, pool usage) and one dashboard per experiment. |
| CloudWatch Alarms | RDS, ALB, EC2, and ElastiCache metrics. |
| Invariant checker | Queries for any section where `enrolled > capacity` and raises a critical alert. |
| AWS Budgets | Cost thresholds for our own AWS spend. |
| SNS topic | Delivers all alerts by email or a chat webhook. |

## Request flows

1. **Register (primary region):** client → ALB → Go app → Redis admission check → Postgres atomic update (`UPDATE ... SET enrolled = enrolled + 1 WHERE enrolled < capacity`) → response.
2. **Read seat counts:** client → ALB → Go app → Redis cache → fall back to Postgres or a replica on a miss.
3. **Register (second region):** client → regional ALB → Go app → write forwarded to the primary region → response. Reads stay local.
4. **Queued register (stretch):** client → Go app → SQS → worker → Postgres. The client polls for the result.

## Alerts

| Alert | Source | Severity |
|---|---|---|
| Over-enrollment detected | Invariant checker | Critical |
| p95 latency above target for 5 minutes | Grafana / ALB | High |
| 5xx or registration error rate above 1% | ALB / Grafana | High |
| RDS replica lag above threshold | CloudWatch | Medium |
| RDS CPU or connections near limit | CloudWatch | Medium |
| Redis memory or evictions high | CloudWatch | Medium |
| AWS spend passes budget thresholds | AWS Budgets | Medium |

## Experiments mapped to the architecture

| # | Experiment | Parts exercised |
|---|---|---|
| E1 | Correctness under contention | App + Postgres (naive vs. locking vs. atomic update vs. Redis admission) |
| E2 | Horizontal scaling | ALB + 1, 2, 4, 8 app instances |
| E3 | Strategy performance | Postgres locking vs. atomic update vs. Redis admission |
| E4 | Traffic spike | Load generator, Redis, optional SQS |
| E5 | Failure injection | Kill an app instance, kill Redis, force RDS Multi-AZ failover |
| E6 | Read caching | Redis cache vs. direct reads (staleness) |
| E7 | Multi-region | Second region, replica lag, forwarded writes, primary-region outage |

## Open decisions for the team

- Regions and instance sizes
- Terraform or CloudFormation for infrastructure-as-code
- Whether the SQS variant and E7 stay in scope, or become future work if time runs short
