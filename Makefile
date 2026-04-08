.PHONY: build install test test-tmux test-e2e smoke fmt vet lint clean

PREFIX ?= $(HOME)/.local

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

build:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/muster ./cmd/muster

install: build
	install -d $(PREFIX)/bin
	install -m 0755 bin/muster $(PREFIX)/bin/muster
	ln -sf muster $(PREFIX)/bin/mst
	@echo "installed $(PREFIX)/bin/muster (with mst symlink)"

test:
	go test ./...

test-tmux:
	go test -tags=tmux ./...

test-e2e:
	go test -tags=e2e,tmux ./test/e2e/...

smoke:
	go test -tags=manual ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

lint: vet
	gofmt -l . | tee /dev/stderr | (! read)

clean:
	rm -rf bin/ coverage.out
