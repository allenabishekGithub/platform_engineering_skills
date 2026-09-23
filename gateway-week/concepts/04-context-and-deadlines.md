# 04 — Context and deadlines

> Python reader: the same concepts translated to `ServicerContext`,
> `threading.Event` and asyncio live in `python/01-context-and-deadlines.md`.

## What context is

`context.Context` is Go's mechanism for carrying three things through a call
tree:

1. **Cancellation** — "stop working, nobody wants the result anymore"
2. **Deadline** — "stop working at time T at the latest"
3. **Request-scoped values** — correlation IDs (the only value you should put
   in it this week)

Every function that does I/O takes `ctx` as its first parameter:
`func DoThing(ctx context.Context, ...)`. The context forms a tree: a child
cancelled/deadline-expired cancels nothing upstream, but every descendant of a
cancelled context is cancelled.

## Where your contexts come from

- gRPC handlers: the framework hands you a ctx that is already wired to
  (a) the client's deadline, (b) cancellation if the client disconnects.
  **Use it. Never replace it.**
- Background workers (startup recovery): `context.Background()` is correct here,
  with its own timeout for the recovery pass.
- `main`: `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`
  for graceful shutdown (you met this in flowd ex1).

The bug this week must not contain: creating `context.Background()` or
`context.TODO()` anywhere in the request path. It severs the client's deadline
from the work being done — the work keeps running after the caller gave up,
and you lose prompt DEADLINE_EXCEEDED.

## Deadline propagation in practice

gRPC clients set a deadline: `clientCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)`.

That deadline travels with the request. In the gateway:

- The handler ctx (already deadline-bearing) is passed to the store and the executor.
- Every retry wait is ctx-aware: before sleeping/backoff and before each attempt,
  check `ctx.Err()`. If the deadline expired, stop — return `DEADLINE_EXCEEDED`
  promptly, do not start attempt #3 with 0 time left.
- Timeouts are not the only cancellation: the client can just hang up
  (`CANCELLED`). Same code path: ctx.Done().

## The idioms

```go
select {
case <-ctx.Done():
    return ctx.Err()
case <-ticker.C:
    // attempt
}
```

- `ctx.Err()` returns `context.DeadlineExceeded` or `context.Canceled` — check it
  with `errors.Is`, map to gRPC codes (`DEADLINE_EXCEEDED` / `CANCELLED`).
- `context.WithTimeout` returns a cancel func — `defer cancel()` always, or you
  leak the timer until it fires.
- In a goroutine you spawn: it must select on ctx.Done() or you cannot shut it down.

## Pitfalls (interview-grade)

1. **`time.After` in a loop leaks**: each call allocates a timer valid until it
   fires. Loops that run for hours accumulate garbage. Use `time.NewTicker`
   and stop it.
2. **Ignoring the ctx in the last hop**: you pass ctx everywhere, then the
   adapter calls the platform with no timeout. The deadline is only as real as
   the leaf call that checks it.
3. **Storing ctx in a struct**: contexts flow through parameters, not fields.
4. **Using values for anything but request-scoped data**: no config, no
   dependencies in ctx.

## Why this matters for the assignment

"Propagate deadlines using Go context" is explicitly in the spec, and the
timeout demo depends on it: a 200ms client deadline against a slow executor must
return DEADLINE_EXCEEDED quickly, with metrics/logs proving no zombie retry loop.
That only works if the ctx reaches every blocking call.
