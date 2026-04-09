.PHONY: build test lint fmt tidy

build:
	go build -o ./bin/work-bridge ./cmd/work-bridge

test:
	go test ./...

lint:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

