BINARY  := animart
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build install test fmt vet check clean

all: check build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) .

install:
	CGO_ENABLED=0 go install -trimpath -ldflags '$(LDFLAGS)' .

test:
	go test -race ./...

fmt:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "gofmt: files need formatting"; exit 1; }

vet:
	go vet ./...

check: fmt vet test

clean:
	rm -f $(BINARY)
	rm -rf dist
