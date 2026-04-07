.PHONY: build test test-tmux test-e2e smoke fmt vet lint clean

build:
	go build -o bin/ssf ./cmd/ssf

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
