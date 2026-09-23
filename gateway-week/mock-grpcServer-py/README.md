# nafmock-py — mock NAF/TFS platform (Python twin)

The Python twin of `../mock-grpcServer/` (Go). Same contract, same behavior
grammar, same admin API, same metric names — read **the Go README first**;
it documents the behavior table, the demo recipes, and how this plugs into
gateway-week. Everything there applies here unchanged.

## Why two twins

`python/06-build-nafmock-python.md` is the build spec this implements.
Keeping both proves the parity contract from `python/README.md`: a Go
gateway works against either mock, `grpcurl` commands are identical, and
`/history` + metrics outputs are interchangeable. Differences between the
implementations are the teaching material — see "Go vs Python notes" below.

## Setup

Python 3.10+ and pip (`pip install --user` is fine, no sudo needed):

```bash
python3 -m pip install --user -r requirements.txt
```

## Run

```bash
cd gateway-week/mock-grpcServer-py

python3 -m pytest tests/ -q          # 1. prove it works first
python3 -m nafmock.main             # 2. run (Ctrl+C to stop)
```

Same flags as the Go twin:

```bash
python3 -m nafmock.main --grpc-addr :50052 --http-addr :9091 --behavior ok
```

Side-by-side with the Go twin: run this one with `--grpc-addr :50062
--http-addr :9092` and run the same grpcurl sequence against both.

## Regenerate the contract

```bash
python3 -m grpc_tools.protoc -I . --python_out=. --grpc_python_out=. \
    api/nafmock/v1/nafmock.proto
```

The proto is copied byte-identical from `../mock-grpcServer/` — one
contract, two languages. Generated code lands in `api/nafmock/v1/` and
imports as `api.nafmock.v1` (regular package, no sys.path tricks needed
when run from this directory).

## Layout

```
api/nafmock/v1/       proto + generated pb2/pb2_grpc (never edit the .py)
nafmock/
  behavior.py         grammar parser (mirrors behavior.go)
  servicer.py         state, flaky counters, history, metrics (mirrors server.go)
  admin.py            /healthz /metrics /history /behavior /reset (mirrors admin.go)
  main.py             wiring: grpc server, health, reflection, signals
tests/test_behavior.py  behavior matrix, ported case-for-case from the Go tests
```

## Go vs Python notes (the teaching deltas)

- **Cancellation is polled, not selected**: Go handlers `select` on
  `ctx.Done()`; here `_wait_for` polls `context.is_active()` every 20ms.
  Same semantics, different mechanics — see `python/01`.
- **Errors are decisions, not exceptions**: `_apply` returns a status code
  and the single `context.abort` happens after history/metrics are recorded
  (Go records then returns `status.Error` — same ordering, same reason).
- **`prometheus_client` names counters without `_total`** and appends it in
  exposition — hence `Counter("nafmock_executions", ...)` to match Go's
  `nafmock_executions_total`.
- **Test harness races**: the handler thread can outlive the client call by
  ~50ms, so tests `wait_for_history` before asserting — a race the Go
  bufconn tests don't exhibit as visibly. Good thing to know for your
  gateway tests on Wednesday.
- **Label values are CamelCase** (`code="OK"`, `"DeadlineExceeded"`) to match
  grpc-go's `code.String()` exactly — check `_CAMEL` in servicer.py.

## Tests

13 cases ported 1:1 from the Go suite: the 8-row behavior matrix (including
`succeed-then-hang` = the in-doubt demo), flaky keying by payload,
correlation-ID recording, admin behavior changes, invalid-input rejection,
reset. Run with the command above.