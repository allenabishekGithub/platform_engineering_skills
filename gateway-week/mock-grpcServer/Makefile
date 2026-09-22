MODULE := github.com/allenabishekGithub/platform_engineering_skills/gateway-week/mock-grpcServer

.PHONY: generate build test run

generate:
	protoc --go_out=. --go_opt=module=$(MODULE) \
	       --go-grpc_out=. --go-grpc_opt=module=$(MODULE) \
	       api/nafmock/v1/nafmock.proto

build:
	go build -o bin/nafmock ./cmd/nafmock

test:
	go test ./...

run: build
	./bin/nafmock
