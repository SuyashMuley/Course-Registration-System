# E1: Correctness under contention

Owner: Santosh.

Goal: show that the naive strategy over-enrolls and the fixed strategies do not.

Setup: 1,000 students (one k6 virtual user each) send one register request each, all at once, for section 1, which has 10 seats. Run once per strategy, 3 times each.

Measure: real enrollment rows vs. capacity (over-enrollment), whether the `enrolled` counter matches the rows, and outcome counts (enrolled, full, errors). Duplicate enrollments cannot happen because `(student_id, section_id)` is the primary key; a duplicate attempt is answered `409 already_enrolled`.

## Files

| File | Purpose |
|---|---|
| `seed.sql` | 1,000 students and section 1 (capacity 10). Loaded automatically by Docker Compose on first start. |
| `reset.sql` | Empties section 1. Run before every E1 run. |
| `e1.js` | The k6 script. |
| `check.sql` | Counts enrollment rows vs. capacity after a run. |

## Prerequisites

- Docker Desktop
- k6 (`brew install k6`), or use the k6 container (see the end of this file)
- macOS: raise the open-file limit in the terminal that runs k6: `ulimit -n 10000`

## Run (local, from the repository root)

```sh
DC="docker compose -f deploy/docker/docker-compose.yml"
PSQL="$DC exec -T postgres psql -U registration -d registration"
mkdir -p loadtest/e1-correctness/results/raw

# 1. Start Postgres and the app with the strategy under test (atomic or naive)
export STRATEGY=atomic
$DC up -d --build

# 2. Reset section 1
$PSQL < loadtest/e1-correctness/reset.sql

# 3. Run the burst (RUN = 1, 2, 3)
RUN=1
k6 run loadtest/e1-correctness/e1.js | tee loadtest/e1-correctness/results/raw/$STRATEGY-run$RUN.txt

# 4. Check the database
$PSQL < loadtest/e1-correctness/check.sql
```

Repeat steps 2–4 for runs 2 and 3. To switch strategy, set `export STRATEGY=naive` and run `$DC up -d app` (this restarts the app with the new strategy), then repeat steps 2–4.

## Expected results

| Strategy | `enrollment_rows` | k6 `reg_full` | k6 exit code |
|---|---|---|---|
| atomic | exactly 10 | 990 | 0 |
| naive | more than 10 (over-enrolled) | fewer than 990 | 0, unless the server returned errors |

The k6 run fails (non-zero exit) if any request gets a 5xx, a 404, or no answer, because that would make the counts unreliable.

## Recording results

Raw output in `results/raw/` is git-ignored. Add one line per run to `results/e1_summary.csv` (committed):

```
strategy,run,students,capacity,enrollment_rows,enrolled_counter,over_enrolled,reg_full,reg_other_errors
```

Take `enrollment_rows`, `enrolled_counter` and `over_enrolled` from `check.sql`, and `reg_full` and `reg_other_errors` from the k6 summary.

## Without installing k6

```sh
$DC --profile loadtest run --rm k6
```

This runs the same script in a container on the Compose network. It passes `STRATEGY` from your shell. The output is printed to the terminal.
