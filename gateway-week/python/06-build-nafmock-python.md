# 06 (Python) — Build spec: nafmock in Python

> STATUS: **already built** from this spec at `../mock-grpcServer-py/`
> (13 tests green, parity verified). Use the code as reference reading —
> or re-derive it from this spec yourself; the spec stands on its own.

Full build spec for the Python twin of `mock-grpcServer/` (the Go mock is
built and tested; this is an **optional** build exercise — and a strong one,
because parity is machine-checked by the demo commands). You write
everything; this doc is the contract + the traps.

## Parity contract (non-negotiable, per python/README)

- same `api/nafmock/v1/nafmock.proto` — byte-for-byte reuse, regenerate with
  `python -m grpc_tools.protoc -I api --python_out=api --grpc_python_out=api`
- same behavior grammar: `ok`, `flaky:N`, `fail-retryable`,
  `fail-nonretryable`, `delay:D`, `hang`, `succeed-then-hang` (comma-separated)
- same gRPC codes: retryable=`UNAVAILABLE`, non-retryable=`FAILED_PRECONDITION`,
  ctx deadline=`DEADLINE_EXCEEDED`
- same admin HTTP: `GET /healthz /metrics /history /behavior`,
  `PUT /behavior`, `POST /reset`
- same metric names: `nafmock_requests_total{code,operation}`,
  `nafmock_executions_total{operation}` (remember the `_total` auto-suffix),
  `nafmock_request_duration_seconds`
- same flags/ports: `--grpc-addr :50052 --http-addr :9091 --behavior ok`

If you keep all of those, **every command and demo script in the Go README
works against your server unchanged** — that is the acceptance test.

## Suggested layout

```
gateway-week/mock-grpcServer-py/
  api/nafmock/v1/nafmock.proto        (copied from Go sibling)
  nafmock/
    api.py                sys.path fix + re-export generated modules
    behavior.py           grammar parser + dataclass
    servicer.py           state, flaky counters, history ring
    admin.py              ThreadingHTTPServer: /healthz /metrics /history /behavior /reset
    main.py               argparse, wiring, signals
  tests/test_behavior.py  parametrized behavior matrix
```

## Component specs

### behavior.py

- `@dataclass class Behavior` with the same fields as the Go struct
  (delay: `timedelta`, flaky_count, fail kind, hang, succeed_then_hang, raw).
- `parse_behavior(raw: str) -> Behavior` raising `ValueError` on unknown
  tokens, bad durations, `hang`+`succeed-then-hang` together, `ok` misuse.
  Port the Go validation one rule at a time — the Go file is your spec.
- Unit-test it separately, parametrized, ~10 cases. Parser first: it is the
  piece where a typo ruins every later demo.

### servicer.py

- State under one `threading.Lock`: `default_behavior`, `flaky_calls`
  dict keyed `operation + "/" + sha256(payload).hex()`, `history` as
  `collections.deque(maxlen=100)`.
- `ApplyOperation(self, request, context)`:
  1. read `x-correlation-id` and `nafmock-behavior` from
     `context.invocation_metadata()` (invalid override → `abort(INVALID_ARGUMENT)`),
  2. delay → use the cancellation-respecting wait from python/01 (this is
     what makes `hang` work at all: plain `time.sleep` ignores the caller),
  3. fail kinds → abort with the parity codes,
  4. flaky → increment-and-test under the lock (mirror the Go logic: calls
     `<= n` fail),
  5. `succeed-then-hang` → **increment executions counter FIRST**, then wait
     on `context.is_active()` until the caller's deadline kills the RPC,
  6. success → increment executions, `return` the response with a random
     `handle` (`naf-` + 8 hex bytes) and `applied_at` UTC ISO8601,
  7. every call appends a history entry
     `{time, correlation_id, operation, behavior, code, executed}` and bumps
     `nafmock_requests_total{code,operation}`.
- History/behavior/reset accessors for the admin server to call.

### admin.py

One `ThreadingHTTPServer` + `BaseHTTPRequestHandler`:

- `/metrics` → `generate_latest()` (see python/03 for why not
  `start_http_server`: one port must host everything)
- `/history` → `json.dumps(list(history))`
- `/behavior` GET → `{"behavior": raw}`; PUT → body text through
  `parse_behavior` (400 on `ValueError`), swap under lock
- `/reset` POST → clear flaky counters, history, and `.reset()` the
  prometheus collectors (Go twin calls `Reset()` too)
- `/healthz` → `ok`

### main.py

argparse flags, `grpc.server(ThreadPoolExecutor(max_workers=16))`, health +
reflection registration, admin server in a daemon thread, `wait_for_termination()`,
SIGINT/SIGTERM → `server.stop(grace=5)` + admin shutdown. Structured JSON
logging per python/03, correlation ID on every line.

## Test plan (`tests/test_behavior.py`)

Port the Go `TestBehaviorMatrix` case-for-case with the ephemeral-port
fixture (python/02) + `@pytest.mark.parametrize`:

| Case | calls | expected codes | executions |
| ---- | ----- | -------------- | ---------- |
| ok | 1 | OK | 1 |
| flaky:2 | 3 | UNAVAILABLE, UNAVAILABLE, OK | 1 |
| fail-retryable | 1 | UNAVAILABLE | 0 |
| fail-nonretryable | 1 | FAILED_PRECONDITION | 0 |
| delay:500ms,ok + 100ms timeout | 1 | DEADLINE_EXCEEDED | 0 |
| hang + 150ms timeout | 1 | DEADLINE_EXCEEDED | 0 |
| succeed-then-hang + 150ms timeout | 1 | DEADLINE_EXCEEDED | **1** |
| metadata override beats default | 1 | per override | — |

Plus: flaky keyed by payload (a/a/b/b pattern → 2 executions), correlation
ID lands in history, PUT /behavior changes behavior, invalid override →
INVALID_ARGUMENT, PUT invalid → 400.

The `succeed-then-hang` row is the one that matters most — it is the
in-doubt demo, and passing it in two languages is proof you understand it
in both.

## Cross-language validation (do this, it is the payoff)

Run both mocks side by side (Go on :50052, Python on :50062 to avoid the
clash — flag it in) and run the same grpcurl sequence against each. Same
inputs → same codes, same `/history` shape, same counter deltas. Any
divergence is either a parity bug or a genuine lesson — write whichever it
was into a short `PARITY.md`.

## Commit

```
tooling: nafmock python twin — behavior parity, admin API, tests
```