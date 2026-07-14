.PHONY: dev down test test-int test-e2e test-all coverage-gate lint perf fmt tidy seed-dev help

GO_MODULES := ./apps/automate-api ./apps/automate-worker ./pkg/...

help: ## list targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

dev: ## docker-compose dev stack (postgres, temporal-dev, azurite, sftp-mock, mailhog)
	docker compose -f docker-compose.dev.yml up -d

down: ## stop dev stack
	docker compose -f docker-compose.dev.yml down

test: ## unit + coverage
	go test ./... -race -coverprofile=coverage.out -covermode=atomic
	cd apps/automate-web && pnpm vitest run --coverage

test-int: ## integration (testcontainers — ต้องมี docker)
	go test ./... -tags=integration -race -p 4 -timeout 20m

test-e2e: ## Playwright
	cd tests/e2e && pnpm playwright test

test-all: test test-int test-e2e coverage-gate ## everything

coverage-gate: ## fail if coverage below per-package threshold
	bash scripts/coverage-gate.sh

lint: ## golangci-lint + eslint + tsc
	golangci-lint run ./...
	cd apps/automate-web && pnpm eslint . && pnpm tsc --noEmit

fmt: ## gofmt + goimports
	gofmt -w apps pkg

tidy: ## go mod tidy across modules
	go work sync

seed-dev: ## seed local dev admin user
	go run ./apps/automate-api/cmd/seed

perf: ## k6 perf scenarios
	k6 run tests/perf/scenario-concurrent-runs.js
	k6 run tests/perf/scenario-filegen-100k.js
	k6 run tests/perf/scenario-scheduler-spike.js
