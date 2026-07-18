# MyWork Automate

Node-based, banking-grade workflow automation engine. Design a flow on a canvas
(schedule → query → generate file → deliver via MFT/email/download), publish it
with immutable versioning, and let a durable Temporal-backed engine run it.

Full specification: [`docs/spec/`](docs/spec/) — **read
[`09-phase-plan.md`](docs/spec/09-phase-plan.md) first.** All current work is
**Phase 1 (MVP)**.

## Layout

| Path | What |
|---|---|
| `apps/automate-api` | Go 1.22+ (Gin) REST control plane |
| `apps/automate-worker` | Go, Temporal worker (node executors) |
| `apps/automate-web` | Next.js 14 App Router UI (`/automate`) |
| `internal/config` | shared, validated runtime config + local-auth guard |
| `pkg/` | shared Go packages (expression, masking, sqlguard, templaterender, filestore, secrets) |
| `deploy/charts/automate` | Helm chart |
| `tests/e2e` · `tests/perf` | Playwright · k6 |
| `scripts/` | coverage gate, policy checks |

## Onboarding (target ≤ 30 min)

Prerequisites: Go ≥ 1.22, Node ≥ 20 + `pnpm`, Docker, `make`. A C toolchain is
required for `pkg/sqlguard` (cgo `pg_query`); macOS: Xcode CLT, Linux: `build-essential`.

```bash
# 1. bring up the dev stack (postgres, temporal-dev, azurite, sftp-mock, mailhog)
make dev

# 2. run the unit suite with coverage
make test

# 3. enforce coverage thresholds (100% on core security pkgs, 95% Go, 90% FE)
make coverage-gate

# 4. run the API locally (dev defaults; local auth allowed only in dev)
AUTH_LOCAL_ENABLED=true go run ./apps/automate-api/cmd/api
curl -s localhost:8080/healthz
```

`make help` lists every target. To set the stack up on a **fresh/other machine**
(prerequisites, infra, DB migration, smoke test, protected‑env notes) see
[`docs/RUNNING.md`](docs/RUNNING.md). Standalone DB migration:
`scripts/migrate.sh "<postgres-url>"`.

## Non-negotiables (see [`CLAUDE.md`](CLAUDE.md))

- **TDD only** — test first, watch it fail, implement, refactor.
- **Coverage gates** — `pkg/masking`, `pkg/sqlguard`, `pkg/expression`,
  `pkg/templaterender` = **100%**; other Go ≥ 95%; FE components/hooks ≥ 90%.
- **SQL** — parameterized only; SQL mode goes through `pkg/sqlguard` (SELECT-only)
  with an injection corpus.
- **Secrets** — via `pkg/secrets` only; never hardcode. `gitleaks` runs in CI.
- **RBAC** — every API handler behind RBAC middleware, deny-by-default, per-role tests.
- **Masking** — external-DB responses pass the masking interceptor, with proving tests.
- **Local auth** — `AUTH_LOCAL_ENABLED=true` is refused at runtime in `sit`/`uat`/`prod`
  (see `internal/config`) and blocked in CI by `scripts/policy-check-local-auth.sh`.

## Status

Phase 0 (bootstrap) and E1-S1 (scaffold) are in place and green. See the
delivery report / `docs/` for the per-story build status and remaining backlog.
