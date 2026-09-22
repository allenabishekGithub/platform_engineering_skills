# Day 1 (Saturday) — Operation state machine

**Read first:** concepts/01, concepts/02

**Goal:** the state machine type, transition rules, and table-driven tests —
the skeleton every later day hangs on. No gRPC, no storage yet.

## Tasks

1. Create the module and folders:
   ```bash
   mkdir -p ~/platform_engineering_skills/gateway-week/gateway
   cd ~/platform_engineering_skills/gateway-week/gateway
   go mod init github.com/<your-handle>/gateway-week/gateway
   mkdir -p internal/statemachine
   ```
2. In `internal/statemachine`, define:
   - `type State string` with constants: `StateReceived`, `StateValidated`,
     `StateExecuting`, `StateSucceeded`, `StateFailed`.
   - `type Reason string` for failure reasons: `validation`, `non_retryable`,
     `retries_exhausted`, `deadline`, `in_doubt`, `duplicate_conflict`.
   - A `transitions` map: `map[State][]State` encoding exactly:
     ```
     RECEIVED  -> VALIDATED | FAILED
     VALIDATED -> EXECUTING | FAILED
     EXECUTING -> SUCCEEDED | FAILED
     SUCCEEDED -> (terminal)
     FAILED    -> (terminal)
     ```
   - `func (s State) CanTransitionTo(next State) bool`
   - `func (s State) Terminal() bool`
3. Write the tests FIRST if you can (TDD practice), then make them pass.

## Hints

- Terminal-state check can be `len(transitions[s]) == 0` — or explicit; pick and justify.
- Table-driven test shape: `[]struct{ name string; from, to State; want bool }`
  covering EVERY legal edge and EVERY illegal edge (build the illegal list
  systematically: all pairs, minus legal).
- Exported constants in Go: `StateReceived State = "RECEIVED"` — string-backed
  so it persists readably in SQLite.

## Acceptance criteria

- [ ] All 4 legal edges and all illegal edges covered in one table-driven test
- [ ] Terminal states proven immutable (attempted transitions rejected)
- [ ] `go test -race ./internal/statemachine/` green
- [ ] You can explain out loud: why VALIDATED exists instead of jumping
      RECEIVED -> EXECUTING (answer: the CAS fence from concepts/03 needs a
      stable pre-execution state)

## Commit

```
day1: operation state machine with transition validation
```
