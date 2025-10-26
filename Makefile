GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

.PHONY: init
# init env
init:
	go install github.com/bufbuild/buf/cmd/buf@latest
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install github.com/google/wire/cmd/wire@latest

.PHONY: config
# generate internal proto
config:
	buf generate --template '{"version":"v1","plugins":[{"plugin":"go","out":".","opt":["paths=source_relative"]}]}' --path internal/infrastructure/configloader/pb

.PHONY: lint
# run static analysis (buf lint + staticcheck + revive)
lint:
	buf lint
	staticcheck -checks=all,-ST1000 ./...
	revive ./...

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/grpc ./cmd/grpc

.PHONY: test
# run unit tests
test:
	go test ./...

.PHONY: generate
# generate
generate:
	buf generate --path api
	sqlc generate
	go generate ./...
	go mod tidy

.PHONY: all
# generate all
all:
	$(MAKE) config
	$(MAKE) generate

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
