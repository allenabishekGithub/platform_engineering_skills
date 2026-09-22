# Day 7 (Friday) — Failure tests, demos, documentation

**Read first:** `evidence-checklist.md`, your own week

**Goal:** prove the system fails correctly, assemble the evidence, write the
design note. No new features today — gaps you discover get documented, not
necessarily fixed.

## Task 1 — The failure matrix (test sweep)

Write these as Go tests unless marked manual. Each has a named test:

| # | Scenario                        | Test type | Assertion focus                            |
| - | ------------------------------- | --------- | ------------------------------------------ |
| 1 | Concurrent duplicates           | test      | 1 execution, both callers coherent        |
| 2 | Duplicate after SUCCEEDED       | test      | cached result, 0 new executions           |
| 3 | Key reuse, different payload    | test      | ALREADY_EXISTS, nothing executed          |
| 4 | Retryable x2 then success       | test      | 3 attempts, backoff gaps                  |
| 5 | Non-retryable failure           | test      | 1 attempt, FAILED(reason)                 |
| 6 | Retries exhausted               | test      | FAILED(retries_exhausted), attempts == max|
| 7 | Client deadline 200ms vs slow op| test      | DEADLINE_EXCEEDED fast, no zombie retries |
| 8 | Client cancels mid-flight       | test      | CANCELLED, in-flight gauge back to 0      |
| 9 | kill -9 during EXECUTING        | manual    | restart -> in-doubt policy applied        |
| 10| Malformed/empty payload         | test      | INVALID_ARGUMENT, state never past RECEIVED |

Manual demo procedure for #9 (script it as `docs/demos.md`):
1. Start gateway with `SlowExecutor` (5s) and a file-backed store.
2. Fire Execute in background; confirm (logs/metrics) state EXECUTING.
3. `kill -9` the process.
4. Restart; observe recovery log; query GetOperation — record terminal per policy.

## Task 2 — Run and capture the evidence

Work through `evidence-checklist.md` top to bottom. Capture into
`flowops/gateway/docs/`:

- `samples.md` — log lines + metrics output (from Thursday, refreshed)
- `demos.md` — exact commands + transcripts for duplicate, timeout, restart
- `design-note.md` — the deliverable below

## Task 3 — The design note (deliverable)

`flowops/gateway/docs/design-note.md`, roughly 2 pages:

1. **Architecture** — your final mermaid diagram (adapt `gateway-week/architecture.md`
   to your real names) + one paragraph.
2. **The four decisions** (from `gateway-week/architecture.md` D1-D4): what you
   chose, why, and what it would take to change your mind. Especially D1:
   your in-doubt policy and its justification.
3. **Retryable vs non-retryable** — required paragraph: the classification table
   + why the after-side-effect case is never retried.
4. **What I would do next with more time** — honest list (async execution +
   polling API, retry budgets, record expiry, tracing, multi-process fencing).

## Task 4 — Final polish

- [ ] `go test -race ./...` and `go vet ./...` clean; golangci-lint if installed
- [ ] `flowops/gateway/README.md`: build, run, test, demo instructions
- [ ] Re-read your week's commits; pick the meaningful hash (Tuesday's, most likely)
- [ ] Verify every box in `evidence-checklist.md` has an artifact behind it

## Review rehearsal (30 min)

Answer these out loud, they map to what a reviewer asks:

1. Walk me through two identical concurrent requests — line by line, who wins, why.
2. Your gateway died mid-execution. What does the client see? What does the
   operator see? What state is the TFS/NAF op in?
3. Why is this failure not retried and this one is? Where in code is that decided?
4. How do you know (from the outside) the gateway is healthy and doing work?
5. Where does the deadline you propagate actually bottom out — what is the
   last call that honors it?

## Commit

```
day7: failure matrix tests, demos, design note
```

Then stop. Evidence assembled > features added. Good luck — report back with
the commit hash and the design note, and you will get a full review.
