.PHONY: lint test test-integration build vet tidy run

VERSION ?= $(shell git describe --tags --always --dirty)

lint:
	golangci-lint run

test:
	go test -race -count=1 ./...

test-integration:
	HARBORMASTER_INTEGRATION=1 go test -tags=integration -count=1 ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build -ldflags="-X main.version=$(VERSION)" -o bin/harbormaster ./cmd/harbormaster

tidy:
	go mod tidy

run: build
	./bin/harbormaster serve
