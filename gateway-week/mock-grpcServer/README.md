# nafmock — mock NAF/TFS platform

A standalone gRPC server that pretends to be the existing NAF/TFS platform for
gateway development and demos. You point the gateway's `Executor` adapter at it.

Two things make it more than a stub:

1. **Fault injection with a small grammar** — make the downstream fail
   retryably, non-retryably, slowly, or ambiguously, per request.
2. **Ground-truth counters** — the mock counts how many times it *actually
   executed* the operation. When your gateway's idempotency works, duplicates
   show 1 execution here; when it is broken, they show 2. This is the evidence
   for "duplicate requests execute only once".

## Setup (this machine)

Go, protoc, the protoc-gen plugins and grpcurl are already installed
user-local. One-time per terminal session (or add to `~/.bashrc` — it may
already be there):

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH
```

Verify:

```bash
go version          # go1.27.x
protoc --version    # libprotoc 29.x
grpcurl --version
```

## Run

```bash
cd ~/platform_engineering_skills/gateway-week/mock-grpcServer

go test ./...                          # 1. prove it works first
go build -o bin/nafmock ./cmd/nafmock  # 2. build (make build also works if make is installed)
./bin/nafmock                          # 3. run in the foreground, Ctrl+C to stop
```

Or with options (defaults shown):

```bash
./bin/nafmock --grpc-addr :50052 --http-addr :9091 --behavior ok
```

| Flag | Default | Meaning |
| ---- | ------- | ------- |
| `--grpc-addr` | `:50052` | gRPC listen address |
| `--http-addr` | `:9091` | Admin HTTP (see below) |
| `--behavior` | `ok` | Default behavior for every operation |

Startup log lines confirm both listeners; SIGINT/SIGTERM shut it down
gracefully (exit 0). Leave it running in one terminal while you poke it
from another — or detach it: `setsid nohup ./bin/nafmock > /tmp/nafmock.log 2>&1 < /dev/null &`
(stop with `pkill -f 'bin/nafmoc[k]'` — the odd bracket stops the pattern
matching the pkill command itself).

## Verify it is alive (30 seconds)

```bash
curl -s localhost:9091/healthz          # -> ok
grpcurl -plaintext localhost:50052 list # -> nafmock.v1.NAFMock (reflection works)
curl -s localhost:9091/history           # -> [] (no calls yet)
```

## Behavior grammar

Behavior is a comma-separated token list, evaluated in order. Set per-request
via gRPC metadata `nafmock-behavior`, or globally via flag / admin API.

| Token | Effect | Use it to demo |
| ----- | ------ | -------------- |
| `ok` | succeed immediately (no-op token, composable) | happy path |
| `flaky:N` | first N calls fail `UNAVAILABLE`, then succeed (counted per operation + payload hash) | retry loop: 3 attempts, 1 execution |
| `fail-retryable` | always `UNAVAILABLE` | retries exhausted |
| `fail-nonretryable` | always `FAILED_PRECONDITION` | single attempt, FAILED |
| `delay:D` | wait `D` first (e.g. `delay:500ms,ok`), ctx-aware | deadline propagation |
| `hang` | sleep until the caller's context dies (`DEADLINE_EXCEEDED`) | timeout with no side effect |
| `succeed-then-hang` | **apply the side effect, count it**, then hang | the in-doubt case: gateway believes it failed, platform actually completed |

Retryable errors are `UNAVAILABLE`; non-retryable are `FAILED_PRECONDITION` —
matching the classification your gateway's `Classify` should implement.

## Admin HTTP (:9091)

| Endpoint | What |
| -------- | ---- |
| `GET /healthz` | liveness |
| `GET /metrics` | Prometheus |
| `GET /history` | JSON list of recent calls: time, correlation_id, operation, behavior, code, executed |
| `GET /behavior` / `PUT /behavior` (raw string body) | read/change default behavior at runtime |
| `POST /reset` | clear counters, history, flaky state (between demo runs) |

Metrics: `nafmock_requests_total{code,operation}`, `nafmock_executions_total{operation}`
(the ground-truth counter), `nafmock_request_duration_seconds`.

## Manual smoke test (grpcurl)

```bash
grpcurl -plaintext -d '{"operation":"naf.provision-link","payload":"eyJhIjoxfQ=="}' \
  -H 'x-correlation-id: demo-1' localhost:50052 nafmock.v1.NAFMock/ApplyOperation

grpcurl -plaintext -H 'nafmock-behavior: flaky:2' \
  -d '{"operation":"naf.provision-link","payload":"eyJhIjoxfQ=="}' \
  localhost:50052 nafmock.v1.NAFMock/ApplyOperation
# twice more with the same payload -> third call succeeds (flaky counter is
# per operation+payload: 2 failures, then ok, handle returned)

curl -s localhost:9091/history          # every call recorded with code + executed
curl -s localhost:9091/metrics | grep '^nafmock_'

# change the default behavior at runtime (no restart):
curl -s -X PUT -d 'delay:2s,ok' localhost:9091/behavior && curl -s localhost:9091/behavior
# ... run demos ...
curl -s -X POST localhost:9091/reset    # clean slate between demos
```

(`jq` is optional; plain `curl` output is fine. `jq` not installed? skip it.)

## How this plugs into gateway-week

- **Wednesday (executor adapter):** `TFSNAFExecutor` (or a
  `NAFMockExecutor` variant) dials this server and forwards
  `ApplyOperation(operation, payload)` under the request ctx. No metadata
  needed — the default behavior drives the scenario.
- **Duplicate-once demo:** PUT `/behavior` to `ok`, fire two identical
  gateway requests concurrently, then check `nafmock_executions_total` == 1
  here and `gateway_duplicate_requests_total` == 1 in the gateway. Two
  independent systems telling the same story.
- **Timeout demo:** PUT `/behavior` to `delay:2s,ok`, call the gateway with a
  200ms deadline. Gateway returns DEADLINE_EXCEEDED; mock shows
  `requests_total{code="DeadlineExceeded"}` and zero executions.
- **In-doubt demo (the good one):** PUT `/behavior` to `succeed-then-hang`.
  The gateway's call dies at its deadline, but this mock's history records
  `executed: true` — the platform DID the work while the gateway believes it
  failed. That gap is exactly what design decision D1 (in-doubt policy)
  exists for. Show this in the Friday review.

## Contract

`api/nafmock/v1/nafmock.proto`: `NAFMock.ApplyOperation(ApplyOperationRequest)
ApplyOperationResponse`. Deliberately generic (`operation` string + opaque
`payload` bytes): when you get access to the real NAF/TFS contract at work,
rewrite the adapter, not the gateway.

Python twin: **built and tested** at `../mock-grpcServer-py/` — identical
contracts, so every command in this README works against either. The build
spec that produced it is `../python/06-build-nafmock-python.md`, and its
README has a "Go vs Python notes" section worth reading.

## Tests

```bash
go test ./...    # (make test also works, if make is installed)
```

Behavior matrix (ok, flaky, fail-retryable, fail-nonretryable, delay+deadline,
hang, succeed-then-hang, metadata override), flaky keying by payload,
correlation-ID recording, admin behavior changes, invalid-input rejection —
all via bufconn against the real gRPC machinery.
