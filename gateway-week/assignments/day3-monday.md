# Day 3 (Monday) — Protobuf contract and gRPC server

**Read first:** `teaching/grpc-fundamentals/` (if you have not built `devreg` yet, do that track first — day 3 assumes it)

**Goal:** the wire contract, generated Go code, a running server skeleton with
interceptor plumbing, and your first bufconn integration test.

## Tasks

1. Tooling is already installed user-local (no sudo on this machine):
   ```bash
   export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH
   go version        # go1.27.x
   protoc --version  # libprotoc 29.x
   which protoc-gen-go protoc-gen-go-grpc   # both in ~/go/bin
   ```
2. Create `api/gateway/v1/gateway.proto` per concepts/06: `Execute` +
   `GetOperation` rpcs, `ExecuteRequest`/`GetOperationRequest`/`ExecuteResponse`.
   Choose and note: generic `bytes payload` vs typed TFS/NAF payload.
3. Generate:
   ```bash
   protoc --go_out=. --go_opt=module=<module> \
          --go-grpc_out=. --go-grpc_opt=module=<module> \
          api/gateway/v1/gateway.proto
   ```
   Add a `Makefile` target `generate`. Commit generated code.
4. Server skeleton (`internal/server/`):
   - `GatewayService` struct implementing both rpcs — return
     `codes.Unimplemented` for now.
   - A `CorrelationIDInterceptor` (concepts/06): read/create
     `x-correlation-id` metadata, ctx-inject, echo back via `grpc.SetHeader`.
   - A `LoggingInterceptor`: log method, correlation ID, duration, code.
   - Register `grpc.health.v1.Health` + set SERVING.
   - HTTP side (`internal/telemetry/`): `/healthz` and `/metrics` (metrics
     endpoint can be empty today — promhttp with default registry).
5. `cmd/gateway/main.go`: flags for ports; graceful stop (signal.NotifyContext,
   `grpcServer.GracefulStop()` with a timeout).
6. First integration test (`internal/server/server_test.go`): bufconn dial,
   `Execute` returns `Unimplemented`, BUT the response headers contain the
   correlation ID you sent (or an invented one). Assert both paths.

## Hints

- Metadata keys are lowercased: `md.Get("x-correlation-id")`.
- Interceptor order matters: correlation ID first, then logging (logging
  wants the ID in ctx). Write them as a chain; verify the order in your test.
- bufconn dialer: `grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) })` plus `grpc.WithTransportCredentials(insecure.NewCredentials())`.
- Generate UUIDs however you like (`crypto/rand` is dependency-free).

## Acceptance criteria

- [ ] `make generate` regenerates without diffs (your contract is the source of truth)
- [ ] bufconn test round-trips and asserts correlation ID echo (both supplied and generated cases)
- [ ] Logging interceptor produces one JSON line per rpc with correlation ID + code
- [ ] gRPC health RPC returns SERVING; `/healthz` returns 200
- [ ] `go test -race ./...` green

## Commit

```
day3: gRPC contract, server skeleton, interceptors, bufconn harness
```
