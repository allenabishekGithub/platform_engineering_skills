# Day 5 (Wednesday) — Timeout, retry and recovery

**Read first:** concepts/05, re-read concepts/04 (deadline idioms)

**Goal:** the failure-policy layer. Timeouts propagate, retries are bounded
and classified, and the gateway recovers correctly from a crash mid-execution.

## Tasks

1. `internal/executor/` — formalize:
   ```go
   type Operation struct { ID string; Name string; Payload []byte }
   type Result struct { Payload []byte }
   type Executor interface {
       Execute(ctx context.Context, op Operation) (Result, error)
   }
   ```
   Plus a `TFSNAFExecutor` stub (logs and returns success — the real adapter is
   swapped in at work) and test fakes: `FlakyExecutor` (N retryable failures
   then success, records timestamps), `SlowExecutor` (sleeps, ctx-aware),
   `NonRetryableExecutor`, `CountingExecutor`.
2. Error classification (`internal/executor/classify.go`):
   `func Classify(err error) (retryable bool, reason string)` with the table
   from concepts/05. Make fake errors carry the classification; the real
   adapter will translate TFS/NAF error codes into the same vocabulary.
3. The retry loop (in the server, wrapping the Executor):
   - max 3 attempts, exponential backoff 50ms base, full jitter
   - before each attempt AND each backoff sleep: check ctx (concepts/04 select
     idiom) — deadline means stop now, return DEADLINE_EXCEEDED
   - each attempt's outcome logged (attempt number, classification)
   - non-retryable -> immediate FAILED, no retry
   - retries exhausted -> FAILED(retries_exhausted), surface last error
4. Crash recovery in `main`/startup:
   - on boot, `store.InExecuting(ctx)`; for each in-doubt record apply your
     documented D1 policy. Start with fail-closed: mark FAILED(in_doubt).
     Note re-execute as the alternative if the op is idempotent.
   - do recovery BEFORE flipping health to SERVING.
5. Tests:
   - Flaky fails 2x then succeeds -> 3 attempts, gaps between attempts
     (assert recorded timestamps are increasing and >= backoff), SUCCEEDED.
   - Non-retryable -> 1 attempt only, FAILED.
   - Slow executor + 200ms deadline -> DEADLINE_EXCEEDED, no further attempts.
   - Recovery: seed store with an EXECUTING record, construct server,
     startup recovery runs, record is FAILED(in_doubt) (or your chosen policy).

## Hints

- Backoff jitter: `wait := base * 2^(attempt-1)`; sleep `rand` in `[0, wait)`.
  Seed your rand in tests or assert ranges, not exact gaps.
- "No further attempts after deadline" is provable with the counting fake:
  its call count must be exactly the number of attempts you expect.
- The retry loop is the most-tested component of the week — every branch:
  retryable-then-success, retryable-exhausted, non-retryable, deadline,
  ctx-cancelled. Write all five.

## Acceptance criteria

- [ ] All five retry-branch tests exist and pass
- [ ] Timestamps prove backoff actually happened (not just attempt counts)
- [ ] Deadline test: call returns within ~deadline, executor attempts == expected
- [ ] Recovery test proves in-doubt records reach a terminal, documented state on boot
- [ ] No `context.Background()` anywhere in the request path (grep for it:
      `grep -rn "context.Background" internal/server internal/executor` — hits only allowed in main/recovery)
- [ ] `go test -race ./...` green

## Commit

```
day5: deadline propagation, classified retry with backoff, crash recovery
```
