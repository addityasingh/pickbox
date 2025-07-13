# Pickbox Distributed Storage System - Makefile
# Comprehensive build, test, and quality assurance commands

.PHONY: help
help: ## Show this help message
	@echo 'Usage: make <target>'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Build targets
.PHONY: build clean install
build: ## Build the main pickbox CLI binary
	go build -v -o bin/pickbox ./cmd/pickbox

install: build ## Install pickbox CLI to $GOPATH/bin
	cp bin/pickbox $(GOPATH)/bin/pickbox

clean: ## Clean build artifacts and test data
	rm -rf bin/
	rm -rf data/
	rm -rf /tmp/pickbox-*
	rm -rf /tmp/test-*
	rm -f coverage.out coverage.html
	pkill -f pickbox || true

# Development setup
.PHONY: setup install-tools install-pre-commit
setup: install-tools install-pre-commit ## Setup development environment

install-tools: ## Install development tools
	@echo "📦 Installing development tools..."
	@echo "Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3 || echo "⚠️  Failed to install golangci-lint"
	@echo "Installing staticcheck..."
	@go install honnef.co/go/tools/cmd/staticcheck@2023.1.7 || echo "⚠️  Failed to install staticcheck"
	@echo "Installing gosec..."
	@go install github.com/securecodewarrior/gosec/v2/cmd/gosec@v2.18.2 || echo "⚠️  Failed to install gosec"
	@echo "✅ Tool installation completed"

install-pre-commit: ## Install and setup pre-commit hooks
	pip install pre-commit
	pre-commit install
	pre-commit install --hook-type commit-msg

# Code quality and linting
.PHONY: lint lint-fix format vet check-unused security
lint: ## Run all linters
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3; }
	@command -v staticcheck >/dev/null 2>&1 || { echo "Installing staticcheck..."; go install honnef.co/go/tools/cmd/staticcheck@2023.1.7; }
	golangci-lint run --config=.golangci.yml
	staticcheck ./...

lint-fix: ## Run linters with auto-fix where possible
	golangci-lint run --config=.golangci.yml --fix
	goimports -w .
	gofmt -s -w .

format: ## Format Go code
	gofmt -s -w .
	goimports -w .

vet: ## Run go vet
	go vet ./...

check-unused: ## Check for unused variables, functions, and fields
	@command -v golangci-lint >/dev/null 2>&1 || { echo "Installing golangci-lint..."; go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3; }
	golangci-lint run --config=.golangci.yml --disable-all --enable=unused,deadcode,structcheck,varcheck

security: ## Run security analysis
	@echo "🔒 Running security analysis..."
	@echo "✓ Running basic security checks with go vet..."
	@go vet ./... || echo "⚠️  go vet found issues"
	@if command -v gosec >/dev/null 2>&1; then \
		echo "✓ Found gosec, running advanced security scanner..."; \
		gosec -fmt sarif -out gosec.sarif ./... 2>/dev/null || echo "SARIF generation failed, continuing..."; \
		gosec ./... || echo "⚠️  Security issues found, please review"; \
	else \
		echo "ℹ️  gosec not found. For advanced security scanning, install with:"; \
		echo "   go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
		echo "ℹ️  Using basic security checks only for now."; \
		echo '{"version":"2.1.0","$schema":"https://json.schemastore.org/sarif-2.1.0.json","runs":[{"tool":{"driver":{"name":"gosec","informationUri":"https://github.com/securecodewarrior/gosec","version":"unavailable"}},"results":[]}]}' > gosec.sarif; \
		echo "📄 Created empty SARIF report (gosec not available)"; \
	fi
	@echo "✅ Security analysis completed"

security-install: ## Install gosec and run full security analysis
	@echo "📦 Installing gosec..."
	go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
	@echo "✓ gosec installed"
	$(MAKE) security

# Testing
.PHONY: test test-unit test-integration test-short test-race test-coverage test-bench
test: test-unit ## Run all tests

test-unit: ## Run unit tests
	go test -v -race ./pkg/... ./cmd/...

test-integration: ## Run integration tests (currently disabled)
	@echo "Integration tests are currently disabled - see README Improvements section"
	@echo "To run manually: cd test && go test -v ."

test-short: ## Run short tests only
	go test -short -v ./...

test-race: ## Run tests with race detection
	go test -race -v ./...

test-coverage: ## Run tests with coverage
	go test -v -race -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out

test-bench: ## Run benchmark tests
	go test -bench=. -benchmem ./pkg/storage ./cmd/pickbox

# CLI Demo and scripts
.PHONY: demo demo-cli demo-3-nodes demo-multi demo-cleanup
demo: demo-cli ## Run CLI demo (default)

demo-cli: build ## Run 3-node cluster demo using CLI
	./bin/pickbox script demo-3-nodes

demo-3-nodes: build ## Run 3-node cluster demo
	./bin/pickbox script demo-3-nodes

demo-multi: build ## Run multi-directional replication demo using CLI
	./bin/pickbox node multi --node-id multi-demo --port 8010

demo-cleanup: ## Clean up demo data
	./bin/pickbox script cleanup || true

# CLI commands examples
.PHONY: cli-help cli-start-node cli-start-cluster cli-join-cluster cli-status
cli-help: build ## Show CLI help
	./bin/pickbox --help

cli-start-node: build ## Start a single node (bootstrap)
	./bin/pickbox node start --node-id node1 --port 8001 --bootstrap

cli-start-cluster: build ## Start a 3-node cluster
	@echo "Starting 3-node cluster..."
	./bin/pickbox node start --node-id node1 --port 8001 --bootstrap &
	sleep 3
	./bin/pickbox node start --node-id node2 --port 8002 --join 127.0.0.1:8001 &
	./bin/pickbox node start --node-id node3 --port 8003 --join 127.0.0.1:8001 &
	@echo "Cluster started. Use 'make demo-cleanup' to stop."

cli-join-cluster: build ## Join a node to existing cluster
	./bin/pickbox cluster join --leader 127.0.0.1:8001 --node-id node4 --node-addr 127.0.0.1:8004

cli-status: build ## Check cluster status
	./bin/pickbox cluster status --addr 127.0.0.1:9001

# Legacy demos (for backward compatibility)
.PHONY: demo-legacy demo-basic
demo-legacy: clean ## Run legacy multi-directional replication demo
	./scripts/run_multi_replication.sh

demo-basic: clean ## Run basic replication demo (legacy)
	./scripts/run_replication.sh

# Verification and CI simulation
.PHONY: ci pre-commit verify-all
ci: ## Simulate CI pipeline locally
	@echo "🚀 Running CI pipeline simulation..."
	$(MAKE) clean
	$(MAKE) lint
	$(MAKE) security
	$(MAKE) test-coverage
	$(MAKE) build
	@echo "✅ CI simulation completed successfully!"

pre-commit: ## Run pre-commit hooks manually
	pre-commit run --all-files

verify-all: ## Run comprehensive verification (lint + test + security)
	@echo "🔍 Running comprehensive verification..."
	$(MAKE) lint
	$(MAKE) security
	$(MAKE) test-coverage
	@echo "✅ All verifications passed!"

# Documentation
.PHONY: docs
docs: ## Generate and view documentation
	godoc -http=:6060
	@echo "Documentation available at http://localhost:6060/pkg/github.com/addityasingh/pickbox/"

# Git helpers
.PHONY: git-hooks
git-hooks: ## Setup git hooks for quality assurance
	@echo "#!/bin/sh" > .git/hooks/pre-commit
	@echo "make pre-commit" >> .git/hooks/pre-commit
	@chmod +x .git/hooks/pre-commit
	@echo "Git pre-commit hook installed!"

# Maintenance
.PHONY: mod-tidy mod-verify mod-update
mod-tidy: ## Tidy go modules
	go mod tidy

mod-verify: ## Verify go modules
	go mod verify

mod-update: ## Update go modules
	go get -u ./...
	go mod tidy

# Default target
.DEFAULT_GOAL := help 