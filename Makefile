.DEFAULT_GOAL := help

GO ?= go
BUN ?= bun
DSN ?= host=localhost port=5432 user=optimus password=optimus dbname=optimus sslmode=disable
VERSION ?= $(shell git describe --always --dirty 2>/dev/null || echo dev)
BACKEND_TMP ?= $(CURDIR)/tmp
export TMPDIR := $(BACKEND_TMP)/work
export GOCACHE := $(BACKEND_TMP)/go-cache
export GOLANGCI_LINT_CACHE := $(BACKEND_TMP)/golangci-lint-cache
SWAG_DIFF_TMP := $(BACKEND_TMP)/swagger-diff
PERMS_DIFF_TMP := $(BACKEND_TMP)/permissions-diff.md

.PHONY: help backend-cache deps run build test test-int lint swag swagger-diff \
	migrate-up migrate-down migrate-status migrate-new seed dump-perms \
	perm-check perm-db-check web web-install web-dev web-check

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "; printf "Optimus development commands\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

backend-cache:
	@mkdir -p "$(TMPDIR)" "$(GOCACHE)" "$(GOLANGCI_LINT_CACHE)" "$(SWAG_DIFF_TMP)"

deps run build test test-int lint swag swagger-diff migrate-up migrate-down migrate-status migrate-new seed dump-perms perm-check perm-db-check: | backend-cache

deps: web-install ## Download modules and install pinned Go tools
	$(GO) mod download
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
	$(GO) install github.com/pressly/goose/v3/cmd/goose@v3.20.0
	$(GO) install github.com/swaggo/swag/cmd/swag@v1.16.4

run: ## Run the backend with go run (API on :8080)
	$(GO) run ./cmd/optimus server

build: web ## Build bin/optimus with the frontend embedded
	$(GO) build -ldflags "-s -w -X main.Version=$(VERSION)" -o bin/optimus ./cmd/optimus

web-install: ## Install locked frontend dependencies with Bun
	$(BUN) install --cwd web --frozen-lockfile

web: web-install ## Build the frontend
	$(BUN) run --cwd web build

web-dev: ## Run the Vite development server
	$(BUN) run --cwd web dev

web-check: ## Lint, typecheck, i18n parity and frontend tests
	$(BUN) run --cwd web lint
	$(BUN) run --cwd web typecheck
	$(BUN) run --cwd web i18n:check
	$(BUN) run --cwd web test

test: ## Run Go unit tests with race detection
	$(GO) test ./... -race -cover

test-int: ## Run Go database integration tests (dbtest tag)
	$(GO) test ./... -tags=dbtest -race -count=1

lint: ## Run golangci-lint
	golangci-lint run

swag: ## Regenerate Swagger artifacts
	swag init -g cmd/optimus/server.go -o api/docs --parseDependency --parseInternal
	cp api/docs/swagger.json docs/api/swagger.json

swagger-diff: ## Fail when committed Swagger artifacts are stale
	@swag init -g cmd/optimus/server.go -o "$(SWAG_DIFF_TMP)" --parseDependency --parseInternal >/dev/null
	@diff -q "$(SWAG_DIFF_TMP)/swagger.json" api/docs/swagger.json || \
	  (echo "embedded swagger.json is stale — run 'make swag' and commit"; exit 1)
	@diff -q "$(SWAG_DIFF_TMP)/swagger.json" docs/api/swagger.json || \
	  (echo "swagger.json is stale — run 'make swag' and commit"; exit 1)

migrate-up: ## Apply database migrations
	goose -dir migrations postgres "$(DSN)" up

migrate-down: ## Roll back the latest database migration
	goose -dir migrations postgres "$(DSN)" down

migrate-status: ## Show migration status
	goose -dir migrations postgres "$(DSN)" status

migrate-new: ## Create a new SQL migration (name=<snake_case>)
	@test -n "$(name)" || (echo "usage: make migrate-new name=<name>"; exit 1)
	goose -dir migrations create $(name) sql

seed: ## Seed permissions and the initial administrator
	$(GO) run ./cmd/optimus seed

dump-perms: ## Regenerate docs/permissions.md
	$(GO) run ./cmd/dump-permissions > docs/permissions.md

perm-check: ## Fail when docs/permissions.md is stale
	@$(GO) run ./cmd/dump-permissions > "$(PERMS_DIFF_TMP)"
	@diff -q "$(PERMS_DIFF_TMP)" docs/permissions.md || \
	  (echo "permissions.md is stale — run 'make dump-perms' and commit"; exit 1)

perm-db-check: ## Compare registered permissions against the database
	$(GO) run ./cmd/optimus server -check-permissions
