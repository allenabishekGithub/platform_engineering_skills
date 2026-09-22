# Day 2 (Sunday) — Go drills: interfaces, errors, goroutines, context

**Read first:** concepts/01 (Executor interface), concepts/04

**Goal:** sharpen the four Go mechanics the gateway is built from. These are
deliberately throwaway exercises — a `drills/` folder you can delete Friday.
Do them in order; do not skip even where it looks trivial.

## Drill 1 — Interfaces

Create `drills/interfaces/` with:

```go
type Downstream interface {
    Do(ctx context.Context, in string) (string, error)
}
```

- Implement TWO types: `RealDownstream` (returns input uppercased) and
  `FlakyDownstream` (fails N times then succeeds, counting calls).
- Write a `Retrier` struct that takes a `Downstream` and retries failures
  using `FlakyDownstream`. No type assertions, no `if concrete := ...` tricks —
  the Retrier must know nothing but the interface.

Acceptance: a test proves Retrier succeeds after 3 flaky failures and that the
fake counted exactly 4 calls.

## Drill 2 — Errors

In `drills/errors/`:

- Define a sentinel error `var ErrTransient = errors.New("transient")`.
- Write a function that wraps it: `fmt.Errorf("dial %s: %w", addr, ErrTransient)`.
- Test with `errors.Is(err, ErrTransient)` — true through the wrap.
- Define `type NotFoundError struct{ ID string }` implementing `error`;
  test `errors.As(err, &target)` and assert `target.ID`.
- Add `errors.Join` usage: return two errors joined, assert `errors.Is` finds
  each. (This is how you will report "deadline exceeded AND last error was X".)

Acceptance: all three idioms (`Is`, `As`, `Join`) used correctly in tests.

## Drill 3 — Goroutines and channels

In `drills/goroutines/`:

- Bounded worker pool: N workers, a `chan Job`, a `sync.WaitGroup`, clean
  shutdown when the jobs channel closes. Every worker logs what it consumed.
- Cancel-safe select loop:
  ```go
  for {
      select {
      case <-ctx.Done(): return
      case <-ticker.C:  // work
      }
  }
  ```
- Prove a goroutine leak: start one that blocks forever on an unbuffered
  channel, detect the leak with `goleak` (`go.uber.org/goleak`) — then fix it.

Acceptance: `go test -race` green; you can explain why the pool exits cleanly
and what the leak was.

## Drill 4 — Context cancellation

In `drills/context/`:

- A "downstream call" that sleeps 1s but selects on ctx — return `ctx.Err()` if
  cancelled first.
- Parent cancels after 100ms → child returns within ~100ms, not 1s.
- Deadline version: `context.WithTimeout(ctx, 200ms)` → `errors.Is(err, context.DeadlineExceeded)`.
- The leak drill: `context.WithTimeout` without `defer cancel()` inside a
  `for` loop running 1e6 iterations — watch memory; then fix with `defer`.

Acceptance: tests prove cancellation propagates in <50ms of the target time,
and you can state the difference between `Canceled` and `DeadlineExceeded`.

## Commit

```
day2: go drills — interfaces, errors, goroutines, context
```
