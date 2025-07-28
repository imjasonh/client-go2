tidy:
	go mod tidy

build:
	go build ./...

test:
	go test -v -race ./...

e2e:
	go test -v -race -tags=e2e ./...

vet:
	go vet ./...

fmt:
	gofmt -s -w .

lint:
	golangci-lint run --fix ./...

.PHONY: all
all: tidy vet fmt lint build test e2e
