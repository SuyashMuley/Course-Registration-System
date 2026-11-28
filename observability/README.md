# observability

Owner: Sameep.

Prometheus config, Grafana dashboards (one per experiment), alert definitions, and the invariant checker that alerts if any section has enrolled > capacity. Alerts are delivered through an SNS topic.

## Metrics exposed by the service

`GET /metrics` on every app instance (Prometheus text format). Code: `internal/metrics/`.

| Metric | Type | Labels | Use |
|---|---|---|---|
| `crs_http_requests_total` | counter | `route`, `method`, `code` | Throughput and error rate per endpoint. `route` is the router pattern (`/sections/{id}`), never the raw path. |
| `crs_http_request_duration_seconds` | histogram | `route`, `method` | p50/p95/p99 latency |
| `crs_http_requests_in_flight` | gauge | | Concurrency per instance |
| `crs_seat_operations_total` | counter | `strategy`, `op`, `outcome` | Outcomes: `ok`, `full`, `already_enrolled`, `not_enrolled`, `not_found`, `canceled`, `error`. Seat conflicts = `full` + `already_enrolled`. |
| `crs_seat_operation_duration_seconds` | histogram | `strategy`, `op` | Database time per strategy (E3) |
| `crs_db_pool_{max,total,acquired,idle,constructing}_conns` | gauge | | Connection pool usage |
| `crs_db_pool_empty_acquires_total` | counter | | Acquires that waited for a connection: rises when the pool is the bottleneck (E2) |
| `crs_db_pool_acquire_wait_seconds_total` | counter | | Total wait for a connection |
| `crs_db_pool_{acquires,canceled_acquires}_total` | counter | | |

Plus Go runtime (`go_*`) and process (`process_*`) metrics. Prometheus adds the `instance` label.

Useful queries:

```promql
sum by (route) (rate(crs_http_requests_total[1m]))                                      # requests/s
histogram_quantile(0.99, sum by (le, route) (rate(crs_http_request_duration_seconds_bucket[1m])))  # p99
sum by (outcome) (rate(crs_seat_operations_total{op="register"}[1m]))                  # register outcomes/s
rate(crs_db_pool_acquire_wait_seconds_total[1m]) / rate(crs_db_pool_acquires_total[1m]) # mean pool wait
```
