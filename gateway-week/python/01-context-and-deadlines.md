# 01 (Python) — Context and deadlines

Python equivalent of `concepts/04-context-and-deadlines.md`. Same concepts,
different carrier: Go has one universal `context.Context`; Python splits the
job across three tools depending on where you stand.

## Where the three context jobs live in Python

| Job | Go | Python |
| --- | --- | ------- |
| Deadline for an RPC | `ctx` carries it | `grpc.ServicerContext.time_remaining()` (server) / `timeout=` (client) |
| Cancellation | `<-ctx.Done()` | `ServicerContext.is_active()` / `.add_callback(fn)`; general code: `threading.Event`; asyncio: task cancellation |
| Request-scoped values | `ctx` values | gRPC metadata + `contextvars.ContextVar` |

Key mental shift: in Go the ctx *flows through your functions* — you pass it
everywhere, so every call site can check it. In grpc-Python the framework
hands each handler one `ServicerContext` — **you** must poll it, because a
plain blocking call (`time.sleep`, `requests.get`) knows nothing about it.
The deadline is only as real as the leaf call that checks it — same lesson,
stated as polling instead of passing.

## Deadline propagation in practice

Client side (this is the whole API):

```python
resp = stub.ApplyOperation(req, timeout=0.2)          # seconds
resp = stub.ApplyOperation(req, metadata=(("x-correlation-id", cid),))
```

That timeout travels with the RPC. Server side:

```python
def ApplyOperation(self, request, context):
    remaining = context.time_remaining()   # seconds, or None if no deadline
    ...
```

Your Wednesday retry loop in Python: before each attempt and each backoff
sleep, check `remaining = context.time_remaining()` — if `remaining is None`
there is no deadline; if it hits 0, abort with `DEADLINE_EXCEEDED`. Do not
start attempt 3 with 0 time left — identical rule to the Go version.

## Cancellation: the polling pattern

The client hanging up does NOT kill your handler thread — it signals it.
Blocking code must cooperate:

```python
# sleep that respects cancellation (the select-on-ctx.Done() equivalent)
def wait_context(context, seconds, poll=0.05):
    deadline = time.monotonic() + seconds
    while time.monotonic() < deadline:
        if not context.is_active():
            context.abort(grpc.StatusCode.CANCELLED, "caller gone")
        time.sleep(min(poll, deadline - time.monotonic()))
```

For long single waits, `context.add_callback(fn)` registers a function that
fires when the RPC dies — set a `threading.Event` in it and `event.wait()`
instead of polling. Both are correct; polling is simpler to reason about.

The bug this week must not contain (same as Go): work that outlives the
caller. Go's version is `context.Background()` in the request path; Python's
is a handler that ignores `context.is_active()` — a thread grinding away on
a result nobody will read.

## asyncio variant (know it exists)

`grpc.aio` gives you real cancellation: `async` handlers, `asyncio.wait_for`,
`CancelledError` — semantics much closer to Go ctx. If you already know
asyncio, the aio server is arguably the nicer teaching target. But the sync
threaded server is simpler and matches this week's specs; we stay sync
everywhere and you can port later.

## Pitfalls (Python editions)

1. **Blocking sleep ignores deadlines** — `time.sleep(5)` in a handler burns
   5s regardless of the caller's 200ms deadline. Use `wait_context` above.
2. **Client timeouts raise, they don't return** — `grpc.RpcError` with
   `e.code() == grpc.StatusCode.DEADLINE_EXCEEDED`. Catch `RpcError`, never
   bare `Exception`.
3. **`contextvars` don't cross threads for free** — in the threaded server
   model, prefer reading what you need from `context.invocation_metadata()`
   inside the handler over clever implicit propagation.
4. **GIL note**: threads are fine for this workload (I/O-bound waiting), but
   they don't parallelize CPU — irrelevant this week, know it anyway.

## Self-check

1. Client sets `timeout=0.2`. Where exactly does your handler observe it?
2. What happens to a handler mid-`time.sleep(2)` when the client cancels?
3. Go: `select { case <-ctx.Done(): ... }`. Python equivalents — name both.
4. Why does the retry loop check `time_remaining()` *before* sleeping?

(1: `context.time_remaining()` returns ~0.2, ticking down. 2: nothing, until
the sleep ends — that's the bug the polling pattern fixes. 3: poll
`is_active()` / `add_callback` + `Event.wait`. 4: to avoid starting an
attempt that cannot finish — return DEADLINE_EXCEEDED promptly instead.)