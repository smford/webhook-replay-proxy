BINARY_NAME=webhook-replay-proxy
MAIN_PATH=./cmd/webhook-replay-proxy

.PHONY: all build test lint clean check

all: build test

build:
	CGO_ENABLED=0 go build -v -ldflags="-s -w" -o bin/$(BINARY_NAME) $(MAIN_PATH)

test:
	go test -v -race -covermode=atomic -coverprofile=coverage.out ./...

lint:
	go mod verify
	test -z "$$(gofmt -s -l .)"
	go vet ./...

check: lint test

clean:
	rm -rf bin/ dist/ coverage.out coverage.html webhooks.db
