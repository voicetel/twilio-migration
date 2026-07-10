.PHONY: all build test race lint vet fmt cover tidy clean

BIN := twilio-migration
PKG := ./...

all: fmt vet test build

build:
	go build -o $(BIN) ./cmd/twilio-migration

test:
	go test $(PKG)

race:
	go test -race -covermode=atomic -coverprofile=cov.out $(PKG)

cover: race
	go tool cover -func=cov.out

vet:
	go vet $(PKG)

fmt:
	gofmt -w .
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

# Optional: requires golangci-lint on PATH.
lint:
	golangci-lint run $(PKG)

tidy:
	go mod tidy

clean:
	rm -f $(BIN) cov.out
