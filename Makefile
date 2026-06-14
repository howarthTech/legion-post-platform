.DEFAULT_GOAL := help
SHELL := /usr/bin/env bash
export PATH := $(HOME)/.local/go/bin:$(PATH)
GO := $(shell command -v go 2>/dev/null || echo $$HOME/.local/go/bin/go)

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Compile bin/provision
	@$(GO) build -o bin/provision ./cmd/provision
	@echo "✓ bin/provision"

.PHONY: run
run: build ## Provision a client. Usage: make run SPEC=clients/post-5.yaml
	@if [ -z "$(SPEC)" ]; then echo "Usage: make run SPEC=clients/<name>.yaml"; exit 1; fi
	@./bin/provision -spec $(SPEC)

.PHONY: test
test: ## Run tests
	@$(GO) test ./...

.PHONY: vet
vet: ## go vet
	@$(GO) vet ./...

.PHONY: clean
clean: ## Remove build + generated output
	@rm -rf bin out
	@echo "✓ cleaned bin/ and out/"
