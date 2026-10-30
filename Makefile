GO ?= go
BIN ?= adpm

.PHONY: build test vet

build:
	$(GO) build -o $(BIN) .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...
