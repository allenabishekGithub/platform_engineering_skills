# 01 — What an execution gateway is

## The problem

You have an existing platform (TFS/NAF) with operations that **do things** —
provision, configure, mutate state. These operations are:

- **Slow** (network calls, sometimes seconds)
- **Failure-prone** (timeouts, connection drops, downstream errors)
- **Side-effecting** (calling twice is not the same as calling once)

Clients calling such operations directly have to solve the same hard problems
over and over: what if my request times out — did it execute? What if I retry —
will I double-provision? Where do I see what happened?

## The solution shape

An **execution gateway** is a thin service that sits in front of one (or a few)
side-effecting operations and adds an execution contract:

```
client --(gRPC + idempotency key + deadline)--> gateway --> existing operation
```

The gateway owns:

- **Identity**: what request is this? (idempotency)
- **Lifecycle**: what state is the operation in? (state machine)
- **Time**: how long may this take? (deadlines)
- **Failure policy**: what is retried, what is not
- **Evidence**: logs, metrics, correlation (observability)

The gateway does NOT own the business logic. That stays in TFS/NAF.

## The key design move: the Executor interface

The single most important line of Go you will write this week:

```go
type Executor interface {
    Execute(ctx context.Context, op Operation) (Result, error)
}
```

Everything hard — tests, fakes, timeout injection, counting executions for the
duplicate demo — becomes trivial because the gateway depends on an **interface**,
not on TFS/NAF. This is dependency inversion: both the real adapter
(`TFSNAFExecutor`) and the test fakes implement `Executor`.

Consequences:

- Unit tests never need the real platform.
- The real adapter is thin: translate request, call the platform, translate the
  response and error.
- Your gateway can be developed end-to-end before you have access to the real op.

## Ports and adapters (one-paragraph version)

The pattern above is a slice of hexagonal architecture: the "core"
(state machine, idempotency, retry, store) depends only on interfaces
("ports"): `Executor`, `Store`, `Clock`. Concrete implementations
("adapters"): the TFS/NAF client, SQLite, the system clock. Test doubles are
just other adapters. You will recognize this shape in every serious platform
codebase you read from now on.

## Scope control

"Do not rewrite the entire platform" means:

- One operation, wrapped. Not a generic execution engine.
- No new business rules. Validation is structural (required fields, formats),
  not semantic.
- The gateway adds an execution contract, nothing else.

If you are tempted to put TFS/NAF logic in the gateway this week, stop — that
logic belongs in the adapter, and the adapter should be boring.

## How this maps to your week

| Gateway concern   | Day        |
| ----------------- | ---------- |
| Lifecycle         | Saturday   |
| Core Go mechanics | Sunday     |
| The front door    | Monday     |
| Identity          | Tuesday   |
| Time + failure    | Wednesday  |
| Evidence          | Thursday  |
| Proof             | Friday    |
