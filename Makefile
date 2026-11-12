# Aegis developer commands. Run `make help` to list them.
SHELL := /bin/bash
COMPOSE := docker compose -f deploy/compose/compose.yaml --env-file .env
BIN := bin/aegis

.DEFAULT_GOAL := help

.PHONY: help
help: ## List available commands
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: setup
setup: ## First-time setup: create .env, download modules, check tools
	@test -f .env || (cp .env.example .env && echo "created .env from .env.example")
	go mod download
	@command -v docker >/dev/null || echo "WARNING: docker not found (needed for make up)"
	@command -v golangci-lint >/dev/null || echo "WARNING: golangci-lint not found (needed for make lint): https://golangci-lint.run/welcome/install/"

.PHONY: build
build: ## Build the aegis binary into bin/
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/aegis

.PHONY: test
test: ## Run unit tests with the race detector
	go test -race -count=1 ./...

.PHONY: integration
integration: .env ## Run integration tests against the local stack (needs make up)
	set -a; source .env; set +a; go test -tags integration -count=1 -v ./...

.PHONY: fmt
fmt: ## Format all Go code
	gofmt -w .

.PHONY: lint
lint: ## Run static analysis
	golangci-lint run ./...

.PHONY: check
check: ## Everything CI runs: format check, vet, lint, tests
	@test -z "$$(gofmt -l .)" || (echo "unformatted files:"; gofmt -l .; exit 1)
	go vet ./...
	golangci-lint run ./...
	go test -race -count=1 ./...

.PHONY: demo
demo: ## Run the ransomware attack -> detect -> recover demo
	./scripts/demo.sh

.PHONY: up
up: .env ## Start local stack (MinIO with Object Lock, Postgres)
	$(COMPOSE) up -d
	@echo "MinIO console: http://localhost:9001   Postgres: localhost:5432"

.PHONY: down
down: ## Stop local stack (keeps data)
	$(COMPOSE) down

.PHONY: nuke
nuke: ## Stop local stack and DELETE all its data
	$(COMPOSE) down -v

.PHONY: logs
logs: ## Follow local stack logs
	$(COMPOSE) logs -f

.PHONY: docker
docker: ## Build the aegis container image
	docker build -t aegis:latest .

.env:
	cp .env.example .env
