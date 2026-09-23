# python/ — the Python track of gateway-week

Every **language-specific** artifact in gateway-week has a Python equivalent
here. Language-agnostic docs (architecture, states, idempotency, retry
policy, evidence checklist) are read as-is — the design does not change with
the language.

## The choice rule (read this first)

Your assignment brief says a **Go** gateway — build the gateway in Go.
Use this track to:

1. Understand every concept first in the language you are most fluent in.
2. Cross-train: the side-by-side Go/Python comparison is itself a skill.
3. Optionally build the mock server (`06`) or the practice server (`05`) in
   Python — same contracts, extra practice, and a Python-built mock still
   works perfectly with your Go gateway.

## Map: Go artifact → Python equivalent

| Go artifact | Python equivalent | What translates |
| ----------- | ----------------- | --------------- |
| concepts/04-context-and-deadlines.md | `01-context-and-deadlines.md` | `context.Context` ↔ `ServicerContext`, `Event`, asyncio |
| concepts/06-grpc-essentials.md | `02-grpc-essentials.md` | grpc-go ↔ grpcio: server, metadata, codes, interceptors, testing |
| concepts/07-observability.md | `03-observability.md` | slog ↔ stdlib logging, client_golang ↔ prometheus_client |
| assignments/day2-sunday.md (Go drills) | `04-drills-in-python.md` | interfaces, errors, goroutines/channels, cancellation — plus a Go↔Python Rosetta table |
| teaching/.../04-build-your-own-server.md | `05-build-devreg-python.md` | the devreg practice server, Python edition |
| mock-grpcServer/ (Go, built + tested) | `06-build-nafmock-python.md` | full build spec for a Python mock with identical contracts |

Concepts 01, 02, 03, 05 (gateway, states, idempotency, retry) have no Python
file because they are design docs; the few Go snippets in them translate via
`04`'s Rosetta table.

## Parity contract (why a mixed setup works)

Any Python build in this track MUST keep identical to its Go sibling:

- the `.proto` contract (same file, no edits)
- the behavior grammar (`ok`, `flaky:N`, `fail-retryable`, ...)
- gRPC status codes (`UNAVAILABLE` = retryable, etc.)
- admin API paths (`/healthz`, `/metrics`, `/history`, `/behavior`, `/reset`)
- metric family names (`nafmock_executions_total{operation}`, ...)
- ports and CLI flags

Result: `grpcurl` commands, demo scripts, and Friday evidence commands work
against either implementation unchanged. Build the Go gateway + Python mock,
or any mix — the demos don't care.

## Reading order

Doing the week in Go (you are), reading Python for fluency:

```
before day 2:  04 (drills — read alongside the Go drills)
before day 3:  02, then 05 (build devreg in Go OR Python, not both)
before day 5:  01 (deadlines)
before day 6:  03 (observability)
sometime:      06 (optional — build the Python mock)
```