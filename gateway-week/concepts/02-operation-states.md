# 02 — Operation states

## The state machine

```
            validate ok            executor picked up
 RECEIVED ───────────> VALIDATED ─────────────────────> EXECUTING
    │                      │                                │
    │ validate fails       │ CAS fails (duplicate          │ executor returns
    ▼                      │ won the race)                 ▼
 FAILED                (returns existing)        SUCCEEDED  FAILED
```

- `RECEIVED`: the request exists and is persisted. Nothing decided yet.
- `VALIDATED`: structurally valid, idempotency-checked, ready to execute.
- `EXECUTING`: the downstream call is in flight. The critical, dangerous state.
- `SUCCEEDED`: terminal. Result recorded.
- `FAILED`: terminal. Reason recorded (validation, non-retryable, exhausted
  retries, deadline, in-doubt).

Rules:

1. Exactly one terminal state per operation. Terminal states never change
   (with one deliberate exception — in-doubt recovery, below).
2. Transitions are explicit and validated. Illegal transition = bug = panic
   or error in tests, never a silent overwrite.
3. Every transition is persisted **with a timestamp** and, ideally, the
   correlation ID that caused it.

## Why persist the state machine

If state lived only in memory, a restart would erase the truth about operations
that are mid-flight, and a duplicate request after restart would re-execute.
Persisting the record gives you:

- Duplicate detection across restarts
- Recovery: on startup, read all records and reconcile
- Audit: what happened, when, why (feeds Thursday's metrics and Friday's demos)

## In-doubt: the hard part

A crash or timeout while in `EXECUTING` leaves the record **in-doubt**: the
gateway does not know whether TFS/NAF completed the operation.

You cannot solve this in the gateway. You choose a policy:

- **Fail-closed (at-most-once bias):** mark `FAILED` with reason `in_doubt`,
  surface for human/tooling reconciliation. Safe when the wrapped operation is
  expensive to reverse.
- **Re-execute (at-least-once bias):** allowed only if the downstream operation
  is itself idempotent or safely repeatable. Simpler recovery, riskier.

Document your choice and why (the semantics of the wrapped operation decide).
This is design decision D1, and it is the first thing a reviewer will probe.

## Delivery semantics, for your notes

- **at-most-once**: may drop, never duplicates. Achieved by never re-executing
  in-doubt operations.
- **at-least-once**: may duplicate, never drops. Achieved by re-executing.
- **effectively-once**: at-least-once execution + idempotency keys so duplicates
  collapse. What your gateway approximates for *new* requests.

Your gateway's contract: effectively-once for requests that reach it, with an
explicit, documented policy for the in-doubt window.

## Implementation shape (Saturday)

Model as a type and a transition map, not a switch statement scattered around:

- `type State string` with typed constants.
- `var transitions = map[State][]State{...}` — the allowed edges.
- `func (s State) CanTransitionTo(next State) bool`.
- Illegal transitions rejected by the store layer, which is the only component
  that writes state.

Test it table-driven: every legal edge, plus every illegal edge you can think
of, plus "terminal states never leave".
