.PHONY: all build test race lint vet fmt cover tidy clean quality bootstrap-health

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
	rm -f $(BIN) cov.out coverage.out

# Full quality gate with the temporary coverage ratchet (fmt, vet, lint, race+coverage).
# Delegates to the Claude Code hook script so `make quality` and the hooks stay in sync.
quality:
	bash .claude/scripts/quality-gate.sh --enforce

# Verify/repair the local Go MCP bootstrap state.
bootstrap-health:
	bash .claude/scripts/bootstrap-local-state.sh --verify --strict
