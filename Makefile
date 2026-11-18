.PHONY: build clean test lint fmt install release-check snapshot help merge-collections-force merge-collections-keep merge-env-force merge-env-keep

BINARY_NAME=postOffice
VERSION?=dev
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

EXAMPLES_DIR=examples
MERGE_OUT_DIR=bin

build: ## Build the binary
	go build $(LDFLAGS) -o $(BINARY_NAME)

clean: ## Remove build artifacts
	rm -f $(BINARY_NAME)
	go clean

test: ## Run tests
	go test -v -race -coverprofile=coverage.out ./...

lint: ## Run linter
	golangci-lint run

fmt: ## Format code
	go fmt ./...

install: ## Install the binary
	go install $(LDFLAGS)

release-check: ## Validate .goreleaser.yaml (needs goreleaser)
	goreleaser check

snapshot: ## Build every release archive locally into dist/ without publishing (needs goreleaser)
	goreleaser release --snapshot --clean

merge-collections-force: build ## Merge example collections (api-testing-workflow.json + servicebus-events.json), src wins conflicts, grouped by collection name
	mkdir -p $(MERGE_OUT_DIR)
	./$(BINARY_NAME) -m $(EXAMPLES_DIR)/api-testing-workflow.json $(EXAMPLES_DIR)/servicebus-events.json --forceMerge -g -o $(MERGE_OUT_DIR)/merged-collection-force.json

merge-collections-keep: build ## Merge example collections (api-testing-workflow.json + servicebus-events.json), dst wins conflicts, grouped by collection name
	mkdir -p $(MERGE_OUT_DIR)
	./$(BINARY_NAME) -m $(EXAMPLES_DIR)/api-testing-workflow.json $(EXAMPLES_DIR)/servicebus-events.json --keep-original -g -o $(MERGE_OUT_DIR)/merged-collection-keep.json

merge-env-force: build ## Merge example environments (test-environment.json + servicebus-emulator.env.json), src wins conflicts
	mkdir -p $(MERGE_OUT_DIR)
	./$(BINARY_NAME) -m $(EXAMPLES_DIR)/test-environment.json $(EXAMPLES_DIR)/servicebus-emulator.env.json --forceMerge -o $(MERGE_OUT_DIR)/merged-env-force.json

merge-env-keep: build ## Merge example environments (test-environment.json + servicebus-emulator.env.json), dst wins conflicts
	mkdir -p $(MERGE_OUT_DIR)
	./$(BINARY_NAME) -m $(EXAMPLES_DIR)/test-environment.json $(EXAMPLES_DIR)/servicebus-emulator.env.json --keep-original -o $(MERGE_OUT_DIR)/merged-env-keep.json

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
