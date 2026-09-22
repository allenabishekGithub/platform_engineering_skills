# gRPC fundamentals (teaching track)

Prerequisite for **day 3** of gateway-week. concepts/06 is the condensed
checklist; this track is the deep dive that makes the checklist make sense.

## Reading order

| File | What you learn | Do while reading |
| ---- | -------------- | ---------------- |
| 01-what-is-grpc.md | What gRPC is, the full request lifecycle, deadlines/metadata/codes | Draw the lifecycle from memory |
| 02-protobuf-syntax.md | How to write `.proto` files that survive evolution | Do the 3 exercises (answers at bottom) |
| 03-grpc-server-anatomy.md | Dissect a real server — `mock-grpcServer` (the mock NAF/TFS platform your gateway will call) | Read its actual source alongside |
| 04-build-your-own-server.md | Build a tiny unary gRPC service yourself, step by step | Everything — this is you doing it |

## Learning goals

By the end you can, without notes:

1. Explain what happens between `client.Execute(ctx, req)` and your handler
   receiving `(ctx, req)` — at the HTTP/2 and protobuf level.
2. Write a `.proto` file a reviewer would approve: packages, options, field
   numbers, reserved, oneof.
3. Implement a Go gRPC service: struct, method, status codes, metadata,
   registration, health, reflection.
4. Say exactly which gRPC feature carries each gateway requirement
   (deadline -> context, idempotency key -> metadata, retry classes -> codes).

## Rules of the track

- No copy-paste servers: file 04 gives instructions, you write the code.
- The sample you mimic is `mock-grpcServer/` — real, tested, in this repo.
  Read it; do not transcribe it. You learn by re-deriving, not copying.
- When stuck for >30 minutes on one step, write down precisely what you
  expected vs. what happened — that note is usually the answer.
