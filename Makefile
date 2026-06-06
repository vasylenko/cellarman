BIN := cellarman
PKG := ./cmd/cellarman
REPO := vasylenko/cellarman
FORMULA := Formula/cellarman.rb

.PHONY: build run test test-integration vet fmt tidy clean release formula

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

release: ## tag + push VERSION to start a release (CI builds the formula + GitHub Release)
	@test -n "$(VERSION)" || { echo "usage: make release VERSION=vX.Y.Z"; exit 1; }
	git tag -a "$(VERSION)" -m "$(VERSION)"
	git push origin "$(VERSION)"
	@echo "tag pushed — the release workflow will refresh the formula and cut the GitHub Release"

formula: ## refresh formula url+sha256 from the published tag tarball (VERSION=vX.Y.Z); used by CI and offline releases
	@test -n "$(VERSION)" || { echo "usage: make formula VERSION=vX.Y.Z"; exit 1; }
	@url="https://github.com/$(REPO)/archive/refs/tags/$(VERSION).tar.gz"; \
	echo "hashing $$url"; \
	sha=$$(curl -fsSL --retry 5 --retry-delay 3 --retry-all-errors "$$url" | shasum -a 256 | cut -d' ' -f1); \
	test -n "$$sha" || { echo "could not fetch/hash tarball"; exit 1; }; \
	sed -i.bak -E 's|archive/refs/tags/v[0-9][0-9.]*\.tar\.gz|archive/refs/tags/$(VERSION).tar.gz|; s|sha256 "[a-f0-9]*"|sha256 "'$$sha'"|' "$(FORMULA)"; \
	rm -f "$(FORMULA).bak"; \
	echo "updated $(FORMULA) -> $(VERSION) ($$sha)"
