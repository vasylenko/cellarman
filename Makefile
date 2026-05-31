BIN := brew-tui
PKG := ./cmd/brew-tui

.PHONY: build run test test-integration vet fmt tidy clean

build: ## build the binary into bin/
	go build -o bin/$(BIN) $(PKG)

run: ## run the TUI
	go run $(PKG)

test: ## unit + view tests (no real brew)
	go test ./...

test-integration: ## tests that exercise the real brew binary
	go test -tags integration ./internal/brew/

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin
