.PHONY: build clean test lint fmt install help

BINARY_NAME=postOffice
VERSION?=dev
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.Version=$(VERSION) \
	-X main.GitCommit=$(GIT_COMMIT) \
	-X main.BuildDate=$(BUILD_DATE)"

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

help: ## Display this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
