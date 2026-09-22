# 04 — Build your own gRPC server (you do this one)

A minimal "device registry" unary service — small enough for one sitting,
expressive enough to exercise every layer you read about in 01-03. You write
all the code; each step tells you what to do and how to verify it before
moving on. This directly rehearses day 3 of gateway-week.

## Step 0 — toolchain check (all user-local on this machine)

```bash
export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH
go version          # expect go1.27.x
protoc --version    # expect libprotoc 29.x
which protoc-gen-go protoc-gen-go-grpc   # both in ~/go/bin
```

If any fail, stop and fix before proceeding (tell me what's missing).

## Step 1 — module and folders

```bash
mkdir -p ~/platform_engineering_skills/gateway-week/devreg/{api/devreg/v1,cmd/devreg,internal/server}
cd ~/platform_engineering_skills/gateway-week/devreg
go mod init github.com/allenabishekGithub/platform_engineering_skills/gateway-week/devreg
```

## Step 2 — write the contract (`api/devreg/v1/devreg.proto`)

You write it — no starter text — meeting this spec:

- `syntax`, `package devreg.v1`, `option go_package` with alias `devregv1`
  (copy the go_package pattern from `nafmock.proto` if unsure — but retype it,
  don't paste)
- `message RegisterRequest`: `string device_id = 1`, `string model = 2`
- `message RegisterResponse`: `string device_id`, `string registered_at`
- `message GetRequest`: `string device_id`
- `message GetResponse`: `string device_id`, `string model`, `bool registered`
- `service DeviceRegistry`: rpcs `Register(RegisterRequest) → RegisterResponse`
  and `Get(GetRequest) → GetResponse`

Before generating, self-review against file 02: snake_case? field numbers
unique and sequential? zero-value sane? rpc naming verb-noun?

## Step 3 — generate and READ the output

```bash
protoc --go_out=. --go_opt=module=github.com/allenabishekGithub/platform_engineering_skills/gateway-week/devreg \
       --go-grpc_out=. --go-grpc_opt=module=github.com/allenabishekGithub/platform_engineering_skills/gateway-week/devreg \
       api/devreg/v1/devreg.proto
```

Do not skip the reading part. In `devreg.pb.go` find: the `DeviceId` getter
and the struct tag `protobuf:"bytes,1,opt,name=device_id"`. In
`devreg_grpc.pb.go` find: the `DeviceRegistryServer` interface,
`UnimplementedDeviceRegistryServer`, and `RegisterDeviceRegistryServer`.
Being able to navigate generated code is the skill; the generation is trivia.

Commit the generated files. Add a `generate` script so regeneration is one
command (`make` is not installed on this machine — a `generate.sh` with the
protoc invocation works fine; `mock-grpcServer/Makefile` shows the command).

## Step 4 — implement the service (`internal/server/server.go`)

Mimic `nafmock`'s struct (file 03, layer 2-3). Requirements:

- `type Registry struct` embedding `UnimplementedDeviceRegistryServer`
- In-memory `map[string]string` (device_id → model) behind a `sync.Mutex`
- `Register`: reject empty device_id or model with `codes.InvalidArgument`;
  duplicate registration of an existing id with `codes.AlreadyExists`;
  otherwise store it, return `registered_at` as `time.Now().UTC().Format(time.RFC3339Nano)`
- `Get`: unknown id → `codes.NotFound` with the id in the message
  (programs get the code, humans debugging get context); known id → fill the
  response, `registered` = true
- Every response built via a struct literal; every failure via `status.Error(code, msg)`
- No `fmt.Println` anywhere — this service has no logging yet, that's fine

## Step 5 — wire the server (`cmd/devreg/main.go`)

Mimic `nafmock/cmd/nafmock/main.go` deliberately — have it open while you
type, but re-derive each block. Required pieces:

1. flags: `--grpc-addr :50053`, `--http-addr :9092`
2. gRPC server + your `Registry` + **health** + **reflection**
3. plain HTTP on the second port with just `/healthz` returning `ok`
4. `signal.NotifyContext`, `GracefulStop()` on signal, http `Shutdown` with
   a 5s timeout
5. `log/slog` JSON, one startup log line with both listen addresses

Get dependencies: `go get google.golang.org/grpc google.golang.org/grpc/health google.golang.org/grpc/reflection`

## Step 6 — build, run, prove it

```bash
go build ./... && go run ./cmd/devreg
```

In a second terminal (install once: `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`):

```bash
# discover via reflection — proves health+reflection registration
grpcurl -plaintext localhost:50053 list

# happy path
grpcurl -plaintext -d '{"device_id":"dev-1","model":"X100"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Register

# duplicate -> AlreadyExists (verify the exact code you chose)
grpcurl -plaintext -d '{"device_id":"dev-1","model":"X100"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Register

# validation -> InvalidArgument
grpcurl -plaintext -d '{"device_id":"","model":"X"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Register

# not found
grpcurl -plaintext -d '{"device_id":"dev-404"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Get
```

Verify graceful shutdown: `kill -TERM <pid>` on the running server; expect
the shutdown log line and exit code 0, and that an in-flight call (if any)
gets to finish.

## Step 7 — the deadline experiment (do not skip)

With the server running, hold a registration "open" — add a 2-second sleep
(`select` on `ctx.Done()` and `time.After`) at the top of `Register`:

```bash
grpcurl -plaintext -max-time 1 \
  -d '{"device_id":"dev-2","model":"X"}' \
  localhost:50053 devreg.v1.DeviceRegistry/Register
```

Two things to observe and write down:

1. grpcurl returns `DEADLINE_EXCEEDED` at ~1s — the deadline came from the
   client, no server code involved.
2. Your handler's sleep should be ctx-aware: with the select, the handler
   wakes early and returns; without it, it burns the full 2s doing work
   nobody will read. Change the sleep to a plain `time.Sleep(2s)` and
   compare the server logs. That gap is the entire lesson of file 01's
   "deadlines are only as real as the leaf call."

Then revert the sleep.

## Step 8 — unit tests with bufconn (`internal/server/server_test.go`)

Copy the harness shape from `nafmock`'s test file (it is the point of that
file existing — a template to steal) but write your own cases:

- table-driven: register-ok, register-duplicate, register-invalid, get-ok,
  get-not-found — each asserting the returned gRPC code via `status.Code(err)`
- one concurrency test: 10 goroutines registering the *same* id; exactly one
  OK, nine `AlreadyExists` (you have a mutex — this is the test that proves
  you needed it; run it and imagine it against your gateway's idempotency
  CAS on Tuesday)

`go test -race ./...` — note: on this machine `-race` needs cgo
(`CGO_ENABLED=1` plus a C compiler); if unavailable, run without and note it.

## Step 9 — reflect and connect (write `NOTES.md` in the devreg folder)

Answer in writing, briefly:

1. Which generated symbol would change if you added rpc `Delete` tomorrow,
   and what would `Registry` have to do to keep compiling? (Hint: the embed.)
2. Where in your handler would a deadline enforcement point need to go if
   Register did network I/O?
3. Map each devreg piece to its gateway-week equivalent: Register → Execute
   (+what the gateway adds: state, store, CAS), map+mutex → store, main.go
   wiring → same, bufconn tests → same but with scenarios from Friday's matrix.
4. What was hardest to derive from scratch vs. easy once you'd read nafmock?

## Definition of done for this track

- [ ] devreg builds, serves, survives all five grpcurl scenarios with the
      correct codes, shuts down cleanly on SIGTERM
- [ ] reflection `list` shows your service; deadline experiment observed and
      written down
- [ ] bufconn tests green, including the 10-goroutine duplicate test
- [ ] NOTES.md written
- [ ] committed: `teaching: devreg — grpc fundamentals practice`

Then day 3 (the real gateway contract) is a re-run of steps 2-6 with a
harder service behind them.
