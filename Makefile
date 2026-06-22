.PHONY: build test lint

build:
	CGO_ENABLED=0 go build -o slis ./cmd/slis

test:
	go test ./...

lint:
	golangci-lint run
