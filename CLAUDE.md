# MyWork Automate — System Handbook & Project Memory

Banking-grade, node-based **workflow automation engine**. Design a flow on a
canvas (schedule → query a DB → generate a file → deliver via MFT/email/download),
publish it with immutable versioning, and let a durable Temporal-backed engine run
it. This file is the single source of truth for an agent or engineer picking the
project up on a new machine. Long-form guides: [`docs/RUNNING.md`](docs/RUNNING.md)
(run it) · [`docs/DEVELOPING.md`](docs/DEVELOPING.md) (develop it) · [`docs/spec/`](docs/spec/)
(full spec — read `09-phase-plan.md` first). Repo: `github.com/kittipeach/Mywork-Automate`.

---

## 1. Stack & versions

| Layer | Tech | Version |
|---|---|---|
| API (control plane) | Go + Gin | Go 1.25 / toolchain **go1.26.5** |
| Worker (engine) | Go + Temporal SDK | — |
| Web UI | Next.js App Router + React | **Next 16.2.10 / React 19.2** |
| DB | PostgreSQL | 16 |
| Durable engine | Temporal | dev server |
| File store | local FS / Azure Blob (azurite in dev) | — |
| Email / MFT (dev) | Mailhog / atmoz sftp | — |

A **C toolchain is required** (cgo `pg_query` in `pkg/sqlguard`). `govulncheck ./...`
is kept at **0**. `make lint` (golangci-lint + eslint flat config + tsc) is green.

## 2. Layout

```
apps/automate-api      Go/Gin REST control plane (auto-migrates + seeds on start)
  cmd/api              entrypoint / composition root (pool, auth, secrets, dialer, temporal)
  internal/httpapi     router, handlers, RBAC middleware, auth (cookie+bearer), connections
  internal/store       Store interface + models; store/postgres (pgx + Migrate/Seed);
                       store/migrations (embedded 0001..0008 *.sql, schema_migrations-tracked)
  internal/{audit,scheduler,runner,preview,nodes,notify,lifecycle,flowvalidate}
apps/automate-worker   Go Temporal worker (node executors)
  cmd/worker           entrypoint (executors, secrets, per-connection pgx pool cache)
  cmd/runflow          Docker-free end-to-end demo (trigger→db.query→if→deliver)
  internal/interpreter FlowWorkflow + ExecuteNode activity + node config shapes
  internal/executors/dbquery   the db.query executor (sqlguard+secrets+masking+dial)
  internal/pgxquerier  pgx Querier (statement timeout, pool)
apps/automate-web      Next.js 16 UI at /automate (React 19, @xyflow, TanStack Query, Zustand)
pkg/                   sqlguard masking expression templaterender secrets filegen
                       filestore mailer authz logscrub obs   (the shared, tested core)
internal/config        validated env + local-auth protected-env guard
internal/flowspec      wire types shared api↔worker (FlowDef/NodeDef/FlowInput)
deploy/charts/automate Helm chart      scripts/ coverage-gate, policy checks, migrate.sh, seed-employees.sql
tests/e2e (Playwright) tests/perf (k6)
```

## 3. Commands (ห้ามเดา — ใช้ตามนี้)

```bash
make dev            # docker-compose: postgres, temporal-dev, azurite, sftp-mock, mailhog
make test           # go test ./... -race + vitest --coverage
make test-int       # integration (testcontainers)
make test-e2e       # Playwright
make coverage-gate  # per-package thresholds (fails build if under)
make lint           # golangci-lint + eslint + tsc --noEmit   (needs golangci-lint on PATH)
make down           # stop dev stack
scripts/migrate.sh "<postgres-url>"     # standalone DB migration (schema_migrations-compatible)
psql "$DATABASE_URL" -f scripts/seed-employees.sql   # demo external data source
```

Run locally (dev): see [`docs/RUNNING.md`](docs/RUNNING.md). Dev admin:
`admin@mywork.local` / `ChangeMe-Admin1`. Note: on a host with a native Postgres on
5432, map the compose Postgres to 5433 and adjust `DATABASE_URL`.

## 4. Environment variables

| Var | Default | Used by | Meaning |
|---|---|---|---|
| `APP_ENV` | `dev` | api, worker | dev / sit / uat / prod (last three **protected**). |
| `DATABASE_URL` | — (**required**) | api, worker | Postgres DSN. |
| `AUTH_LOCAL_ENABLED` | `false` | api | Local login. **Refused at startup in protected envs.** |
| `AUTH_JWT_SECRET` | dev default | api | HMAC secret for local-login JWTs. |
| `HTTP_ADDR` | `:8080` | api | Listen address. |
| `TEMPORAL_HOSTPORT` | `localhost:7233` | api, worker | Temporal frontend. |
| `FILE_STORE` / `FILE_DIR` | `local` | api, worker | `local`|`azure_blob`; local dir. |
| `SMTP_ADDR` / `SMTP_FROM` | `localhost:1025` | api, worker | Email (Mailhog in dev). |
| `SFTP_ADDR`/`SFTP_USER`/`SFTP_PASSWORD` | empty | worker | MFT/SFTP target. |
| `SECRETS_FILE` | `secrets.local.yaml` | api, worker | Dev secret store (Key Vault in prod). |
| `NEXT_PUBLIC_API_BASE` | `/api/automate/v1` | web | Point the UI at the Go API. |

## 5. HTTP API (`/api/automate/v1`)

Public: `GET /healthz` `GET /readyz` `GET /auth/config` `POST /auth/local/login`
`POST /auth/logout`. Everything below resolves a role (JWT/cookie → X-Role →
default) and is gated per route:

- **Identity**: `GET /me`.
- **Flows**: `GET /flows` `GET /flows/:id` (`flow.view`); `POST /flows`
  `PUT /flows/:id/draft` (`flow.create`); `POST /flows/:id/{publish,pause,resume,
  stop,rollback}` (`flow.publish`); `DELETE /flows/:id` (`flow.create`);
  `POST /flows/:id/restore` (admin); `GET/POST/DELETE /flows/:id/grants`
  (`flow.publish`); `POST /flows/:id/validate` (`flow.view`); `POST /flows/:id/run`
  (`flow.run`); `GET /flows/:id/versions` (`flow.view`).
- **Runs**: `GET /executions[/:id]` (`run.view`); `GET /executions/:id/stream`
  (SSE live status); `POST /executions/:id/{cancel,retry}` (`flow.run`).
- **Connections**: `GET /connections` (`flow.view`); `POST/PUT/DELETE /connections[/:id]`
  + `POST /connections/:id/test` (`connection.manage`); `POST /connections/:id/query-preview`
  + `GET /connections/:id/schema` (`flow.view`).
- **Nodes / audit**: `GET /nodes` (`flow.view`); `GET /audit-logs` (admin-only).

## 6. Node catalog

`trigger.schedule` · `trigger.manual` · `db.query` (SELECT-only, per-connection,
masked) · `logic.if` · `logic.transform` (select/rename/filter/sort) ·
`file.generate` (xlsx/csv/txt) · `delivery.mft` (SFTP, atomic+retry) ·
`delivery.email` (SMTP/Graph) · `delivery.download` (My Files). Catalog lives in
`apps/automate-api/internal/nodes/registry.go` (drives `GET /nodes` + FE palette).

## 7. Data model & migrations

Control-plane tables (Postgres): `folders`, `flows`, `flow_versions`,
`connections`, `executions`, `execution_steps`, `audit_logs`, `flow_grants`, plus
`schema_migrations`. Migrations are embedded `apps/automate-api/internal/store/
migrations/00NN_*.sql`, applied **once, in order**, tracked by filename — run
automatically on API start (`postgres.Migrate` + `postgres.Seed`, seed only when
`flows` is empty) **or** standalone via `scripts/migrate.sh`. ⚠️ `0004` recreates
`flow_versions` (DROP+CREATE) — the apply-once bookkeeping is what makes the set
re-runnable; never `psql -f` raw files by hand. `0008` added connection dial
fields (port/database/username/ssl_mode/secret_ref). The `employees` table used by
the db.query demo is **external** — seed it with `scripts/seed-employees.sql`.

## 8. Security model (the crown jewels)

- **SQL guard** (`pkg/sqlguard`): user SQL must be exactly one read-only `SELECT`,
  enforced on the **PostgreSQL parse tree** (pg_query) — default-deny any non-SELECT
  statement anywhere (CTEs, subqueries, stacked, locking, SELECT INTO). 100% tested.
- **Masking** (`pkg/masking`): default rules mask salary/citizen_id/bank_account/
  phone/email (EN + TH column names) at preview/log/AI points; applied to every
  external-DB result. Fails safe — a matched field is never returned in cleartext
  (handles native numerics **and** pgx `pgtype.Numeric` / `driver.Valuer`). 100% tested.
- **Secrets** (`pkg/secrets`): resolve/write named secrets (Key Vault in prod, a
  gitignored `secrets.local.yaml` in dev). **Connection passwords are stored only in
  the secret store** — the connection row keeps just the `secretRef`. Never hardcode
  or read credentials from env directly.
- **RBAC** (`pkg/authz` + `internal/httpapi/rbac.go`): four roles
  admin > designer > operator > viewer, deny-by-default matrix. Every handler behind
  `RequirePermission`/`RequireAdmin`.
- **Auth / SSO**: identity from a **Bearer JWT** or the **`mw_access_token` cookie**
  (verified by the local auth service), else gateway-injected `X-Role`, else the
  default. In **protected envs a request that resolves no identity fails closed
  (401)** — never the admin default (that fail-open bug is fixed). Login sets the
  HttpOnly cookie; `POST /auth/logout` clears it. Local auth is refused at startup
  in sit/uat/prod (config guard + `scripts/policy-check-local-auth.sh`).
- **Connections → external DB**: admin sets host/port/db/user + password (saved to
  the secret store); `POST /connections/:id/test` dials live. At run start the API
  injects each db.query node's connection dial fields; the worker resolves the
  password and dials a **per-connection pgx pool**. SELECT-only + masking still apply.

## 9. Non-negotiable rules

1. **TDD เท่านั้น**: test ก่อน → เห็น fail → implement → เห็น pass → refactor.
2. **Coverage gates** (`scripts/coverage-gate.sh`): masking/sqlguard/expression/
   templaterender = **100%**; other Go ≥ **95%**; FE components/hooks ≥ **90%**.
3. SQL: parameterized only; SQL mode ผ่าน `pkg/sqlguard` (SELECT-only) — ทุก query
   path ต้องมี injection test.
4. Secrets: ผ่าน `pkg/secrets` เท่านั้น — ห้าม hardcode/env ตรง; gitleaks ทุก commit.
5. ทุก API handler ต้องผ่าน RBAC middleware + deny-by-default test ต่อ role.
6. Response ที่มี data จาก DB ภายนอกต้องผ่าน masking interceptor — มี test พิสูจน์.
7. โครง test: Go = table-driven + testify; ตั้งชื่อ `Test<Func>_<Case>`; `*_test.go`
   คู่ทุกไฟล์ `.go`.
8. ห้ามแตะไฟล์นอก scope ของ task ตัวเอง (กัน conflict) — ถ้าจำเป็นให้หยุดแล้วรายงาน.
9. Commit: conventional commits, prefix ด้วย story id เช่น `feat(E6-S3): ...`.
   Branch `feature/<name>` จาก `main`; merge กลับ `main`.
10. จบ task: `make lint && make test && make coverage-gate` เขียวทั้งหมดก่อนสรุป.

### Definition of Done (ทุก task)
- [ ] Tests เขียนก่อนและครอบ: happy path, edge cases, error paths, security cases.
- [ ] Coverage ผ่าน gate | Lint ผ่าน | ไม่มี TODO ที่ไม่มี ticket.
- [ ] Integration test ถ้าแตะ DB/Temporal/SFTP/Blob (testcontainers).
- [ ] อัปเดต docs ถ้า API/schema เปลี่ยน.

## 10. Status (2026-07-18)

Phase 1 MVP is functional end-to-end (login/RBAC/masking/versioning/engine/
history/audit). Recently landed on `feature/security-hardening` (merged) and
`feature/db-connections`: masking numeric-leak fix, auth fail-open → fail-closed,
all 11 govulncheck CVEs cleared, Next 16 + React 19, ESLint 9 flat config +
golangci clean, **external DB connections (backend + live test-connect + per-
connection run-time dialing)**, and **SSO auth via HttpOnly cookie**.
**In progress / backlog**: connections admin **form fields + Test button (FE)** and
a full second-DB E2E; real Entra **JWKS** validation for prod SSO; prod still
trusts a gateway-injected `X-Role`. See `docs/spec/09-phase-plan.md` for P2–P4.
