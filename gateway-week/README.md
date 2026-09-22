# gateway-week

One-week assignment: build a small Go **execution gateway** in front of one existing
TFS/NAF operation. Not a rewrite of the platform — a controlled, observable,
fault-tolerant front door for a single side-effecting operation.

## What you will build

A gRPC service that:

1. Accepts a structured operation through gRPC.
2. Attaches an idempotency key to every request.
3. Propagates deadlines using Go `context`.
4. Retries only explicitly retryable failures.
5. Persists operation state.
6. Prevents duplicate execution.
7. Exposes health and Prometheus metrics.
8. Has unit and failure-path tests.

## Folder map

```
gateway-week/
  README.md              <- you are here
  architecture.md        target design, diagram, the 4 core design decisions
  evidence-checklist.md  maps every "definition of done" item to a concrete artifact
  concepts/              WHAT this is and WHY (read the matching file before each day)
    01-execution-gateway.md
    02-operation-states.md
    03-idempotency.md
    04-context-and-deadlines.md
    05-retry-policy.md
    06-grpc-essentials.md
    07-observability.md
  assignments/           WHAT to do each day (tasks, acceptance criteria, commit)
    day1-saturday.md
    day2-sunday.md
    day3-monday.md
    day4-tuesday.md
    day5-wednesday.md
    day6-thursday.md
    day7-friday.md
```

## Code location

Docs live here. Your Go code goes in `platform_engineering_skills/flowops/gateway/`
so it stays part of the capstone repo. Target layout:

```
flowops/gateway/
  go.mod
  cmd/gateway/main.go          entrypoint
  internal/statemachine/       states + transitions
  internal/executor/           Executor interface + TFS/NAF adapter + fakes
  internal/store/              SQLite operation store
  internal/server/             gRPC service, interceptors, wiring
  internal/telemetry/          logging, metrics, correlation IDs
  api/gateway/v1/gateway.proto contract
```

## Schedule

| Day       | Focus                                   | Read first            |
| --------- | --------------------------------------- | --------------------- |
| Saturday  | Operation state machine                 | concepts/02           |
| Sunday    | Go drills: interfaces, errors, goroutines, context | concepts/01, 04 |
| Monday    | Protobuf contract + gRPC server        | concepts/06           |
| Tuesday   | Idempotency + persistent records       | concepts/03           |
| Wednesday | Timeout, retry, recovery                | concepts/05 (+04)     |
| Thursday  | Structured logs, metrics, correlation  | concepts/07           |
| Friday    | Failure tests + design documentation   | all                   |

## Working rules

1. **You write the code.** These docs tell you what and why, not the full how.
   Each assignment has "hints" — concepts to look up, not copy-paste code.
2. **Acceptance criteria are the contract.** Do not move to the next day until
   every box for the current day is checked and verified by running something.
3. **One commit per day**, message suggested at the end of each assignment.
   Friday you pick your most meaningful hash for the review evidence.
4. **Tests are code, not an afterthought.** Each assignment that adds behavior
   also adds its test in the same day.
5. Run `go test -race ./...` before every commit.

## Prerequisites

- Go >= 1.22 installed (`/usr/local/go/bin` on PATH)
- `protoc` + Go plugins (installed in day 3 instructions)
- Basic Docker (only needed if you want to containerize at the end — optional)

## Definition of done

See `evidence-checklist.md`. Every item there maps to one of the assignment
acceptance criteria — if you complete the days honestly, the evidence exists
automatically on Friday.
