# Day 4 (Tuesday) — Idempotency and persistent operation records

**Read first:** concepts/03 (the whole file — the CAS section especially)

**Goal:** the SQLite store, idempotency enforcement, duplicate prevention —
the heart of the assignment. By tonight "duplicate requests execute only once"
is true and provable.

## Tasks

1. Dependency: `go get modernc.org/sqlite` (pure Go, no CGO — runs anywhere).
2. `internal/store/` — a `Store` interface (remember the ports idea):
   ```go
   type Record struct { ID, Key, OpName, PayloadHash string; State statemachine.State;
                        Reason statemachine.Reason; Result []byte; CreatedAt, UpdatedAt time.Time }
   type Store interface {
       Create(ctx, Record) error                     // insert as RECEIVED
       GetByKey(ctx, key, opName, payloadHash) (*Record, error)
       Transition(ctx, id, from, to, reason) error   // THE CAS
       UpdateResult(ctx, id, state, result) error
       InExecuting(ctx) ([]Record, error)             // for Wednesday recovery
   }
   ```
3. SQLite implementation:
   - `CREATE TABLE operations (...)` with PK id, unique index on
     (key, op_name, payload_hash) — wait: different payload with same key
     must NOT be blocked by a unique index; index on (key, op_name) only,
     and compare payload_hash in code. Think this through; it is a real
     design step.
   - `db.SetMaxOpenConns(1)` (SQLite single-writer; simplest correct config)
     and `PRAGMA journal_mode=WAL`.
   - CAS via `UPDATE operations SET state=?1, updated_at=?2
     WHERE id=?3 AND state=?4`; check `RowsAffected == 1`, else return a
     sentinel `ErrLostRace` (or fetch current state — your choice, document).
4. Payload hashing (concepts/03): `sha256` over
   `proto.MarshalOptions{Deterministic: true}` output.
5. Wire the handler: validation -> (RECEIVED -> VALIDATED -> EXECUTING via
   CAS) -> call the `Executor` you inject (today: a fake from `internal/executor`).
   Duplicate paths per concepts/03: completed result vs in-flight vs key/payload
   conflict (`ALREADY_EXISTS`).
6. Tests:
   - Restart persistence: create store on a temp file, insert, close, reopen,
     record still there.
   - Concurrent duplicates: two goroutines, same key+payload, one CAS wins —
     assert the fake executor ran once. (This is Tuesday's big test.)
   - Key reuse with different payload -> `ALREADY_EXISTS`, zero executions.

## Hints

- Handler flow only ever moves state through the Store — never keeps state
  in a local variable across awaits. The store is the truth.
- In-flight duplicates: implement the simpler option first (return current
  state "EXECUTING"), note option 2 (wait for completion) in code comments as
  future work.
- Use `t.TempDir()` for the SQLite file; never test against a shared file.
- Every store method takes ctx and passes it to db calls.

## Acceptance criteria

- [ ] Concurrent same-key test: exactly one execution, both callers get coherent answers
- [ ] Same key + different payload rejected with ALREADY_EXISTS, nothing executed
- [ ] Duplicate after completion returns recorded result, no second execution
- [ ] Store survives close/reopen with all fields intact
- [ ] No state transitions happen outside the store's CAS path
- [ ] `go test -race ./...` green — the race detector is your duplicate bug finder

## Commit

```
day4: persistent operation store, idempotency CAS, duplicate prevention
```

This is your candidate "meaningful commit hash" for the review.
