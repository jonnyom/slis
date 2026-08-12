.PHONY: build test lint

build:
	./scripts/fetch-zmx.sh
	CGO_ENABLED=0 go build -o slis ./cmd/slis
	cp "third_party/zmx/dist/$$(go env GOOS)-$$(go env GOARCH)/zmx" zmx

test:
	go test ./...

lint:
	golangci-lint run
