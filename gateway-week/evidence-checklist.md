# Evidence checklist (definition of done)

Map each review-evidence item to a concrete artifact. Produce these as you go;
on Friday you only assemble, not scramble.

## 1. Repository or branch containing the gateway

- Branch `gateway-week` (or work on `main` if you prefer), repo =
  `platform_engineering_skills` with code under `gateway-week/gateway/`.
- Artifact: `git log --oneline` shows one commit per day.

## 2. One meaningful commit hash

- Natural candidate: Tuesday's idempotency + persistence commit.
- Artifact: paste the hash + `git show --stat <hash>` into the review notes.

## 3. Architecture diagram

- `gateway-week/architecture.md` already contains the mermaid diagram — replace
  the placeholder boxes with YOUR actual component/function names once built.
- Artifact: rendered diagram (mermaid renders on GitHub).

## 4. Passing unit and integration tests

- `go test -race ./...` green.
- Unit: state machine transitions, retry classification, idempotency logic.
- Integration: full gRPC round-trip via `bufconn`, including the duplicate and
  recovery scenarios.
- Artifact: paste test output into the design note.

## 5. Duplicate requests execute only once

Demo script (run it, paste output):

1. Start gateway with a fake executor that counts executions.
2. Fire two identical `Execute` calls concurrently (same idempotency key).
3. Show: both get the same result, executor counter == 1, store has one record.
4. Fire the same call again after completion: counter still 1.

## 6. Timeout and restart recovery

Timeout demo:

1. Fake executor sleeps 2s; call with a 200ms gRPC deadline.
2. Show: DEADLINE_EXCEEDED returned quickly, retries did not restart execution
   (check metrics/log lines), record is FAILED or in-doubt per your D1 choice.

Restart demo:

1. Fake executor sleeps 5s; start `Execute` (state reaches EXECUTING).
2. `kill -9` the gateway mid-execution.
3. Restart the gateway; show startup recovery: in-doubt record handled exactly
   as documented in your design note.

## 7. Sample structured logs and metrics

- Artifact: 5-10 JSON log lines showing correlation ID propagation across
  intercept/handler/executor.
- Artifact: `/metrics` output showing the metric families (see concepts/07 for
  the metric list) after running the duplicate + timeout demos.

## 8. Retryable vs non-retryable note

One paragraph each plus the classification table (concepts/05 has the
skeleton — you fill in your final decisions and why, referencing the wrapped
operation's real semantics where you know them).

## Assembly (Friday, ~1h)

1. `gateway-week/gateway/docs/design-note.md`: architecture pointer, decisions D1-D4,
   retry note, pasted test outputs, demo transcripts.
2. `gateway-week/gateway/docs/demos.md`: exact commands to reproduce each demo.
3. Final README for `gateway-week/gateway/`: what it is, how to run, how to test.
