# Developing MyWork Automate

How to work on this codebase: architecture, layout, the TDD workflow, the quality
gates, and the recipes for the changes you'll make most often. To *run* the stack
(infra, DB migration, smoke test) see [`RUNNING.md`](RUNNING.md); for the product
rules see [`../CLAUDE.md`](../CLAUDE.md); for the full spec see [`spec/`](spec/).

## 1. One-time setup

```bash
# infra (postgres, temporal-dev, azurite, sftp-mock, mailhog)
make dev
# frontend deps
( cd apps/automate-web && pnpm install )
# everything green?
make test && make coverage-gate
```

Prereqs: Go ≥ 1.25 (toolchain `go1.26.5` auto-fetched), a **C toolchain** (cgo for
`pkg/sqlguard`'s `pg_query`), Node ≥ 20 + pnpm, Docker, make. `golangci-lint` for
linting (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`).

## 2. Architecture at a glance

```
Browser ──HTTP──▶ automate-api (Gin)  ──Temporal──▶ automate-worker
   │  (Next.js)      │  control plane        starts    │  node executors
   │                 │  REST + RBAC + audit  workflow   │  db.query / file / delivery
   │                 ▼                                   ▼
   │            Postgres (control-plane +           Temporal (durable engine)
   │            connections + audit + versions)     external DBs (per-connection pools)
   └── SSO cookie / Bearer JWT ─────────────────────────┘
```

- **automate-api** (`apps/automate-api`) — the control plane. Serves the REST API,
  runs migrations + seed on startup, enforces RBAC + masking, writes the audit
  trail, and starts runs on Temporal. Stateless except for the Postgres pool.
- **automate-worker** (`apps/automate-worker`) — a Temporal worker. The interpreter
  *workflow* walks the flow graph; one *activity* (`ExecuteNode`) runs each node
  through its executor. Durable: a crash resumes from the last completed node.
- **automate-web** (`apps/automate-web`) — Next.js 16 App Router UI at `/automate`
  (React 19). Talks to the Go API via `NEXT_PUBLIC_API_BASE`. The `src/app/api/**`
  routes are local mocks for standalone FE dev; production uses the Go API.
- **Shared Go packages** (`pkg/*`) — the security-critical, pure, 100%-tested core:
  `sqlguard` (SELECT-only parse-tree guard), `masking` (column masking), `secrets`
  (Key Vault / file resolver + writer), `expression`, `templaterender`, plus
  `filegen`, `filestore`, `mailer`, `authz`, `logscrub`, `obs`.
- **internal/** — `config` (validated env + the local-auth protected-env guard),
  `flowspec` (the wire FlowDef/NodeDef/FlowInput shared by api ↔ worker).

## 3. Repository layout

| Path | What |
|---|---|
| `apps/automate-api/cmd/api` | API entrypoint / composition root (wires pool, auth, secrets, dialer, Temporal). |
| `apps/automate-api/cmd/seed` | dev-admin seed guard (real seeding is in `internal/store/postgres`). |
| `apps/automate-api/internal/httpapi` | Gin router, handlers, RBAC middleware, auth (cookie/bearer), connection endpoints. |
| `apps/automate-api/internal/store` | `Store` interface + models; `store/postgres` (pgx impl + `Migrate`/`Seed`); `store/migrations` (embedded `*.sql`). |
| `apps/automate-api/internal/{audit,scheduler,runner,preview,nodes,notify,lifecycle,flowvalidate}` | audit trail, schedule sync, Temporal run adapter, query-preview pool, node catalog, run-fail email, state machine, graph validation. |
| `apps/automate-worker/cmd/worker` | worker entrypoint (wires executors, secrets, per-connection pool cache). |
| `apps/automate-worker/internal/interpreter` | Temporal workflow + the `ExecuteNode` activity + node config shapes. |
| `apps/automate-worker/internal/executors/dbquery` | the db.query executor (sqlguard + secrets + masking + per-connection dial). |
| `apps/automate-worker/internal/pgxquerier` | pgx-backed `Querier` (statement timeout, pool). |
| `pkg/*` | shared Go packages (see §2). |
| `apps/automate-web/src` | `app/` (routes), `features/` (canvas, node-config, connections), `api/` (client + hooks), `components/ui`, `lib/`. |
| `deploy/charts/automate` | Helm chart. `scripts/` | coverage gate, policy checks, `migrate.sh`, `seed-employees.sql`. |
| `tests/e2e` · `tests/perf` | Playwright · k6. |

## 4. The non-negotiable workflow (TDD)

1. **Write the test first**, watch it fail, then implement, watch it pass, refactor.
2. Cover happy path, edge cases, error paths, **and security cases**.
3. Keep the gates green before you call anything done:

```bash
make lint            # golangci-lint + eslint (flat config) + tsc --noEmit
make test            # go test ./... -race + vitest --coverage
make coverage-gate   # per-package thresholds (below)
```

**Coverage gates** (`scripts/coverage-gate.sh`):
- `pkg/masking`, `pkg/sqlguard`, `pkg/expression`, `pkg/templaterender` = **100%** statements.
- every other Go package ≥ **95%**; total Go ≥ 95%.
- frontend components/hooks ≥ **90%** (statements/lines), branches/functions ≥ 80%.
- Excluded from the *unit* gate (integration/runtime-tested instead): `cmd/`,
  `store/postgres`, `audit/postgres`, `pgxquerier`, `runner`, `scheduler/temporal.go`,
  `preview/pool.go`.

## 5. Conventions

- **SQL**: parameterized only; user SQL goes through `pkg/sqlguard` (single read-only
  SELECT, enforced on the parse tree). Every query path needs an injection test.
- **Secrets**: only via `pkg/secrets` — never hardcode or read credentials from
  env/config directly. Connection passwords live in the secret store; only the
  secret *name* (`secretRef`) is stored on the connection row.
- **RBAC**: every API handler sits behind `RequirePermission`/`RequireAdmin`
  (deny-by-default). Add a per-role test. Protected envs fail closed (401) when no
  identity resolves.
- **Masking**: external-DB rows pass the masking interceptor with a proving test.
- **Errors**: wrap with `%w` (`errorlint` enforced); match with `errors.Is`/`As`.
- **Go tests**: table-driven + `testify` where useful; `Test<Func>_<Case>`; a
  `*_test.go` beside every `.go`.
- **Commits**: conventional commits prefixed with the story id, e.g.
  `feat(E6-S1): add per-connection dialing`.
- **Branches**: `feature/<name>` off `main`; PR back into `main`.

## 6. Common recipes

### Add a DB migration
Drop a numbered file in `apps/automate-api/internal/store/migrations/`
(`00NN_name.sql`); it is `go:embed`-ed and applied once, in order, tracked in
`schema_migrations`. It runs automatically on API start, or standalone via
`scripts/migrate.sh "<postgres-url>"`. Prefer `IF NOT EXISTS` / `ADD COLUMN IF NOT
EXISTS`; if a migration must be destructive (see 0004's DROP+CREATE), rely on the
apply-once bookkeeping — never re-run raw `.sql` by hand.

### Add a node executor
1. Executor package under `apps/automate-worker/internal/executors/<node>` — pure,
   seam-based (inject the DB/mailer/etc. behind an interface), unit-tested.
2. Add the node config shape + a `case "<type>":` in
   `interpreter/activity.go:ExecuteNode`, wiring the executor with worker deps.
3. Add the node to the catalog in `apps/automate-api/internal/nodes/registry.go`
   (drives `GET /nodes` and the FE palette/config panel) and the golden test.
4. FE: node icon/label + a config form via the JSON-schema-driven `SchemaForm`.

### Add an API endpoint
Register it in `internal/httpapi/router.go` behind the right
`RequirePermission(...)`; implement the handler; add the store method to the
`Store` interface + `postgres` impl + the test `fakeStore`; write per-role RBAC +
happy/error tests.

### External DB connections (E6-S1)
Admin configures host/port/database/username + a password. The API saves the
password to the secret store and persists only its `secretRef`; `POST
/connections/:id/test` dials it live. At run start the API resolves each db.query
node's `connectionId` into dial fields (injected into the node config); the worker
resolves the password from the secret store and dials a **per-connection pool**.
The admin form captures host/port/database/username + password (or a `secretRef`)
with a live **Test** button that surfaces the probe result. End to end and tested.

### Auth / SSO (E2-S1/S2/S3)
Identity resolves from a **Bearer JWT** or the **`mw_access_token` cookie** (SSO),
verified by the local auth service; else the gateway-injected `X-Role`; else the
default role (admin in dev, **401 in sit/uat/prod**). Local login sets the cookie
(HttpOnly, SameSite=Lax, Secure in prod); `POST /auth/logout` clears it. Real Entra
JWKS validation is the remaining backlog item for prod SSO.

## 7. CI & quality tooling

CI (`.github/workflows/ci.yml`) runs lint + unit + coverage gate + the local-auth
policy check (`scripts/policy-check-local-auth.sh` — `AUTH_LOCAL_ENABLED` must be
false for sit/uat/prod). `gitleaks` (secrets), SonarQube and Trivy are part of the
banking quality gate. `govulncheck ./...` must report 0 (dependencies are kept
current — see the git history).
