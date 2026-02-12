GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

ifeq ($(GOHOSTOS), windows)
	#the `find.exe` is different from `find` in bash/shell.
	#to see https://docs.microsoft.com/en-us/windows-server/administration/windows-commands/find.
	#changed to use git-bash.exe to run find cli or other cli friendly, caused of every developer has a Git.
	#Git_Bash= $(subst cmd\,bin\bash.exe,$(dir $(shell where git)))
	Git_Bash=$(subst \,/,$(subst cmd\,bin\bash.exe,$(dir $(shell where git))))
	INTERNAL_PROTO_FILES=$(shell $(Git_Bash) -c "find internal -name *.proto")
	API_PROTO_FILES=$(shell $(Git_Bash) -c "find api -name *.proto")
else
	INTERNAL_PROTO_FILES=$(shell find internal -name *.proto)
	API_PROTO_FILES=$(shell find api -name *.proto)
endif

.PHONY: init
# init env
init:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
	go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
	go install github.com/google/gnostic/cmd/protoc-gen-openapi@latest
	go install github.com/google/wire/cmd/wire@latest

# ============================================
# Development Environment Commands
# ============================================

.PHONY: dev-up
# Start infrastructure services for local development (MySQL, Redis, MinIO, Envoy)
dev-up:
	docker-compose -f docker-compose.dev.yaml up -d
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@docker-compose -f docker-compose.dev.yaml ps

.PHONY: dev-down
# Stop infrastructure services
dev-down:
	docker-compose -f docker-compose.dev.yaml down

.PHONY: dev-logs
# View infrastructure services logs
dev-logs:
	docker-compose -f docker-compose.dev.yaml logs -f

.PHONY: dev-clean
# Stop infrastructure services and remove volumes (WARNING: data will be lost)
dev-clean:
	docker-compose -f docker-compose.dev.yaml down -v

.PHONY: run
# Run the application locally with kratos (infrastructure must be running)
# Usage: make run
# Or with custom config: make run CONF=./configs
run:
	@if ! docker-compose -f docker-compose.dev.yaml ps | grep -q "Up"; then \
		echo "⚠️  Infrastructure services not running. Starting them first..."; \
		make dev-up; \
		sleep 5; \
	fi
	kratos run -c $(or $(CONF),./configs)

.PHONY: run-go
# Run the application directly with go run (alternative to kratos run)
# Usage: make run-go
run-go:
	@if ! docker-compose -f docker-compose.dev.yaml ps | grep -q "Up"; then \
		echo "⚠️  Infrastructure services not running. Starting them first..."; \
		make dev-up; \
		sleep 5; \
	fi
	go run ./cmd/injective-explorer -conf $(or $(CONF),./configs)

.PHONY: config
# generate internal proto
config:
	protoc --proto_path=./internal \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./internal \
	       $(INTERNAL_PROTO_FILES)

.PHONY: api
# generate api proto
api:
	protoc --proto_path=./api \
	       --proto_path=./third_party \
 	       --go_out=paths=source_relative:./api \
 	       --go-http_out=paths=source_relative:./api \
 	       --go-grpc_out=paths=source_relative:./api \
	       --openapi_out=fq_schema_naming=true,default_response=false:. \
	       $(API_PROTO_FILES)

.PHONY: build
# build
build:
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./...

.PHONY: build-loadtest
# build loadtest tool
build-loadtest:
	mkdir -p bin/ && go build -o bin/loadtest ./cmd/loadtest

.PHONY: loadtest
# run loadtest (usage: make loadtest URL=http://localhost:8000 C=10 D=30s)
loadtest: build-loadtest
	@if [ -z "$(D)" ] && [ -z "$(N)" ]; then \
		echo "错误: 必须指定 D (持续时间) 或 N (请求数)"; \
		echo "示例: make loadtest D=30s C=10"; \
		echo "示例: make loadtest N=1000 C=10"; \
		exit 1; \
	fi
	@URL=$(or $(URL),http://localhost:8000); \
	C=$(or $(C),10); \
	if [ -n "$(D)" ]; then \
		./bin/loadtest -url $$URL -c $$C -d $(D) $(if $(ENDPOINT),-endpoint $(ENDPOINT)) $(if $(V),-v); \
	elif [ -n "$(N)" ]; then \
		./bin/loadtest -url $$URL -c $$C -n $(N) $(if $(ENDPOINT),-endpoint $(ENDPOINT)) $(if $(V),-v); \
	fi

.PHONY: generate
# generate
generate:
	go generate ./...
	go mod tidy

.PHONY: all
# generate all
all:
	make api;
	make config;
	make generate;

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
