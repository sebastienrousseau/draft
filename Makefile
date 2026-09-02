BINARY := draft
BIN_DIR := bin
PKG := ./...
# Local builds carry the same version a release would, derived from git.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := build

.PHONY: build
build: ## Compile the binary to ./bin/draft
	@mkdir -p $(BIN_DIR)
	go build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$(BINARY) ./cmd/draft

.PHONY: install-go
install-go: ## Install the binary into GOPATH/bin (see GNUmakefile for the FHS install)
	go install -ldflags '$(LDFLAGS)' ./cmd/draft

.PHONY: run
run: ## Run the CLI, e.g. make run ARGS='--help'
	go run ./cmd/draft $(ARGS)

.PHONY: test
test: ## Run the test suite
	go test $(PKG)

.PHONY: race
race: ## Run tests with the race detector
	go test -race $(PKG)

.PHONY: cover
cover: ## Report test coverage and apply the same 98% gate CI does (demos excluded)
	@go test -coverprofile=coverage.out ./... >/dev/null
	@grep -v '/examples/' coverage.out > coverage.filtered.out
	@go tool cover -func=coverage.filtered.out | tail -1
	@pct=$$(go tool cover -func=coverage.filtered.out | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	  awk -v p="$$pct" 'BEGIN { exit (p+0 >= 98 ? 0 : 1) }' \
	  || { echo "coverage $$pct% is below 98%"; exit 1; }

.PHONY: bench
bench: ## Run benchmarks
	go test -run=NONE -bench=. -benchmem $(PKG)

.PHONY: fuzz
fuzz: ## Run each fuzz target briefly (FUZZTIME=30s make fuzz)
	@go test ./claims/ -run FuzzParse -fuzz FuzzParse -fuzztime $(or $(FUZZTIME),20s)
	@go test ./frontmatter/ -run FuzzSplit -fuzz FuzzSplit -fuzztime $(or $(FUZZTIME),20s)
	@go test ./frontmatter/ -run FuzzExtractMetadata -fuzz FuzzExtractMetadata -fuzztime $(or $(FUZZTIME),20s)
	@go test ./pipeline/ -run FuzzParseSurgicalEdits -fuzz FuzzParseSurgicalEdits -fuzztime $(or $(FUZZTIME),20s)

.PHONY: mutation
mutation: ## Mutation-test the security-critical grounding gate
	go run github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0 unleash ./claims \
	  --workers 4 --test-cpu 1 --threshold-efficacy 100 --threshold-mcover 100

.PHONY: vuln
vuln: ## Scan for known vulnerabilities (same check as CI)
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: docs-lint
docs-lint: ## Lint the documentation exactly as CI does (markdown, spelling, links)
	npx --yes markdownlint-cli2
	python3 -m pip install --quiet --disable-pip-version-check codespell
	python3 -m codespell_lib
	python3 scripts/check-links.py

.PHONY: docs-fix
docs-fix: ## Auto-fix what can be fixed in the docs (table alignment)
	python3 scripts/align-tables.py

.PHONY: lint
lint: ## Run golangci-lint if installed
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed; skipping"; \
	fi

.PHONY: tidy
tidy: ## Tidy module dependencies
	go mod tidy

.PHONY: check
check: fmt vet test docs-lint ## Format, vet, test, and lint the docs

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BIN_DIR)

.PHONY: help
help: ## Show this help
	@# -h suppresses the filename prefix: MAKEFILE_LIST holds both this file
	@# and GNUmakefile, and without it awk reads the filename as the target.
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
