.PHONY: build test lint generate cover snapshot docs

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

# Same package set and threshold logic as CI (scripts/coverage.sh).
COVER_THRESHOLD ?= 80

cover:
	scripts/coverage.sh $(COVER_THRESHOLD)

# Command reference (Markdown) from the German help; TestReferenceUpToDate
# fails until it is regenerated. Man pages are built at release time.
docs:
	go run ./cmd/papierkram gen-docs --dir docs/reference

snapshot:
	go run github.com/goreleaser/goreleaser/v2@v2.19.0 release --snapshot --clean
