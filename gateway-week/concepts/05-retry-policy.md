# 05 — Retry policy

## The rule

Retry only failures that are **(a) explicitly classified retryable AND (b)
provably side-effect-free at the point of failure**. Everything else: fail
fast, persist the reason, let the caller decide.

Blind retries ("just retry 3 times on error") on a side-effecting operation is
how double-provisioning happens. The gateway's job is to be smarter than that.

## Classification

| Failure                          | Retryable? | Why                                        |
| -------------------------------- | ---------- | ------------------------------------------ |
| `UNAVAILABLE` from downstream    | Yes        | Connection refused/reset before work       |
| Connection refused / DNS failure | Yes        | Never reached the operation                |
| `INVALID_ARGUMENT`               | No         | Same input, same failure — wasted work     |
| `NOT_FOUND` / `ALREADY_EXISTS`   | No         | Deterministic response, not a fault        |
| Auth failures (`UNAUTHENTICATED`, `PERMISSION_DENIED`) | No | Retrying won't fix credentials |
| `DEADLINE_EXCEEDED` before execution started | Once, maybe | Only if time remains and nothing began |
| Failure **after execution began** | NO — in-doubt | Side effect may have happened; blind retry duplicates |

Implement classification as a function: `func Classify(err error) (retryable bool, reason string)`.
Test it table-driven. The executor adapter translates platform-specific errors
into this classification; the retry loop only consults `Classify`.

Note the asymmetry: whether a failure is retryable often depends on **where in
the protocol it happened** (before/after the side effect), not just the error
code. Your adapter to the real TFS/NAF operation is the only place that can
know — encode it there.

## Backoff with jitter

Fixed-interval retries hammer a struggling downstream in lockstep. The fix:

- Exponential base: e.g. 50ms, 100ms, 200ms...
- **Jitter**: randomize each wait (full jitter: uniform in [0, backoff)) so
  concurrent retries desynchronize.
- Bounded attempts (e.g. 3) and every wait ctx-aware:

```go
select {
case <-ctx.Done():
    return ctx.Err()          // deadline won — stop, do not start next attempt
case <-time.After(wait):
}
```

## The ambiguous case (in-doubt), restated

Timeout or connection drop *while the operation is executing* → you do not know
if TFS/NAF completed. This is NOT retryable — it is in-doubt. Handle per your
D1 policy (fail with reason `in_doubt`, or re-execute if the op is idempotent).
The retry loop must never be the thing that resolves ambiguity by accident.

## Retry budget / storm safety (know the terms)

- **Retry budget**: cap the fraction of calls that are retries (e.g. if >20% of
  calls to downstream are retries, stop retrying — the downstream is down and
  retries amplify the outage). Optional this week; note it in the design note.
- **Retry storm**: cascading synchronized retries across many clients. Jitter
  is the standard mitigation.

## What to record

Every attempt (at least attempt number, outcome, retryable classification,
wait before retry) goes into logs/metrics. Thursday's `gateway_retries_total`
counter and Friday's timeout demo both depend on the retry loop being
observable, not silent.

## The test that proves it

Wednesday's acceptance: a fake executor that fails twice with a retryable error
then succeeds → exactly 3 attempts, backoff gaps observable (assert the fake's
call timestamps are increasing and spaced), record ends SUCCEEDED. And the
mirror: a non-retryable failure → exactly 1 attempt, FAILED.
