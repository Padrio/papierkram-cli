.PHONY: build test lint generate cover snapshot

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/Padrio/papierkram-cli/internal/buildinfo.Version=$(VERSION) \
	-X github.com/Padrio/papierkram-cli/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/Padrio/papierkram-cli/internal/buildinfo.Date=$(DATE)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/papierkram ./cmd/papierkram

test:
	go test ./...

lint:
	golangci-lint run ./...

generate:
	go generate ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean
