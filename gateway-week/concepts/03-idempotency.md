# 03 — Idempotency

## What it means

An operation is idempotent if executing it N times has the same effect as
executing it once. Network operations are not naturally idempotent — but you
can make the *gateway contract* idempotent by remembering what already happened.

The pattern: the client supplies an **idempotency key** (a UUID generated once
per logical operation, retried with the same key on failure). The gateway
records key -> outcome. Subsequent requests with the same key return the
recorded outcome without executing again.

This is how payment APIs (Stripe's `Idempotency-Key` header) avoid charging
cards twice when a client retries. You are building the same contract in gRPC
metadata.

## Key scope

The key alone is not enough. Full identity is:

```
(key, operation name, SHA-256(deterministic proto marshal of payload))
```

Three cases:

| Case                          | Behavior                                  |
| ----------------------------- | ----------------------------------------- |
| Same key + same payload       | Deduplicate: return recorded result       |
| Same key + different payload  | Reject — client bug or key reuse          |
| Different key                 | New operation, execute normally           |

Why hash the payload: a client that reuses a stale key for a different request
should get an explicit error (`ALREADY_EXISTS`-style), not a silently wrong
cached result. Never execute in this case.

Deterministic marshaling: use `proto.MarshalOptions{Deterministic: true}` so the
same payload always hashes the same. Without it, map iteration order can make
two identical messages hash differently.

## The race: why "check then execute" is not enough

Naive flow: read record; if none, execute. Two concurrent duplicates both read
"none", both execute. Duplicate execution — the exact bug this week exists to
prevent.

The fix is an **atomic compare-and-swap (CAS)** on the persisted state:

```sql
UPDATE operations SET state = 'EXECUTING', started_at = ?
WHERE id = ? AND state = 'VALIDATED';
```

- `RowsAffected == 1`: you won, you execute.
- `RowsAffected == 0`: someone else is executing or it already finished — you do
  NOT execute; you read the record and return its result/current state.

SQLite executes a single UPDATE atomically, so this is a real fence, not a
convention. (In-process per-key mutexes reduce contention, but the SQL CAS is
the correctness guarantee — the store would be safe even with multiple
gateway processes.)

## While in flight

A duplicate that arrives while the original is in `EXECUTING` should not fail.
Options, pick one and document:

1. Wait (with the caller's deadline) for completion and return the result.
2. Return the current state ("still executing") and let the client poll.

Option 1 is friendlier; option 2 is simpler. Both prevent duplicate execution.

## What idempotency does NOT protect

- Two *different* keys with identical payloads: that is two logical operations
  to the gateway. If TFS/NAF itself must deduplicate those, that is the
  platform's concern, documented as such.
- Retries *after a side effect started*: that is the in-doubt problem
  (concepts/02), not idempotency. Idempotency collapses duplicates at the
  gateway; in-doubt is what happens when the gateway loses contact mid-flight.

## Retention

Real systems expire idempotency records (24h is Stripe's default). Out of
scope this week, but note it in the design note as a known limitation.
