# internal/api

Owner: Suyash.

HTTP handlers for the registration API. Planned endpoints:

| Method | Path | Purpose |
|---|---|---|
| POST | `/register` | Claim a seat (`student_id`, `section_id`) |
| POST | `/drop` | Release a seat |
| GET | `/sections/{id}` | Seat count and capacity (cache-friendly read) |
| GET | `/healthz` | Health check (already in `cmd/server`) |
