# 01 — What gRPC is and how it works

## The one-sentence version

gRPC is a framework where a client calls a method on a remote machine as if it
were a local function call; the contract (methods, parameters, return types)
is defined once in a `.proto` file, and *both* sides get generated, typed code
from it.

```
you write: proto contract  ──protoc──>  Go client stub  +  Go server interface
                                                      │
client.Execute(ctx, req) ─────────────────────────────┘
   → [your handler] func (s *Server) Execute(ctx, req) (resp, error)
```

## Why it exists (what REST doesn't give you)

REST + JSON works, but every client re-implements: URL shapes, status-code
conventions, JSON field naming, retry semantics, timeout handling. gRPC
moves all of that into the contract and the transport:

| Concern          | REST/JSON                     | gRPC                              |
| ---------------- | ----------------------------- | --------------------------------- |
| Contract         | OpenAPI doc (often lagging)   | `.proto` — generates the code     |
| Wire format      | Text JSON (parse, quote, big) | Binary protobuf (small, fast)     |
| Transport        | HTTP/1.1 (usually)            | HTTP/2 (multiplexed streams)     |
| Typing           | You build/hand-parse structs  | Types are generated              |
| Deadlines        | Ad hoc                        | First-class, propagated          |
| Streaming        | SSE/WebSocket add-ons         | Built-in (4 call kinds)          |
| Error vocabulary | Per-API status juggling       | Fixed `codes.Code` enum          |

For an *execution gateway* — internal, service-to-service, strict contract,
deadline-sensitive — gRPC is the natural fit. That is why your assignment
specifies it.

## The request lifecycle, end to end

Follow one unary call; this is the mental model to carry:

```
 1. client.Execute(ctx, req)                  — generated stub method
 2. stub marshals req to protobuf bytes       — field numbers, not names!
 3. stub attaches deadline from ctx + metadata to HTTP/2 HEADERS
 4. HTTP/2 stream opened over ONE connection  — many calls share the channel
 5. server's grpc runtime receives stream
 6. it unmarshals bytes into the generated struct
 7. YOUR handler runs: (ctx, req) -> (resp, error)
 8. resp marshaled; error mapped to a code via status
 9. stream closed with status OK or a code (in the "trailers")
10. client stub unmarshals, returns (resp, nil) or (nil, status err)
```

Things worth internalizing:

- **The connection is the channel.** A `grpc.ClientConn` is long-lived and
  multiplexes thousands of concurrent calls. Do not dial per request.
- **Field numbers are the identity of a field on the wire.** Names exist only
  in the `.proto` (file 02 is why this matters for evolution).
- **Errors are codes, not strings.** The status travels in HTTP/2 trailers;
  `status.Error(codes.Unavailable, "...")` on one side comes out as
  `status.Code(err) == codes.Unavailable` on the other. The message is for
  humans; the code is for programs. Your retry classification keys on codes.
- **Deadline travels with the call, automatically.** If the client ctx has a
  deadline, the server's ctx has *the same* deadline, and the runtime
  cancels handlers when it fires. This is why "propagate deadlines using
  Go context" is mostly: *use the ctx you were handed, don't replace it*.
- **Cancellation propagates.** Client hangs up mid-call → server ctx is
  cancelled → your `select` on `ctx.Done()` wakes up. No code you write
  moves it; refusing to break the ctx chain is what preserves it.

## The four call kinds

| Kind | Signature shape | Example use |
| --- | --- | --- |
| Unary | `req → resp` | **your `Execute`** |
| Server-streaming | `req → stream resp` | subscribe to operation progress |
| Client-streaming | `stream req → resp` | upload batch of operations |
| Bidirectional | `stream req → stream resp` | chat/relay |

This week: unary only. Know the others exist.

## Metadata: the headers of gRPC

`metadata.MD` is string→[]string, attached to a call like HTTP headers.

- Client → server: `metadata.AppendToOutgoingContext(ctx, "key", "value")`,
  server reads `metadata.FromIncomingContext(ctx)`. **Keys are lowercased.**
- Server → client: `grpc.SetHeader` / `grpc.SetTrailer`.

Your gateway uses metadata for: `idempotency-key` (in), `x-correlation-id`
(in, echoed back out). Identity and tracing ride metadata; the *operation*
rides the message. That separation is deliberate — review it later.

## The codes you will live in this week

| Code | Meaning | Your gateway maps it to |
| --- | --- | --- |
| `OK` | success | SUCCEEDED |
| `INVALID_ARGUMENT` | malformed request | FAILED(validation), non-retryable |
| `ALREADY_EXISTS` | conflict creating | key-reuse-with-different-payload |
| `UNAVAILABLE` | transport / not reachable | retryable |
| `DEADLINE_EXCEEDED` | timeout | deadline → in-doubt policy |
| `CANCELLED` | caller gave up | stop work |
| `FAILED_PRECONDITION` | state says no | non-retryable |
| `INTERNAL` | bug | non-retryable, page yourself |

`nafmock` speaks exactly this vocabulary (see its behavior table) so your
`Classify` can key on codes end-to-end.

## Interceptors: the middleware of gRPC

Unary interceptor signature (server side):

```go
func(ctx context.Context, req any,
    info *grpc.UnaryServerInfo,
    handler grpc.UnaryHandler) (any, error)
```

Run *before* your handler, wrap it: logging, correlation IDs, metrics,
recovery. Chain them: correlation-ID first (so later interceptors and the
handler see it in ctx), then logging, then metrics. Your Thursday is
interceptors.

## Health & reflection

Two standard add-ons every server should register:

- `grpc.health.v1.Health` — the standard "are you serving" RPC; orchestrators
  (k8s, service meshes) speak it natively.
- Reflection — lets tools like `grpcurl` discover your API at runtime without
  the `.proto` file. Essential for manual testing.

Both are one-liner registrations in `main` — see them in `nafmock/cmd/nafmock/main.go`.

## Self-check

Answer without looking:

1. Two clients share one `ClientConn` and both call `Execute`. How many TCP
   connections carry both calls?
2. Where does the retryable-vs-not signal live on the wire: the error message
   or the status code?
3. Client sets a 200ms deadline. Exactly what must the handler be doing so
   the deadline is real at the TFS/NAF call?
4. Why is the idempotency key in metadata rather than a proto field?

(1: one — HTTP/2 multiplexing. 2: the code. 3: passing the handler ctx into
every blocking call, checking ctx.Done() — never context.Background().
4: it identifies the *caller's intent*, not the operation content — payloads
should hash identically regardless of key.)
