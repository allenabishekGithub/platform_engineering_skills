# 07 — Observability

> Python reader: prometheus_client + stdlib JSON logging equivalent is
> `python/03-observability.md` — including the Counter `_total` naming trap
> that breaks metric parity.

Three pillars for this week: **structured logs**, **metrics**, **correlation
IDs**. (Traces would be the fourth — out of scope, noted as future work.)

## Correlation IDs

Every request gets a unique ID:

1. Interceptor checks incoming gRPC metadata for `x-correlation-id`; if absent,
   generate one (UUID).
2. Put it in the context (this is a legitimate request-scoped ctx value).
3. Every log line anywhere in that request's handling includes it.
4. Echo it back to the client via `grpc.SetHeader` so the caller can quote it.

The payoff, and Thursday's acceptance test: given any log line, you can find
every other line from the same request. Without this, multi-component debugging
is archaeology.

## Structured logs with slog

Use `log/slog` with a JSON handler. Log events, not sentences:

```json
{"time":"...","level":"INFO","msg":"operation executing",
 "correlation_id":"...","operation_id":"...","attempt":2}
```

Conventions:

- One line per event, `msg` short and stable (it is a key, not prose).
- Fields: correlation_id, operation_id, state, attempt, error/reason.
- Levels: INFO for lifecycle, WARN for retryable failures, ERROR for terminal failures.
- Get the slog logger from ctx (a common pattern: middleware stores the
  request-scoped logger in the context) so every component logs identically.

## Prometheus metrics

Use `github.com/prometheus/client_golang/prometheus` +
`promhttp`. Metric types and what belongs in each:

| Type       | Semantics                  | This week                                      |
| ---------- | -------------------------- | ---------------------------------------------- |
| Counter    | Monotonic total           | requests, executions, retries, duplicates      |
| Gauge      | Current value             | operations in flight, per-state counts        |
| Histogram  | Distribution of durations | executor latency, total request latency       |

Suggested set (labels in brackets):

```
gateway_requests_total{code}              counter
gateway_operations_total{state}           counter  (terminal states)
gateway_operations_in_flight{state}       gauge
gateway_executor_attempts_total{outcome} counter
gateway_retries_total                    counter
gateway_duplicate_requests_total         counter
gateway_request_duration_seconds          histogram
gateway_executor_duration_seconds        histogram
```

Rules that keep metrics useful:

- Label cardinality stays low: gRPC codes, states — never payload content or
  raw error strings.
- Count the things your demos must prove: duplicates seen, attempts made,
  in-flight. Friday's evidence is "read the metric, show the number".

Serve on `:9090`: `http.Handle("/metrics", promhttp.Handler())` plus a trivial
`/healthz` (HTTP-level liveness for demos; the real gRPC health check is the
grpc health service from concepts/06).

## Health, concretely

- gRPC standard health (`grpc.health.v1.Health`) — set SERVING after startup
  recovery completes, not before.
- `/healthz` on the HTTP port — process-is-alive check for demos.

## Logs + metrics tell one story

The timeout demo proves the point: the log shows attempt 1 begin and the
ctx-deadline abort; the metrics show `gateway_executor_attempts_total{outcome="deadline"}`
and no retry after it. Both views must agree — when they do not, you have found
a bug, and that is exactly the debugging skill this week trains.
