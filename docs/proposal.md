---
title: "Final Project Proposal: A Scalable Course Registration System"
author: "Suyash Muley, Santosh Sunkara, Sameep Sawant"
date: "November 2026"
---

> **DRAFT NOTE (delete before submitting):** Items marked **[FILL IN]** are facts only the team knows (backgrounds, real measurements, meeting times). Nothing in Section 6 has been measured yet. Do not submit invented numbers.

# 1. Problem, Team, and Overview of Experiments

**Problem.** When course registration opens, thousands of students hit the system within seconds, all competing for a small number of seats. Under this load, naive systems show three failures: slow or failed requests, **race conditions that over-enroll a section** (two students both read "1 seat left" and both succeed), and stale seat counts that mislead students. Students lose seats they should have had. Administrators have to manually fix over-enrolled sections and field complaints. Our stakeholders are **students** (fair, fast, correct enrollment), **registrar/administrators** (seat limits are never violated, and they can see system health), and **future developers/operators** (they need to know where the system's limits are before registration day).

**Team.**

| Member | What they bring |
|---|---|
| Suyash Muley | **[FILL IN: e.g., Go backend/API development, Postgres schema, prior HW work on the Go DynamoDB store]** |
| Santosh Sunkara | **[FILL IN: e.g., AWS deployment/infrastructure, load testing]** |
| Sameep Sawant | **[FILL IN: e.g., observability, data analysis, Redis/caching]** |

**Results our experiments will evaluate.** (1) *Correctness*: zero over-enrollment and zero lost or duplicated enrollments under contention. (2) *Performance*: throughput (registrations/s), median and p95/p99 latency, error rate. (3) *Scalability*: how throughput changes as we add application instances, and where the database becomes the bottleneck. (4) *Resilience*: behavior during traffic spikes and when an application server or Redis is killed. (5) *Multi-region behavior*: many universities have campuses in different locations and time zones, so we evaluate latency, consistency, and failover when users connect from a remote region.

**Role of AI.** AI is a development assistant (code generation, test scaffolding, log/metric analysis, and documentation drafts). The *system itself* has no AI component in its request path. This keeps behavior deterministic and cost predictable. See Sections 2 and 3.

**Observability plan.** Every Go instance exports metrics (request count, latency histograms, error counts, seat-conflict/retry counts, DB connection-pool usage) to Prometheus and displays them in Grafana. AWS CloudWatch supplies infrastructure metrics for RDS (CPU, connections, IOPS), ElastiCache, the load balancer, and EC2. Logs are structured JSON with a request ID that follows each request through the load balancer, app, and database. Alerting is built in: CloudWatch Alarms and Grafana alerts on latency, error rate, replica lag, and an over-enrollment invariant check, delivered through Amazon SNS. Load-test results are stored as CSV so every graph in the final report can be regenerated.

# 2. Project Plan and Recent Progress

**Recent progress.** **[FILL IN with what is actually done. Suggested items: architecture chosen; repo created; data model drafted; first endpoints working; load-test tool selected.]** Our earlier coursework (the key-value/DynamoDB store from the homework) gives us a base for storing and updating seat counts.

**Timeline and task breakdown** (about 3 weeks of remaining work; adjust to the real deadline):

| Week | Milestone | Owner(s) |
|---|---|---|
| Nov 23–29 | Single-instance Go service (register, drop, seat-count APIs); Postgres schema; local Docker Compose; metrics endpoint; AWS account/budget alarm | Suyash (API/DB), Sameep (metrics), Santosh (AWS setup) |
| Nov 30–Dec 6 | Deploy to AWS (EC2 instances behind an ALB, RDS Postgres, ElastiCache Redis); concurrency-control variants; Prometheus + Grafana; load-test harness | Santosh (AWS/LB/harness), Suyash (concurrency), Sameep (dashboards) |
| Dec 7–13 | Run all experiments; Redis variants; failure injection; second-region deployment (read replica + remote clients, E7); analysis | All; Santosh runs tests and second region, Sameep analyzes |
| Dec 14–end | Final report, graphs, demo, cleanup, README for classmates | All |

**Who does what.** Suyash owns the application service and correctness (concurrency control, tests). Santosh owns infrastructure and load generation (load balancer, deployment, workload scripts, failure injection). Sameep owns observability and analysis (metrics, dashboards, plots, cache layer). Each person reviews at least one other person's pull request every week.

**AI in the plan: costs and benefits.**

*Benefits:* faster boilerplate (handlers, Docker/Prometheus configs, load-test scripts); help writing concurrency tests and edge cases we might miss; quicker log and result analysis; draft documentation.

*Costs/risks:* generated concurrency code can look right but contain subtle races, so it needs careful review and the over-enrollment test. Wrong or hallucinated configs can waste debugging time. There is time spent writing prompts and verifying output. Tool subscription or API costs are minimal **[FILL IN]**. Mitigation: no AI-generated code is merged without a human reviewer and passing tests, and we note which parts were AI-assisted in the report.

# 3. Objectives

**Short-term (this course).**

1. Build a working multi-instance registration service with a shared database and a load balancer.
2. Demonstrate and then eliminate over-enrollment using concurrency control.
3. Measure throughput and p95 latency as instances scale from 1 to N, and identify the bottleneck.
4. Compare at least two seat-allocation strategies and a caching/queueing option with Redis.
5. Provide dashboards and structured logs that make failures diagnosable.
6. Evaluate a two-region deployment (primary region plus a remote region) to measure the latency and consistency cost of serving campuses in different time zones.

**Design principle: CP for writes, AP-leaning for reads.** We choose consistency per operation, not one setting for the whole system. *Registering or dropping a seat is CP*: a single writer (PostgreSQL, Multi-AZ with synchronous replication) guarantees zero over-enrollment, and during a partition or a primary-region outage the system rejects or queues writes instead of risking an oversold section. We stay highly available against likely failures (an app instance, a Redis node, or an availability zone) because the standby takes over without losing committed writes. *Course search, seat-count display, and waitlist views are AP-leaning*: they are served from Redis and read replicas, so they stay fast and available during failures at the cost of brief staleness. We do not claim to have both properties during a partition: for seat writes we explicitly choose consistency.

**Long-term vision.** A reference architecture and benchmark kit that a university could use to size and stress-test its registration system *before* registration day. Concrete future work: (a) partition/shard seat counts by course to remove the single-row hot spot; (b) a waitlist and priority-window system (seniors first), with student notifications (seat granted, waitlist movement) sent through Amazon SNS; (c) a full active-active multi-region deployment with regional write capability and explicit consistency tradeoffs (beyond the primary-plus-replica setup we test), plus per-campus registration windows in local time zones; (d) autoscaling driven by the same metrics we collect; (e) SLO tracking and burn-rate alerting built on the alerts we define now (for example, "p95 < 500 ms, zero over-enrollment"); (f) replaying anonymized real registration traces.

**Observability, reliability, and cost control for deployers.** Observability is a core objective: dashboards for the four golden signals (latency, traffic, errors, saturation), an invariant checker that continuously verifies `enrolled <= capacity` for every section, and runbook-style notes on what each alert means. The system has no AI in production, so there is no model-cost or nondeterminism risk for deployers. Where AI helped build it, correctness is guaranteed by the test suite and invariant checker, not by trust in the tool. Infrastructure cost is controlled by documenting the capacity measured per instance, so operators can choose the smallest fleet that meets the target. For our own AWS usage we set an AWS Budgets alarm, use small instance types, run experiments in short windows, and tear down the environment (via infrastructure-as-code) when not testing.

# 4. Related Work

- **Course readings.** *Dynamo: Amazon's Highly Available Key-value Store* (DeCandia et al.) motivates our consistency vs. availability tradeoffs, and ties to our DynamoDB homework. **[FILL IN: confirm and add other course readings, e.g., on CAP theorem, consensus, consistency models, caching, and observability.]** Brewer's CAP theorem frames our choice. Dynamo is an AP design, which suits our read paths (course search, seat-count display), while seat writes need a CP design, so we use a single consistent writer. Real systems mix both per operation, and some are tunable (e.g., DynamoDB strongly vs. eventually consistent reads), so we pick per operation.
- **Concurrency control.** Classic work on optimistic vs. pessimistic locking and serializable isolation (Bernstein & Goodman; Berenson et al., "A Critique of ANSI SQL Isolation Levels") underlies our row-lock vs. conditional-update comparison.
- **Hot spots and ticketing systems.** Ticket sales, flash sales, and airline booking face the same "many buyers, few items" contention; common solutions include atomic counters, queues, and virtual waiting rooms. We apply these ideas to registration.
- **Geo-distribution.** Google Spanner (Corbett et al.), Amazon Aurora Global Database, and DynamoDB Global Tables show the main options for multi-region data: a single write region with read replicas, or multi-writer replication with conflict resolution or consensus. The PACELC tradeoff (Abadi) frames our question: even without failures, lower latency costs consistency. Seat counts need a single writer, so we test that design and discuss what active-active would require.
- **Caching and Redis.** Redis atomic operations (`DECR`, Lua scripts) are widely used for inventory counters and rate limiting. We evaluate their tradeoffs against database-only designs.
- **Scalability.** Amdahl's law and the Universal Scalability Law (Gunther) explain why adding servers does not scale linearly when a shared resource is serialized, which is our central hypothesis.
- **Observability.** Google SRE's "four golden signals" and the Prometheus/Grafana ecosystem guide our metrics design.
- **Other teams.** **[FILL IN: any classmate projects on load balancing, caching, or observability. Reach out and cite them.]**

Our contribution is a small, reproducible, end-to-end experiment on one concrete domain, which measures where scaling stops and shows what each fix costs.

# 5. Methodology

**Tech stack.** Go (application service), PostgreSQL (source of truth), Redis (admission/caching), Docker, and AWS as the cloud provider.

**Architecture.** Clients (load generator running on a separate EC2 instance in the same region) → AWS Application Load Balancer → N stateless Go application instances on EC2 (containerized) → shared **Amazon RDS for PostgreSQL** as the source of truth, with **Amazon ElastiCache for Redis** for caching and admission control. Prometheus and Grafana run on a separate EC2 instance, and CloudWatch collects managed-service metrics. **Multi-region extension.** Real universities have campuses in several locations and time zones. We deploy a second region containing a cross-region RDS Postgres read replica, its own application instances behind an ALB, and a regional Redis. Writes (seat-changing registrations) are forwarded to the single primary region, so the seat invariant still has one writer. Reads (course search, seat counts) can be served locally, with possibly stale data. Remote-region clients are generated from EC2 instances in the second region. Main experiments E1–E6 run in a single region so that cross-region latency does not distort them. The same containers run locally with Docker Compose for development, and the AWS environment is defined as infrastructure-as-code (e.g., Terraform or CloudFormation) so it can be created, scaled to N instances, and torn down reproducibly. Instance types, RDS size, and region are fixed and recorded for every experiment.

![System architecture: primary region (ALB, Go app instances, Redis, Postgres primary with Multi-AZ standby), second region (replica and remote clients), and the observability and alerting stack.](architecture.png)

**Consistency model per operation.**

| Operation | Choice | Mechanism |
|---|---|---|
| Register / drop a seat | CP | Single-writer Postgres (RDS Multi-AZ, synchronous standby); Redis is only an admission filter and never the source of truth |
| Seat-count and course-search reads | AP-leaning | Redis cache and read replicas; bounded staleness measured in E6/E7 |
| Waitlist view | AP-leaning | Replica or cache read; ordering is decided by the primary |

**Data model.** `courses/sections(id, capacity, enrolled)`, `students(id)`, `enrollments(student_id, section_id)` with a unique constraint on the pair to prevent duplicates.

**Where we use AWS messaging.** *SNS* is used for alert delivery (core). *SQS* is an optional stretch variant for E4: the API accepts a registration, places it on a queue, and workers apply it to Postgres at a steady rate, which acts as a virtual waiting room. Because standard SQS delivers at least once and out of order, workers are idempotent (the unique `(student_id, section_id)` constraint plus the atomic seat update), and the client polls for the result. We only do this if the core experiments are finished. SNS student notifications are future work.

**Seat-allocation strategies (the main variable).**

1. *Naive read-then-write* (no concurrency control). It is expected to over-enroll and serves as the failure baseline.
2. *Pessimistic locking* (`SELECT ... FOR UPDATE`).
3. *Atomic conditional update* (`UPDATE ... SET enrolled = enrolled + 1 WHERE enrolled < capacity`).
4. *Redis atomic counter/queue in front of the DB* (fast admission, DB confirms asynchronously).

**How AI is used.** AI assists with generating scaffolding, tests, and scripts, and with summarizing results. It is *not* in the runtime path. Every AI-written concurrency or SQL change is reviewed by a teammate and must pass the over-enrollment invariant test.

**Observability support.** Prometheus metrics per instance (latency histogram, requests by status, conflicts/retries, pool usage, queue depth); DB metrics (active connections, lock waits, CPU); host/container CPU and memory; structured JSON logs with request IDs; a Grafana dashboard per experiment; and an invariant checker that queries for any section where `enrolled > capacity`.

**Alerting.** Alarms are defined for each signal and delivered by an Amazon SNS topic (email, optionally a chat webhook). CloudWatch Alarms cover managed-service metrics, Grafana alerting covers our application metrics, and AWS Budgets covers cost.

| Alert | Source | Severity |
|---|---|---|
| Over-enrollment detected (`enrolled > capacity`) | Invariant checker | Critical |
| p95 latency above target for 5 minutes | Grafana / ALB metrics | High |
| 5xx or registration error rate above 1% | ALB / Grafana | High |
| RDS replica lag above threshold (E7) | CloudWatch | Medium |
| RDS CPU or connections near limit | CloudWatch | Medium |
| Redis memory or evictions high | CloudWatch | Medium |
| AWS spend passes budget thresholds | AWS Budgets | Medium |

We also use these alerts during experiments to check that they fire when we inject failures (E5, E7).

**Experiments and tradeoffs.**

| # | Experiment | Variable | Measures | Tradeoff evaluated |
|---|---|---|---|---|
| E1 | Correctness under contention | Strategy 1–4; 1,000 users vs. 10 seats | Over-enrollment count, duplicates | Correctness vs. simplicity |
| E2 | Horizontal scaling | 1, 2, 4, 8 app instances | Throughput, p95/p99, error rate | Scalability vs. DB bottleneck |
| E3 | Strategy performance | Strategy 2 vs. 3 vs. 4 | Throughput, latency, lock waits | Consistency vs. latency/throughput |
| E4 | Traffic spike | Step load from baseline to 10×; stretch variant: direct writes vs. an Amazon SQS queue in front of the database workers | Latency recovery, errors, queueing, time-to-result | Availability vs. latency under overload; instant answer vs. protecting the database |
| E5 | Failure injection | Kill an app instance; kill Redis; force an RDS Multi-AZ failover | Error spike, recovery time, invariant held? | Fault tolerance vs. consistency (CAP) |
| E6 | Caching of reads | Seat-count/course-search reads with and without cache | Read latency, staleness | Freshness vs. speed |
| E7 | Multi-region | Clients in primary vs. remote region; writes forwarded vs. local reads from replica; synchronous vs. asynchronous cross-region replication (if feasible); primary-region outage | Cross-region latency, replica lag, stale-read rate, failover time, invariant held? | Latency vs. consistency (PACELC); availability vs. single-writer safety |

Each run is repeated at least 3 times and reported as median with variance. Instance types, versions, and configuration are recorded so results are reproducible. Because cloud performance can vary between runs (noisy neighbors, burstable CPU credits), we use non-burstable instance types where affordable and repeat runs at different times.

# 6. Preliminary Results

> **Honest status: [FILL IN].** Replace the "Status" cells with real results as soon as you have them. Even one early run (E1 on the naive baseline) is valuable evidence for this section.

| Experiment | Status | Preliminary result | Remaining |
|---|---|---|---|
| E1 Correctness | **[FILL IN]** | **[e.g., naive strategy over-enrolled X of 10 seats with 1,000 requests]** | Run strategies 2–4 and compare |
| E2 Scaling | **[FILL IN]** | **[none yet / single-instance baseline: X req/s, p95 Y ms]** | 2, 4, 8 instances |
| E3 Strategy performance | **[FILL IN]** | — | All strategies at equal load |
| E4 Spike | **[FILL IN]** | — | Step-load script and dashboards |
| E5 Failure injection | **[FILL IN]** | — | Kill scripts and recovery measurement |
| E6 Read caching | **[FILL IN]** | — | Cache layer and staleness checks |
| E7 Multi-region | **[FILL IN]** | — | Second-region deployment, replica-lag and failover measurements |

**Hypotheses to confirm or refute** (clearly labeled as expectations, not results): the naive strategy will over-enroll; throughput will rise with instance count and then flatten once the database (row locks or connection pool) saturates; Redis admission will raise throughput but adds a consistency window to handle.

**Base-case workload.** A realistic mix: many students browsing and searching courses (read-heavy, about 90% reads), spread across many sections, with registrations arriving at a moderate rate. This is the "normal day" and should reveal little contention.

**Pathological worst case.** Registration-opening moment: thousands of students simultaneously attempt to register for the **same few popular sections with very few seats** (for example, 5,000 requests for 3 sections with 30 seats), plus retries and refresh-spamming. This is the worst case because every request contends for the *same row*, serializing writes, so extra servers add load without adding throughput, lock waits and connection pools saturate, retries amplify the load, and the risk of over-enrollment is highest. A second pathological case is a failure *during* the spike (an instance dies mid-burst). A third, for the multi-region setup, is a spike in which most clients are in the remote region: every write pays the cross-region round trip while holding a row lock, which stretches lock hold time and cuts throughput on hot sections.

# 7. Impact

Registration meltdowns are a familiar, high-stakes problem for every student. Our results show **where and why** scaling fails and how much each fix costs, using a domain the whole class understands. The benchmark kit (Docker Compose, AWS infrastructure code, load scripts, dashboards) will be documented so others can reproduce our numbers or plug in their own design. Classmates can help by running the load generator against our deployed instance or by using our workload scripts to test their own services. We will offer a short demo/README and invite classmates to try to break the seat limit. **[FILL IN: confirm if you will host a public test endpoint for the class.]**

# 8. Team Expectations

- **Time:** each member commits to about **10–12 hours per week** **[FILL IN: confirm]**, including meetings, coding, and testing.
- **Deliverables:** work is tracked as issues/tasks with owners and due dates. Pull requests are small and are reviewed by another member within 24 hours.
- **Unforeseen circumstances** (illness, interviews, emergencies): tell the team as soon as you know, ideally 48 hours ahead for planned absences (e.g., interviews). The team reassigns or reschedules tasks, and the absent member picks up equivalent work afterward. Critical-path tasks always have a named backup.
- **Accountability:** a short weekly check-in lists what each person did and plans to do. Progress is visible through commits and pull requests. Contribution is reviewed at each milestone, and the final report lists who did what.

# 9. Team Communication

- **Platform:** private group chat (**[FILL IN: e.g., WhatsApp/Discord/Slack]**), not shared with course staff. Code and issues live in a shared GitHub repository.
- **Response time:** acknowledge messages within **12 hours** (within 2 hours for anything marked urgent in the 48 hours before a deadline).
- **Synchronization:** one **weekly video meeting** of about 45 minutes **[FILL IN: day/time]**, with an additional in-person working session at campus when possible. Short async updates in chat on other days. Decisions made in meetings are posted in chat afterward.

# 10. Team Responsibilities & Conflict Resolution

**Task division.** See Section 2: Suyash owns the application service and correctness; Santosh owns infrastructure, load generation, and failure injection; Sameep owns observability, caching, and analysis. All three share experiment runs, the final report, and the demo, and each member writes the report sections for their own area.

**How we address teamwork issues.** Raise concerns early and directly, focusing on the work and not the person. First, discuss it in the weekly meeting or a one-on-one call. If unresolved, agree on a written action item with an owner and a date. Technical disagreements are settled by a quick experiment or measurement where possible, or otherwise by majority vote.

**If a member is contributing too little.** (1) A private, friendly check-in to find out whether something is blocking them. (2) If it continues for a week, a team conversation resets expectations with specific tasks and deadlines. (3) If tasks are still missed after another week, the team redistributes the work and documents each member's contribution.

**When we escalate to the instructor.** If the problem persists after steps 1–3 (about two weeks), if it threatens the final deliverable, or if any member feels the team cannot resolve a conflict, we contact the instructor, with documentation of the attempts we made, no later than **one week before the final deadline**.
