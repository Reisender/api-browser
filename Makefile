.PHONY: build test run lint

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/apibrowser ./cmd/apibrowser

test:
	go test ./... -cover

lint:
	go vet ./...
	gofmt -l .

run: build
	./bin/apibrowser $(ARGS)
