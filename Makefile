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

.PHONY: install
install: ## Install the binary into GOPATH/bin
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
cover: ## Report test coverage and apply the same 95% gate CI does (demos excluded)
	@go test -coverprofile=coverage.out ./... >/dev/null
	@grep -v '/examples/' coverage.out > coverage.filtered.out
	@go tool cover -func=coverage.filtered.out | tail -1
	@pct=$$(go tool cover -func=coverage.filtered.out | awk '/^total:/ {print substr($$3, 1, length($$3)-1)}'); \
	  awk -v p="$$pct" 'BEGIN { exit (p+0 >= 95 ? 0 : 1) }' \
	  || { echo "coverage $$pct% is below 95%"; exit 1; }

.PHONY: bench
bench: ## Run benchmarks
	go test -run=NONE -bench=. -benchmem $(PKG)

.PHONY: fuzz
fuzz: ## Run each fuzz target briefly (FUZZTIME=30s make fuzz)
	@go test ./claims/ -run FuzzParse -fuzz FuzzParse -fuzztime $(or $(FUZZTIME),20s)
	@go test ./frontmatter/ -run FuzzSplit -fuzz FuzzSplit -fuzztime $(or $(FUZZTIME),20s)
	@go test ./frontmatter/ -run FuzzExtractMetadata -fuzz FuzzExtractMetadata -fuzztime $(or $(FUZZTIME),20s)
	@go test ./pipeline/ -run FuzzParseSurgicalEdits -fuzz FuzzParseSurgicalEdits -fuzztime $(or $(FUZZTIME),20s)

.PHONY: vuln
vuln: ## Scan for known vulnerabilities (same check as CI)
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

.PHONY: vet
vet: ## Run go vet
	go vet $(PKG)

.PHONY: fmt
fmt: ## Format the code
	gofmt -s -w .

.PHONY: lint
lint: ## Run golangci-lint if installed
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed; skipping"

.PHONY: tidy
tidy: ## Tidy module dependencies
	go mod tidy

.PHONY: check
check: fmt vet test ## Format, vet, and test

.PHONY: clean
clean: ## Remove build artefacts
	rm -rf $(BIN_DIR)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
	  awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
