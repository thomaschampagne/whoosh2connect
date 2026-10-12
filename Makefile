BINARY := whoosh2connect
PKG := ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X whoosh2connect/internal/version.Version=$(VERSION) -X whoosh2connect/internal/version.Commit=$(COMMIT)

.PHONY: build test vet lint fmt tidy clean run

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

run:
	go run ./cmd/$(BINARY)

test:
	go test $(PKG)

vet:
	go vet $(PKG)

lint:
	golangci-lint run

fmt:
	gofmt -s -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
