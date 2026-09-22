# 06 — gRPC essentials

## What you need this week

- proto3 file defining one service with (at least) `Execute` and a status query
- A Go server implementing it
- An interceptor for cross-cutting concerns (correlation ID, logging, metrics)
- The standard health service
- `bufconn` for in-process tests (no real network, no port juggling)

## The contract

```protobuf
syntax = "proto3";

package gateway.v1;

option go_package = "github.com/<you>/gateway-week/gateway/api/gateway/v1;gatewayv1";

message ExecuteRequest {
  string operation = 1;     // which TFS/NAF operation
  bytes  payload = 2;       // structured operation body
}

message ExecuteResponse {
  string operation_id = 1;
  string state = 2;         // State enum as string is fine this week
  bytes  result = 3;
  string error_reason = 4;  // populated when FAILED
}

service Gateway {
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);
  rpc GetOperation(GetOperationRequest) returns (ExecuteResponse);
}
```

The idempotency key travels in **gRPC metadata** (`idempotency-key`), matching
the Stripe-header convention — not in the payload, because it is caller
identity, not operation content.

Design choices to make consciously:

- `Execute` returns the *final* state this week (the operation is synchronous).
  A production gateway would likely return immediately with `operation_id` and
  let clients poll `GetOperation` — note this as future work.
- `payload` as bytes keeps the gateway generic over the one wrapped operation.
  Alternatively type it concretely to your real TFS/NAF operation — both are
  defensible; write down why.

## Errors: status codes, not error strings

Return gRPC status errors from handlers:

```go
return nil, status.Errorf(codes.InvalidArgument, "payload: %v", err)
```

Mapping you will use: `INVALID_ARGUMENT` (validation), `ALREADY_EXISTS`
(key reuse with different payload), `DEADLINE_EXCEEDED`, `CANCELLED`,
`UNAVAILABLE` (downstream, when retries are exhausted), `INTERNAL` (bugs).
This is the vocabulary your retry classification and tests share.

## Interceptors

Server-side unary interceptor:

```go
func CorrelationID(ctx context.Context, req any,
    info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
    // read/create correlation ID from incoming metadata
    // stash in ctx, pass to handler
}
```

- Incoming metadata: `metadata.FromIncomingContext(ctx)` (lowercase keys).
- Outgoing (trailer/header back to client): `grpc.SetTrailer` / `grpc.SetHeader`.
- Interceptors are where Thursday's logging/metrics/correlation live — the
  handler stays clean.

## Health

Register the standard health service — clients and orchestrators speak it natively:

```go
import healthpb "google.golang.org/grpc/health/grpc_health_v1"
healthpb.RegisterHealthServer(grpcServer, health.NewServer())
```

Serving `/metrics` is plain HTTP on a separate port (`:9090`), not gRPC.

## Testing with bufconn

`bufconn` is an in-memory listener: you run the real gRPC machinery —
interceptors, metadata, deadlines — over a pipe instead of TCP:

```go
lis := bufconn.Listen(1024 * 1024)
// serve on lis, dial with grpc.WithContextDialer(func(ctx, _) { return lis.DialContext(ctx) })
```

That is your integration-test harness: start server + fake executor +
temp-file store per test, dial, assert. The duplicate demo is an integration
test with two concurrent dials.

## Tooling you will install (Monday)

- `protoc` (apt) — the compiler
- `protoc-gen-go` and `protoc-gen-go-grpc` via `go install` — the Go plugins

Generate into `api/gateway/v1/`, commit the generated code, and a
`make generate` target so regeneration is one command.
