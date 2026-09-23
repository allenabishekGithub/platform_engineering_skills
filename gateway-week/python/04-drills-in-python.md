# 04 (Python) — Day 2 drills, Python edition + Rosetta stone

Same drills as `assignments/day2-sunday.md`, done with Python idioms — plus
the Go↔Python primitive table you'll use all week to translate any snippet
in these docs.

## The Rosetta table (bookmark this)

| Go | Python | Note |
| -- | ------ | ---- |
| interface | `typing.Protocol` (structural) or ABC | Protocol is closest: no inheritance needed |
| `errors.Is` / `errors.As` | `isinstance(e, MyExc)` / exception attributes | exceptions are classes |
| `fmt.Errorf("...: %w", err)` | `raise NewExc(...) from err` | chaining = wrapping |
| goroutine | `threading.Thread` / `asyncio.Task` | threads for I/O; tasks in aio |
| channel | `queue.Queue` | `maxsize=` gives you the bounded-channel semantics for free |
| `WaitGroup.Wait()` | pool shutdown + `queue.join()` / `with ThreadPoolExecutor` | structured concurrency |
| `<-ctx.Done()` | `Event.wait()` / `context.is_active()` / `task.cancel()` | cancellation must be *cooperated with* |
| `select` | poll loop (threads) / `asyncio.wait` (aio) | |
| `time.After` timer | `Event.wait(timeout=...)` | |
| `sync.Mutex` | `threading.Lock` (`with lock:`) | |
| `go test -race` | no equivalent — discipline instead | reason Go devs sleep well |

## Drill 1 — interfaces → `typing.Protocol`

```python
from typing import Protocol

class Downstream(Protocol):
    def do(self, op: str) -> str: ...

class FlakyDownstream:
    def __init__(self, failures: int):
        self.failures, self.calls = failures, 0
    def do(self, op: str) -> str:
        self.calls += 1
        if self.calls <= self.failures:
            raise TransientError("downstream hiccup")
        return op.upper()
```

A `Retrier(downstream: Downstream)` that knows nothing but the Protocol.
Acceptance: same as Go — retry loop succeeds after 3 flaky failures; the fake
counted 4 calls. Notice: `FlakyDownstream` never declared it implements
anything — structural typing, exactly like Go.

## Drill 2 — errors → exception design

Build the hierarchy your gateway will actually use:

```python
class GatewayError(Exception): ...
class TransientError(GatewayError): ...        # retryable
class PermanentError(GatewayError): ...         # non-retryable
class InDoubtError(GatewayError): ...          # ambiguous — never retried
```

Exercises: a `Classify(err)` function keyed on `isinstance` (this IS your
Wednesday `Classify`); a chained failure `raise RetriesExhausted(...) from
last_err` and asserting `e.__cause__` survives; catching order — why
`except GatewayError` before `except TransientError` is a bug (subclass!).
Acceptance: tests for all three idioms.

## Drill 3 — goroutines/channels → bounded pool with `queue.Queue`

- N worker threads consuming from `queue.Queue`.
- Bounded = `Queue(maxsize=10)`: `put()` **blocks** when full (backpressure)
  or `put_nowait()` raises `queue.Full` (drop policy) — you just met
  Wednesday's backpressure decision, in the stdlib.
- Clean shutdown: N sentinel objects (`None`) through the queue, workers
  exit on sentinel; main joins via `queue.join()` or thread joins.
- Leak drill equivalent: a worker blocked forever on an unbounded queue
  with no sentinel — detect by joining with timeout and observing the
  worker never exits; fix with the sentinel.

Acceptance: pool drains a 50-item queue with 4 workers, all items processed
exactly once, all threads exit (assert via `is_alive()` polling), and you
can demonstrate backpressure by shrinking maxsize.

## Drill 4 — context cancellation → `threading.Event`

- A "downstream call" as a loop: `while not stop_event.is_set(): sleep(0.1)`
  then returns.
- Parent sets the event after 100ms → child returns within ~100ms, not 1s.
- Deadline version: `if not stop_event.wait(timeout=0.2): raise TimeoutError`.
- asyncio variant (do both): `task = asyncio.create_task(work())`,
  `task.cancel()` after 100ms, handler sees `asyncio.CancelledError` — note
  that cancellation only lands where the coroutine *awaits* (cooperative,
  like Go checking ctx).

Acceptance: cancellation observed within 50ms of target; you can state the
threaded vs asyncio difference out loud (polling/flag vs scheduled interrupt).

## Commit

```
day2py: python drills — protocols, exception design, bounded pools, cancellation
```

(Or fold into the same commit as your Go drills — but doing both is how the
Rosetta table moves from paper to muscle.)