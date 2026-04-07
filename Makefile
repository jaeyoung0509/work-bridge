.PHONY: build test lint fmt tidy

build:
	go build -o ./bin/sessionport ./cmd/sessionport

test:
	go test ./...

lint:
	go vet ./...

fmt:
	go fmt ./...

tidy:
	go mod tidy

