# Day 6 (Thursday) — Structured logs, metrics, correlation IDs

**Read first:** concepts/07

**Goal:** the gateway becomes observable. Every lifecycle event leaves a
trace in logs and a number in metrics; any line is findable by correlation ID.

## Tasks

1. `internal/telemetry/logging.go`:
   - `slog` with JSON handler to stdout.
   - `LoggerFromContext(ctx)` / `WithLogger(ctx, logger)` — the interceptor
     creates the request logger (with correlation ID field pre-attached) and
     stores it in ctx; every component logs via it, so every line is tagged.
2. Correlation ID completeness (started day 3): ensure the ID now flows to
   store writes (log line on each transition), executor attempts, and back to
   the client in headers. One ID, everywhere.
3. `internal/telemetry/metrics.go` — register the concepts/07 set:
   ```
   gateway_requests_total{code}
   gateway_operations_total{state}
   gateway_operations_in_flight{state}   (gauge; inc/dec around EXECUTING)
   gateway_executor_attempts_total{outcome}
   gateway_retries_total
   gateway_duplicate_requests_total
   gateway_request_duration_seconds
   gateway_executor_duration_seconds
   ```
4. Wire metrics into the interceptor (requests, duration, code) and into the
   handler/executor path (attempts, retries, duplicates, in-flight, terminal
   states). Never inside the store — the store stays metric-free.
5. `/metrics` and `/healthz` on `:9090` (day 3 skeleton — now real).
6. End-to-end sanity script (manual, no test needed): run the server with a
   flaky executor, make 3 calls (success, duplicate, deadline), then:
   - `curl localhost:9090/metrics | grep gateway_`
   - capture 10 log lines
   - verify the same correlation ID appears across all lines of one request
     AND in the grpc response headers.

## Hints

- Interceptor metrics: capture start time, call handler, THEN observe code —
  `status.Code(err)` converts any handler error to the grpc code.
- Gauges with labels: `operations_in_flight{state="EXECUTING"}` — inc when
  entering EXECUTING, dec on leaving. Panics can leak gauges; a defer fixes it.
- Prometheus labels: codes and states only. If you feel the urge to label by
  error message — don't (cardinality explosion; it belongs in logs).
- slog: `logger.LogAttrs(ctx, slog.LevelInfo, "operation executing", ...)`
  passes ctx so the handler can be traced later — use the ctx-aware methods.

## Acceptance criteria

- [ ] Every log line in the request path carries correlation_id, and one ID
      spans intercept -> handler -> executor -> store
- [ ] Duplicate request visible as `gateway_duplicate_requests_total` +1 and
      distinct from executions
- [ ] After a deadline demo: `gateway_executor_attempts_total{outcome="deadline"}`
      and `gateway_retries_total` tell a consistent story
- [ ] `/metrics` exposes every family above; `/healthz` 200
- [ ] Paste 5-10 log lines + relevant metrics output into
      `gateway-week/gateway/docs/samples.md` (this is Friday evidence, capture it now)
- [ ] `go test -race ./...` green

## Commit

```
day6: correlation IDs end-to-end, slog JSON logging, Prometheus metrics
```
