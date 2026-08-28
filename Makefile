GO        ?= go
BIN       ?= bin/api
PKG       ?= ./...
GOPATH    ?= $(shell $(GO) env GOPATH)
GOROOT    ?= $(shell $(GO) env GOROOT)
export PATH := $(GOROOT)/bin:$(GOPATH)/bin:$(PATH)

.PHONY: run test migrate lint build tidy vet fmt docker

build:
	$(GO) build -o $(BIN) ./cmd/api

run: build
	$(GO) run ./cmd/api

test:
	$(GO) test $(PKG) -count=1 -timeout 120s

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)

tidy:
	$(GO) mod tidy

migrate: build
	$(GO) run ./cmd/api -migrate-only

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "installing golangci-lint"; \
		$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest; \
	fi
	golangci-lint run ./...

docker:
	docker compose up --build
