.PHONY: build clean install test lint tidy release

BINARY := agent-proxy
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/agent-proxy

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/

test:
	go test ./...

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

# Usage: make release VERSION=v0.8.0
# Generates CHANGELOG.md from conventional commits, commits it,
# creates an annotated tag, and pushes everything to trigger GoReleaser.
release:
ifndef VERSION
	$(error VERSION is required. Usage: make release VERSION=v0.8.0)
endif
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not installed: brew install git-cliff"; exit 1; }
	@echo "→ Generating CHANGELOG.md..."
	git-cliff --config cliff.toml --tag $(VERSION) -o CHANGELOG.md
	@echo "→ Committing CHANGELOG..."
	git add CHANGELOG.md
	git commit -m "docs: CHANGELOG for $(VERSION)" --allow-empty
	@echo "→ Tagging $(VERSION)..."
	git tag -a $(VERSION) -m "$(VERSION)"
	@echo "→ Pushing to origin..."
	git push --no-verify origin main
	git push --no-verify origin $(VERSION)
	@echo ""
	@echo "✓ Released $(VERSION) — GoReleaser workflow triggered."
	@echo "  Monitor: gh run list --repo chiga0/agent-proxy --limit 1"
