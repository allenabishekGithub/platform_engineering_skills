# 05 (Python) — Build devreg (practice server), Python edition

Same project as `teaching/grpc-fundamentals/04-build-your-own-server.md` —
read that first for the goals and acceptance criteria; this file changes only
the how. Spec, not code: you write everything. **Pick one language for devreg
(Go or Python), do not build both.**

## Step 0 — environment

```bash
python3 --version                 # 3.10+ fine
python3 -m venv .venv && source .venv/bin/activate
pip install grpcio grpcio-tools grpcio-health-checking grpcio-reflection pytest
```

## Step 1 — module and folders

```
gateway-week/devreg/            (Python does not need go.mod; you still need order)
  api/devreg/v1/devreg.proto
  devreg/__init__.py
  devreg/server.py               servicer + in-memory registry
  devreg/main.py                wiring
  tests/test_server.py
```

## Step 2 — write the contract

Identical `.proto` to the Go version (see teaching/04 step 2 — same spec:
`RegisterRequest`/`RegisterResponse`, `GetRequest`/`GetResponse`, service
`DeviceRegistry`). One contract, two languages is the point of gRPC — reuse
the exact same field numbers.

## Step 3 — generate and READ the output

```bash
python -m grpc_tools.protoc -I api \
    --python_out=api --grpc_python_out=api api/devreg/v1/devreg.proto
```

Find in `devreg_pb2.py`: nothing readable (serialized descriptor) — Python
hides more than Go; in `devreg_pb2_grpc.py`: class `DeviceRegistryServicer`
(the interface you inherit), `add_DeviceRegistryServicer_to_server`, and
`DeviceRegistryStub` (the client). Handle the import wart with the sys.path
fix from `02-grpc-essentials.md` — write a tiny `devreg/api.py` that does the
path insert and re-exports the generated modules, so the rest of the code
just does `from devreg.api import devreg_pb2, devreg_pb2_grpc`.

## Step 4 — implement the servicer (`devreg/server.py`)

- `class DeviceRegistry(devreg_pb2_grpc.DeviceRegistryServicer)`
- `self._devices = {}` guarded by `threading.Lock` (`with self._lock:`)
- `Register(self, request, context)`:
  - empty id/model → `context.abort(grpc.StatusCode.INVALID_ARGUMENT, ...)`
  - existing id → `context.abort(grpc.StatusCode.ALREADY_EXISTS, ...)`
  - store, return `devreg_pb2.RegisterResponse(device_id=..., registered_at=<UTC ISO8601>)`
- `Get`: unknown → `context.abort(grpc.StatusCode.NOT_FOUND, ...)`; known →
  `devreg_pb2.GetResponse(device_id=..., model=..., registered=True)`
- `context.abort` raises — so code after it is unreachable; order your checks.

## Step 5 — wire the server (`devreg/main.py`)

From `02-grpc-essentials.md`: `grpc.server(ThreadPoolExecutor(max_workers=10))`,
register servicer + health servicer + reflection, `add_insecure_port` with
the ports from CLI flags (`argparse`: `--grpc-addr`, `--http-addr`), plus a
`ThreadingHTTPServer` serving `/healthz`. Graceful stop: `signal.signal(SIGTERM, handler)`
where the handler calls `server.stop(grace=5)` and shuts the HTTP server —
Python's shape of `GracefulStop()`.

## Step 6 — build, run, prove it (identical commands to the Go version!)

```bash
python devreg/main.py --grpc-addr 50053 --http-addr 9092

grpcurl -plaintext localhost:50053 list                                  # reflection
grpcurl -plaintext -d '{"device_id":"dev-1","model":"X100"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Register                      # ok
# ...duplicate -> AlreadyExists, empty id -> InvalidArgument, Get 404 -> NotFound
```

grpcurl cannot tell which language the server is written in — that IS the
demonstration of contract-first design. SIGTERM → clean exit 0.

## Step 7 — the deadline experiment

Handler: at the top of `Register`, wait ~2s using the polling pattern from
`python/01` (`while context.is_active(): sleep(0.05)` with a 2s cap).

```bash
grpcurl -plaintext -max-time 1 -d '{...}' localhost:50053 .../Register
```

Then break it on purpose: replace the poll loop with a blind
`time.sleep(2)`. Compare server behavior with the Go version's blind-sleep
variant. Write both observations in `NOTES.md` — the Go and Python lessons
are the same sentence.

## Step 8 — tests (`tests/test_server.py`)

The ephemeral-port fixture from `02-grpc-essentials.md`, then:

- `@pytest.mark.parametrize` table for the five scenarios (register-ok,
  register-duplicate, register-invalid, get-ok, get-not-found) asserting
  `e.code()` from the raised `grpc.RpcError`
- the concurrency test: 10 threads, same id, `concurrent.futures` — exactly
  one `OK`, nine `ALREADY_EXISTS` (this is your Tuesday idempotency race,
  arriving early — the `Lock` is what saves you; note the parallel to the
  SQL CAS)

## Definition of done

Same checkboxes as teaching/04, plus:

- [ ] `NOTES.md` includes one paragraph: what surprised you differently
      from the Go version (suggested: the import wart, exception-based
      control flow, no `-race` safety net — what discipline replaces it?)

## Commit

```
teaching: devreg — grpc fundamentals practice (python)
```