BINARY := draft
BIN_DIR := bin
PKG := ./...

.DEFAULT_GOAL := build

.PHONY: build
build: ## Compile the binary to ./bin/draft
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/$(BINARY) ./cmd/draft

.PHONY: install
install: ## Install the binary into GOPATH/bin
	go install ./cmd/draft

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
cover: ## Report test coverage (app + library packages; demos excluded)
	go test -cover ./internal/... ./cmd/...

.PHONY: bench
bench: ## Run benchmarks
	go test -run=NONE -bench=. -benchmem $(PKG)

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
