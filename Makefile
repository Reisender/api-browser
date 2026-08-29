.PHONY: build test run lint

build:
	go build -o bin/apibrowser ./cmd/apibrowser

test:
	go test ./... -cover

lint:
	go vet ./...
	gofmt -l .

run: build
	./bin/apibrowser $(ARGS)
