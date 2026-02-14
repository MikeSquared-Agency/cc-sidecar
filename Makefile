.PHONY: build test lint ci install

build:
	go build -o bin/cc-sidecar .

install: build
	cp bin/cc-sidecar /usr/local/bin/cc-sidecar

test:
	go test ./... -v -race -count=1

lint:
	golangci-lint run ./...

ci: build test lint
