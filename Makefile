BIN := cellarman
PKG := ./cmd/cellarman
REPO := vasylenko/cellarman
FORMULA := Formula/cellarman.rb

.PHONY: build run test test-integration vet fmt tidy clean release

build: ## build the binary into bin/
	go build -o bin/$(BIN) $(PKG)

run: ## run the TUI
	go run $(PKG)

test: ## unit + view tests (no real brew)
	go test ./...

test-integration: ## tests that exercise the real brew binary (client + UI e2e)
	go test -tags integration ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

tidy:
	go mod tidy

clean:
	rm -rf bin

release: ## tag + push VERSION (e.g. make release VERSION=v0.1.0), then refresh the formula url+sha256
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=vX.Y.Z"; exit 1; }
	git tag -a "$(VERSION)" -m "$(VERSION)"
	git push origin "$(VERSION)"
	@url="https://github.com/$(REPO)/archive/refs/tags/$(VERSION).tar.gz"; \
	echo "hashing $$url"; \
	sha=$$(curl -fsSL "$$url" | shasum -a 256 | cut -d' ' -f1); \
	test -n "$$sha" || { echo "could not fetch/hash tarball"; exit 1; }; \
	sed -i '' -E 's|archive/refs/tags/v[0-9][0-9.]*\.tar\.gz|archive/refs/tags/$(VERSION).tar.gz|; s|sha256 "[a-f0-9]*"|sha256 "'$$sha'"|' "$(FORMULA)"; \
	echo "updated $(FORMULA) -> $(VERSION) ($$sha)"; \
	echo "review the diff, then: git commit -am 'cellarman $(VERSION)' && git push"
