# 02 — How to write protobuf files

## Anatomy first

The entire `.proto` your gateway needs, annotated:

```protobuf
syntax = "proto3";                     // language version — always proto3 today

package gateway.v1;                    // proto namespace; guards name collisions
                                       // convention: <domain>.v<N>

option go_package =                    // where generated Go lives + import name
  "github.com/<you>/gateway-week/gateway/api/gateway/v1;gatewayv1";

message ExecuteRequest {               // message = a struct
  string operation = 1;                // scalar type, name, = FIELD NUMBER
  bytes  payload  = 2;
}

service Gateway {                      // service = the RPC interface
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);   // unary
}
```

Five kinds of lines: syntax, package, options, messages, services. That is the
whole grammar surface you need.

## The rules that matter

### 1. Field numbers are forever

On the wire, a field is **its number and a type**, never its name. `operation = 1`
costs 2 bytes: tag byte `0x0A` ("field 1, wire type bytes") plus a length.
Rename a field freely; **never reuse or renumber**.

Evolution rule:

```protobuf
// WRONG: v2 reuses field 2 for new meaning
message ExecuteRequest {
  string operation = 1;
  bytes  request_body = 2;   // old clients send payload here!
}

// RIGHT: reserve dead numbers, add new fields
message ExecuteRequest {
  reserved 2;                 // "2 is dead, nobody may ever use it"
  reserved "payload";
  string operation = 1;
  bytes  request_body = 3;
}
```

Old binaries sending field 2 to new binaries simply skip it (proto3 ignores
unknown fields) — *that* is your wire compatibility, and it survives only if
numbers are never recycled.

### 2. Scalar types (the ones you'll actually use)

| Proto type | Go type  | Notes |
| ---------- | -------- | ----- |
| `string`   | `string` | UTF-8 |
| `bytes`    | `[]byte` | arbitrary blob — your payload |
| `int64`    | `int64`  |
| `bool`     | `bool`   |
| `enum`     | int-backed type | define your own |
| `google.protobuf.Duration` | (well-known) | only if you need precision |

proto3 default: **unset scalar == zero value** — no `nil` strings, no
"was it sent?" for scalars. If absent-vs-present matters, use `optional`
(it generates pointer fields). Otherwise your validation just checks
`req.GetOperation() == ""`.

### 3. Structure: nested, repeated, oneof, maps

```protobuf
message Operation {
  string id = 1;

  enum State {                // enum nested in the message it belongs to
    STATE_UNSPECIFIED = 0;    // MUST have a zero value, name it UNSPECIFIED
    RECEIVED = 1;
    EXECUTING = 2;
    SUCCEEDED = 3;
    FAILED = 4;
  }
  State state = 2;

  repeated string attempts = 3;   // -> []string; a list
  map<string, string> labels = 4; // -> map[string]string; unordered!

  message RetryPolicy {            // nested message type
    int32 max_attempts = 1;
    int32 backoff_ms = 2;
  }
  RetryPolicy retry_policy = 5;   // -> *RetryPolicy (nil-able pointer)

  oneof result {                   // exactly one of these is ever set
    bytes success_payload = 6;
    string failure_reason = 7;
  }
}
```

`oneof` is how proto does "variant" — perfect for your terminal states
(SUCCEEDED-with-result vs FAILED-with-reason) if you choose to model it
in the message rather than plain fields.

### 4. Style conventions (the reviewer-pleasing ones)

- Messages: `PascalCase`. Fields: `snake_case`. Enums: `SCREAMING_SNAKE`.
- Enum values prefixed with the enum name (`State_STATE_UNSPECIFIED` idiom).
- Every message with an identity gets `string <entity>_id = 1;`.
- `bytes payload` for opaque pass-through (your gateway: generic over the
  NAF/TFS operation). Typed fields once the operation is fixed — you'll
  write down which you chose and why.
- `repeated` field names: plural.

### 5. Services

```protobuf
service Gateway {
  // Unary: blocking call/return — this week.
  rpc Execute(ExecuteRequest) returns (ExecuteResponse);

  // Streaming (know the syntax, don't use this week):
  // rpc Watch(OperationId) returns (stream OperationEvent);
}
```

RPC naming: verb-noun, not nouns ("Execute", "GetOperation", not
"operation_execute"). Input and output are always messages, never scalars —
so `GetOperationRequest` exists even if it holds one `operation_id` string.
That is a rule, and it keeps every rpc evolvable.

## Generating Go

```bash
protoc --go_out=. --go_opt=module=<your-module> \
       --go-grpc_out=. --go-grpc_opt=module=<your-module> \
       path/to/thing.proto
```

Read what lands next to the proto:

- `thing.pb.go` — struct types, getters (`GetOperation()` is nil-safe!),
  and `Marshal`/`Unmarshal` taking the field numbers to bytes.
- `thing_grpc.pb.go` — server side: `ThingServer` interface + `RegisterThingServer`;
  client side: `NewThingClient` returning the stub; + `UnimplementedThingServer`
  to embed for forward compatibility.

## Deterministic marshal — your idempotency depends on it

Plain `proto.Marshal(m)` may serialize map entries in any order → same
logical message, different bytes, different SHA-256 → your idempotency store
would see two payloads for identical requests. The fix is one line your
gateway must use:

```go
b, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
```

Remember this on Tuesday; it is the difference between dedup working and
"works until someone adds a map label."

## Exercise set (do them, answers at the bottom — no peeking)

**E1.** Write `api/gateway/v1/gateway.proto`: package `gateway.v1`,
`go_package` with alias `gatewayv1`, messages `ExecuteRequest`
(`operation` string, `payload` bytes), `GetOperationRequest`
(`operation_id` string), `ExecuteResponse` (`operation_id`, `state`,
`result` bytes, `failure_reason` string), and service `Gateway` with
`Execute` and `GetOperation`. Run protoc. Open `gateway.pb.go` and find:
the getter for payload, and the field number 2 in the struct tags.

**E2.** Bugs in this proto — find all four:

```protobuf
syntax = "proto3";
package gateway;

message ExecuteRequest {
  string operation = 1;
  bytes payload = 1;
  string opName = 2;
}

service gateway {
  rpc Execute(ExecuteRequest) returns (string);
}
```

**E3.** You shipped v1 with `bool force = 4;`. Product now says "remove it."
Write the safe v2 change, and say what an old client still sending
`force=true` will experience against a v2 server.

## Answers

E2: duplicate field number (1 used twice — protoc actually rejects this);
field naming not snake_case (`opName` → `op_name`); no `go_package` option;
service name lowercase (must be `Gateway`); rpc returning a bare scalar
`string` (must be a message).

E3: `reserved 4; reserved "force";` — no replacement field. Old client:
v2 parses the message, **ignores unknown field 4**, proceeds normally.
That is why "remove" means *reserved*, never *renumber*.
