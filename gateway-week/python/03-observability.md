# 03 (Python) — Observability

Python equivalent of `concepts/07-observability.md`. Read that first: the
correlation-ID strategy, metric selection, and the "logs + metrics tell one
story" argument are identical. This file covers the Python tooling.

## Structured logging (stdlib only)

Go has `slog`; Python's stdlib needs a small JSON formatter to get the same:

```python
import logging, json

class JsonFormatter(logging.Formatter):
    def format(self, record):
        return json.dumps({
            "time": self.formatTime(record),
            "level": record.levelname,
            "msg": record.getMessage(),
            **getattr(record, "fields", {}),
        })

handler = logging.StreamHandler()
handler.setFormatter(JsonFormatter())
logging.basicConfig(level=logging.INFO, handlers=[handler])
log = logging.getLogger("nafmock")
```

Then a log call with fields — the Python shape of
`slog.InfoContext(ctx, "operation executing", "attempt", 2)`:

```python
log.info("operation executing", extra={"fields": {
    "correlation_id": cid, "operation_id": oid, "attempt": 2}})
```

One stable short `msg` + structured fields, same rules as the Go doc.
(`structlog` is the nicer library if you're allowed dependencies — same
output, less boilerplate.)

## Correlation IDs

Same contract: read `x-correlation-id` from `context.invocation_metadata()`,
generate if absent, log it on every line, echo it back with
`context.send_initial_metadata((("x-correlation-id", cid),))`. Acceptance
test is identical: pick any log line, find all its siblings by ID.

## Prometheus metrics with prometheus_client

```python
from prometheus_client import Counter, Gauge, Histogram

# NOTE: prometheus_client appends _total to Counter names in the output.
# Name them WITHOUT the suffix to get the exact Go metric names:
executions = Counter("nafmock_executions", "side effects applied", ["operation"])
requests   = Counter("nafmock_requests",   "requests received", ["code", "operation"])
inflight   = Gauge("gateway_operations_in_flight", "by state", ["state"])
duration   = Histogram("nafmock_request_duration_seconds", "latency")

executions.labels("naf.provision-link").inc()
requests.labels("OK", "naf.provision-link").inc()
duration.observe(0.042)
```

That naming gotcha is the #1 parity trap: if you name the Counter
`nafmock_executions_total` you expose `nafmock_executions_total_total` and
break the "identical metrics either language" contract. Label rules from
concepts/07 apply unchanged: codes and states only, never payloads or error
strings.

## Serving /metrics with the admin endpoints

`prometheus_client.start_http_server(9091)` is one line but it owns the port —
and your admin API needs `/history`, `/behavior`, `/reset` too. So serve
everything from one small stdlib HTTP server:

```python
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from prometheus_client import generate_latest

class Admin(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/metrics":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(generate_latest())
        elif self.path == "/healthz":
            self.send_response(200); self.end_headers(); self.wfile.write(b"ok")
        # /history -> json.dumps of your records, etc.
```

`generate_latest()` emits the standard exposition format — `curl` and
Prometheus cannot tell it apart from the Go server's output.

## Health

Two layers, same as Go: `/healthz` on the HTTP port (process-alive for
demos) and the gRPC health service from `02-grpc-essentials.md` for
orchestrators. Set SERVING after startup recovery, never before.

## The parity table (what your Python build must expose)

| Metric family | Labels | Go/Python identical |
| ------------- | ------ | ------------------- |
| `gateway_requests_total` | code | yes |
| `gateway_operations_total` | state | yes |
| `gateway_operations_in_flight` | state | yes |
| `gateway_executor_attempts_total` | outcome | yes |
| `gateway_retries_total` | — | yes |
| `gateway_duplicate_requests_total` | — | yes |
| `gateway_request_duration_seconds` | — | yes |

Friday's timeout demo works identically: the log shows attempt 1 aborted at
the deadline, the metric shows `attempts_total{outcome="deadline"}` and no
retry — two views, one story, any language.