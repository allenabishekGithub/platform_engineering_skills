# 02 (Python) — gRPC essentials

Python equivalent of `concepts/06-grpc-essentials.md`. Read that one first —
the lifecycle, metadata, and codes sections are identical (gRPC is gRPC).
This file covers what is different in Python.

## Setup

```bash
python3 -m venv .venv && source .venv/bin/activate
pip install grpcio grpcio-tools            # runtime + protoc
pip install grpcio-health-checking grpcio-reflection pytest prometheus_client
```

## The contract is the same file

The core gRPC idea shines here: your `.proto` is language-neutral. The
`devreg.proto`/`nafmock.proto` you write for Go compiles to Python with a
different generator, same semantics, same field numbers, same wire format.
A Go client talks to a Python server and neither knows.

Generate:

```bash
python -m grpc_tools.protoc -I api \
    --python_out=api --grpc_python_out=api \
    api/nafmock/v1/nafmock.proto
```

This writes `api/nafmock/v1/nafmock_pb2.py` (messages) and
`..._pb2_grpc.py` (stubs). **Known wart**: the generated grpc file imports
`nafmock.v1.nafmock_pb2` as a top-level path, so `api/` must be importable:

```python
sys.path.insert(0, str(Path(__file__).resolve().parents[N] / "api"))
from nafmock.v1 import nafmock_pb2, nafmock_pb2_grpc
```

This trips every Python/gRPC beginner once. Now it won't trip you.

## Server anatomy

```python
from concurrent import futures

server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
nafmock_pb2_grpc.add_NAFMockServicer_to_server(NAFMockServicer(), server)
server.add_insecure_port("[::]:50052")
server.start()
server.wait_for_termination()
```

A servicer is a class, one method per rpc, signature
`(self, request, context)`:

```python
class NAFMockServicer(nafmock_pb2_grpc.NAFMockServicer):
    def ApplyOperation(self, request, context):
        if not request.operation:                          # validation
            context.abort(grpc.StatusCode.INVALID_ARGUMENT, "operation required")
        md = dict(context.invocation_metadata())            # keys lowercase
        behavior = md.get("nafmock-behavior", "ok")
        ...
        return nafmock_pb2.ApplyOperationResponse(handle=h, echo=request.payload)
```

Differences worth noticing vs Go:

- **No `Unimplemented` embed** — inheritance covers forward compat instead.
- **`context.abort(...)` raises** — control flow is exceptions, not returns.
- **One thread per in-flight request** (from the pool) — so shared state
  needs a `threading.Lock`, exactly like the Go struct needed its mutex.
- **Return the response object**; proto fields are snake_case attributes:
  `request.payload`, `response.applied_at`.

## Client side

```python
channel = grpc.insecure_channel("localhost:50052")
stub = nafmock_pb2_grpc.NAFMockStub(channel)
try:
    resp = stub.ApplyOperation(req, timeout=0.2,
                               metadata=(("idempotency-key", key),))
except grpc.RpcError as e:
    code, details = e.code(), e.details()    # codes are the vocabulary
```

`grpcurl` works against Python servers too — once reflection is enabled
(below), every command in `mock-grpcServer/README.md` runs unchanged.

## Health + reflection (the two one-liners)

```python
from grpc_health.v1 import health, health_pb2, health_pb2_grpc
health_pb2_grpc.add_HealthServicer_to_server(health.HealthServicer(), server)

from grpc_reflection.v1alpha import reflection
reflection.enable_server_reflection(
    (nafmock_pb2.DESCRIPTOR.full_name, reflection.SERVICE_NAME), server)
```

## Interceptors

Exist, but clunkier than Go's — you wrap the handler chain:

```python
class LoggingInterceptor(grpc.ServerInterceptor):
    def intercept_service(self, continuation, details):
        handler = continuation(details)
        # details.method, details.invocation_metadata available here
        return handler
```

In Go, Thursday's correlation-ID work is pure interceptor. In Python's
threaded model the honest pattern is simpler: read
`context.invocation_metadata()` in the handler (or a small helper) and log
it. Interceptors are optional polish, not the backbone.

## Testing: the bufconn equivalent

grpc-Python has no bufconn; the standard is an **ephemeral port** — real
loopback TCP, port 0, kernel picks a free one:

```python
@pytest.fixture
def client():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=4))
    nafmock_pb2_grpc.add_NAFMockServicer_to_server(servicer, server)
    port = server.add_insecure_port("localhost:0")     # <- the trick
    server.start()
    with grpc.insecure_channel(f"localhost:{port}") as ch:
        yield nafmock_pb2_grpc.NAFMockStub(ch)
    server.stop(0)
```

Same properties you want from bufconn: per-test isolation, no port
collisions, real gRPC machinery (metadata, deadlines, codes) exercised.
Friday's failure matrix in pytest = this fixture + `@pytest.mark.parametrize`.

## Codes table

Identical enum, `grpc.StatusCode.X` in Python — see concepts/06. The one
translation rule: Go `status.Error(c, msg)` ≡ Python `context.abort(c, msg)`;
Go `status.Code(err)` ≡ Python `e.code()`.