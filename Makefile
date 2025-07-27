tidy:
	go mod tidy

example:
	go run ./

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

all: tidy vet fmt lint test e2e example
