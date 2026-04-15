.PHONY: build test lint fmt tidy run release test-e2e

# Version is injected via -ldflags at build time.
# Falls back to "dev" when built without the flag (e.g. go run or go test).
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

PKG      := github.com/jaeyoung0509/work-bridge/internal/cli
LDFLAGS  := -X '$(PKG).Version=$(VERSION)' \
            -X '$(PKG).Commit=$(COMMIT)' \
            -X '$(PKG).BuildDate=$(BUILD_DATE)'

build:
	go build -ldflags "$(LDFLAGS)" -o ./bin/work-bridge ./cmd/work-bridge

run: build
	./bin/work-bridge

test:
	go test ./...

lint:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

# Run E2E tests (local only, not in CI)
test-e2e:
	@echo "Running E2E tests (local only)..."
	@echo "These tests build the binary and test real CLI behavior."
	@echo "Skipping automatically when CI=true or GITHUB_ACTIONS=true"
	@if [ -z "$$CI" ] && [ -z "$$GITHUB_ACTIONS" ]; then \
		WORKBRIDGE_E2E=1 go test -tags=e2e ./tests/e2e/... -v; \
	else \
		echo "Skipping E2E tests in CI environment."; \
	fi

# Create a release binary for the current VERSION.
# Usage: make release VERSION=v0.1.8
release: tidy test
	go build -ldflags "$(LDFLAGS)" -trimpath -o ./bin/work-bridge ./cmd/work-bridge
	@echo "Built work-bridge $(VERSION) ($(COMMIT))"
