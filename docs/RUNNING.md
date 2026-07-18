# Running MyWork Automate on another machine

End‑to‑end guide to bring the stack up on a fresh machine: dependencies →
infra → database migration → API/worker/web → smoke test. For the non‑negotiable
engineering rules see [`CLAUDE.md`](../CLAUDE.md); for the architecture see
[`docs/spec/04-architecture.md`](spec/04-architecture.md).

## 1. What runs

| Component | Path | Port (dev) | Notes |
|---|---|---|---|
| `automate-api` | `apps/automate-api` | `8080` | Gin REST control plane. **Auto‑migrates + seeds** the DB on startup. |
| `automate-worker` | `apps/automate-worker` | — | Temporal worker; runs the node executors (db.query → filegen → delivery). |
| `automate-web` | `apps/automate-web` | `3000` | Next.js 16 UI at `/automate`. |
| Postgres 16 | docker‑compose | `5432` | Control‑plane store **and** the demo external data source. |
| Temporal (dev) | docker‑compose | `7233` gRPC / `8233` UI | Durable workflow engine. |
| Azurite | docker‑compose | `10000‑2` | Azure Blob emulator (file store). |
| SFTP mock | docker‑compose | `2222` | MFT/SFTP delivery target. |
| Mailhog | docker‑compose | `1025` SMTP / `8025` UI | Email delivery sink. |

## 2. Prerequisites

- **Go ≥ 1.25** (the module pins `toolchain go1.26.5`; Go fetches it automatically
  if `GOTOOLCHAIN=auto`, the default — needs network the first time, or install
  go1.26.5 up front).
- **A C toolchain** — `pkg/sqlguard` uses cgo (`pganalyze/pg_query_go`). macOS:
  Xcode Command Line Tools (`xcode-select --install`); Debian/Ubuntu:
  `build-essential`.
- **Node ≥ 20** + **pnpm** (`corepack enable` or `npm i -g pnpm`).
- **Docker** (for the dev infra stack) + **make**.
- **psql** (PostgreSQL client) — only if you use the standalone `scripts/migrate.sh`.

Quick check:

```bash
go version        # go1.25+ ; toolchain go1.26.5 auto-selected on build
node --version    # v20+
pnpm --version
docker version
psql --version    # optional (standalone migrations)
```

## 3. Quick start (dev, one machine)

```bash
git clone https://github.com/kittipeach/Mywork-Automate.git
cd Mywork-Automate

# 3.1 infra: postgres, temporal-dev, azurite, sftp-mock, mailhog
make dev                        # docker compose up -d
docker compose -f docker-compose.dev.yml ps   # wait until postgres/temporal are healthy

# 3.2 install FE deps
( cd apps/automate-web && pnpm install )

# 3.3 environment (dev defaults). Postgres from make dev = localhost:5432
export APP_ENV=dev
export AUTH_LOCAL_ENABLED=true
export DATABASE_URL="postgres://automate:automate@localhost:5432/automate?sslmode=disable"
export TEMPORAL_HOSTPORT=localhost:7233

# 3.4 API — applies migrations + seed on startup, then serves :8080
go run ./apps/automate-api/cmd/api
#   → {"msg":"automate-api listening","addr":":8080","env":"dev"}

# 3.5 worker (new terminal, same env) — serves the Temporal task queue
export SMTP_ADDR=localhost:1025 FILE_DIR=/tmp/automate-files
go run ./apps/automate-worker/cmd/worker

# 3.6 web (new terminal) — point the UI at the real API
cd apps/automate-web
NEXT_PUBLIC_API_BASE="http://localhost:8080/api/automate/v1" pnpm dev
#   → open http://localhost:3000/automate
```

> **Port 5432 already in use?** If the host already runs a local Postgres, map the
> container to another port (e.g. edit `docker-compose.dev.yml` `ports: "5433:5432"`)
> and set `DATABASE_URL=...@localhost:5433/...` everywhere.

## 4. Database migration

The schema lives in `apps/automate-api/internal/store/migrations/*.sql` and is
tracked in a `schema_migrations` table (one row per applied file). There are two
equivalent ways to apply it — pick one; they are compatible and idempotent.

### 4a. Automatic (default)
The API runs `postgres.Migrate` **and** `postgres.Seed` on every startup:
- **Migrate** applies each `NNNN_*.sql` exactly once, in numbered order.
- **Seed** inserts the demo control‑plane dataset (5 flows / 3 executions / 4
  connections) **only when the `flows` table is empty**.

So on a fresh DB you don't have to do anything — just start the API.

### 4b. Standalone (pre‑provision / DBA / CI)
To migrate a database **without** starting the API (e.g. a managed Postgres a DBA
provisions ahead of deploy):

```bash
scripts/migrate.sh "postgres://user:pass@host:5432/dbname?sslmode=disable"
# or:  DATABASE_URL="postgres://..." scripts/migrate.sh
```

It creates `schema_migrations`, applies each pending `*.sql` in order inside a
transaction, and records it — identical bookkeeping to the Go migrator, so the
API will then skip them. Re‑running is a no‑op.

> ⚠️ Migration `0004_flow_versions.sql` **drops and recreates** `flow_versions`,
> so it is not idempotent on its own. Always go through `scripts/migrate.sh` or the
> API migrator (which apply it exactly once) — never `psql -f` the raw files.

### 4c. Demo external data (`employees`)
The seeded "runnable" flow queries an **external** `employees` table via the
`db.query` node. The app does **not** create it (it stands in for a customer HR
database). To make Test‑run / `runflow` return rows, seed it into the same DB the
worker uses:

```bash
psql "$DATABASE_URL" -f scripts/seed-employees.sql
```

## 5. Smoke test

```bash
BASE=localhost:8080/api/automate/v1

# health
curl -s localhost:8080/healthz          # {"status":"ok"}

# login (dev local admin) → JWT
TOKEN=$(curl -s -X POST $BASE/auth/local/login -H 'Content-Type: application/json' \
  -d '{"email":"admin@mywork.local","password":"ChangeMe-Admin1"}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["token"])')

# seeded flows come back
curl -s $BASE/flows -H "Authorization: Bearer $TOKEN"

# run the engine end-to-end (needs worker + employees table). From the repo root:
TEMPORAL_HOSTPORT=localhost:7233 go run ./apps/automate-worker/cmd/runflow viewer
#   → salary/citizen_id/phone/email come back MASKED for the viewer role
```

Temporal UI: <http://localhost:8233> · Mailhog UI: <http://localhost:8025>.

## 6. Environment variables

| Var | Default | Used by | Meaning |
|---|---|---|---|
| `APP_ENV` | `dev` | api, worker | `dev` / `sit` / `uat` / `prod`. `sit/uat/prod` are **protected**. |
| `DATABASE_URL` | — (**required**) | api, worker | Postgres connection string. |
| `AUTH_LOCAL_ENABLED` | `false` | api | Enables local login. **Refused at startup in protected envs.** |
| `AUTH_JWT_SECRET` | dev default | api | HMAC secret for local‑login JWTs (set a real one anywhere non‑dev). |
| `HTTP_ADDR` | `:8080` | api | API listen address. |
| `TEMPORAL_HOSTPORT` | `localhost:7233` | api, worker | Temporal frontend. |
| `FILE_STORE` | `local` | api, worker | `local` or `azure_blob`. |
| `FILE_DIR` | (worker default) | worker | Local file store dir when `FILE_STORE=local`. |
| `SMTP_ADDR` / `SMTP_FROM` | `localhost:1025` | worker | Email delivery (Mailhog in dev). |
| `SFTP_ADDR` / `SFTP_USER` / `SFTP_PASSWORD` | empty | worker | MFT/SFTP delivery target (empty ⇒ delivery.mft errors clearly). |
| `SECRETS_FILE` | `secrets.local.yaml` | api, worker | Dev secret store (gitignored YAML). Holds external-DB connection passwords; Key Vault replaces it in prod. |
| `ENTRA_TENANT_ID` | — | api | Entra (Azure AD) tenant — derives the v2.0 issuer + JWKS URL. Enables prod SSO. |
| `ENTRA_AUDIENCE` | — | api | The API's application (client) id / `api://…` URI the token must be issued for. |
| `ENTRA_ISSUER` / `ENTRA_JWKS_URL` | derived | api | Override the issuer / JWKS URL (otherwise derived from the tenant id). |
| `NEXT_PUBLIC_API_BASE` | `/api/automate/v1` | web (build/dev) | Point the UI at the real API, e.g. `http://localhost:8080/api/automate/v1`. |

## 6b. Configure an external database connection (admin)

A flow's `db.query` node reads from an **external** database defined as a
*connection*. Create one via the admin API (or the Connections UI). The password
is saved to the secret store (`secrets.local.yaml` in dev, Key Vault in prod) — the
connection row only stores its `secretRef`, never the password.

```bash
BASE=localhost:8080/api/automate/v1
TOKEN=...   # admin JWT from §5

# create a Postgres connection (dev: pass a raw password → saved to the secret store)
CID=$(curl -s -X POST $BASE/connections -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' -d '{
  "name":"HR Postgres","type":"postgres","host":"hr-db.internal","port":5432,
  "database":"hr","username":"reader","password":"CHANGE_ME","sslMode":"disable",
  "allowedRoles":["admin","designer"]
}' | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')

# live reachability probe (dials the DB, runs SELECT 1)
curl -s -X POST $BASE/connections/$CID/test -H "Authorization: Bearer $TOKEN"
#   → {"status":"ok","message":"connected"}   (or status:"error" with a reason)
```

In production set `secretRef` to an existing Key Vault secret name and omit
`password`. A flow's `db.query` node references the connection via `connectionId`;
at run start the API injects the dial fields and the worker dials a dedicated pool
for that connection (SELECT-only guard + masking still apply).

## 6c. SSO / session auth

Identity is resolved from a **Bearer JWT** (`Authorization: Bearer …`) or, when
absent, the **`mw_access_token` cookie** — so an SSO-authenticated browser session
works without a JS-managed header. `POST /auth/local/login` sets that cookie
(HttpOnly, SameSite=Lax, `Secure` in sit/uat/prod) in addition to returning the
token; `POST /auth/logout` clears it. In protected environments a request that
resolves no identity **fails closed with 401**.

For **production SSO**, set `ENTRA_TENANT_ID` + `ENTRA_AUDIENCE`: the API then
validates real Microsoft Entra ID access tokens (RS256, verified against the
tenant JWKS, issuer/audience/expiry enforced) presented as a Bearer token or the
`mw_access_token` cookie, mapping the token's `roles` app-role claim to the
caller's authz role. This is independent of local auth and works even in protected
envs where local login is disabled.

## 7. Deploying to a protected environment (sit/uat/prod)

- **`AUTH_LOCAL_ENABLED=false`** — the config guard refuses to start otherwise
  (there is also a CI policy check, `scripts/policy-check-local-auth.sh`).
- With local auth off, the API resolves identity from a gateway‑injected
  `X-Role` header; an unauthenticated request **fails closed with 401** in
  protected envs. Front the API with a gateway (Kong/Entra) that validates the
  user and injects `X-Role`, and strips any client‑supplied one. *(Real Entra JWT
  validation, E2‑S1, is still on the backlog — see the PR follow‑ups.)*
- Provide a real managed **Postgres** (`DATABASE_URL`) and run
  `scripts/migrate.sh` against it as a deploy step (or let the API migrate on
  first boot). The demo `Seed` still runs when `flows` is empty — provision an
  empty‑but‑expected DB accordingly if you don't want the sample rows.
- Set a strong **`AUTH_JWT_SECRET`**; source real secrets via `pkg/secrets`
  (Key Vault), never env in prod.
- Helm chart for AKS/kind: `deploy/charts/automate`.

## 8. Troubleshooting

| Symptom | Fix |
|---|---|
| `bind: address already in use` on 5432 | Another Postgres owns the port — remap the compose port to 5433 and update `DATABASE_URL` (see §3). |
| cgo / `pg_query` build errors | Install a C toolchain (Xcode CLT / `build-essential`); `pkg/sqlguard` needs it. |
| API exits `DATABASE_URL is required` | Export `DATABASE_URL` before `go run ./apps/automate-api/cmd/api`. |
| API exits `AUTH_LOCAL_ENABLED must be false in sit/uat/prod` | You set `APP_ENV=sit\|uat\|prod` with local auth on — turn it off. |
| `dial temporal ... connection refused` | Temporal not up/healthy — `docker compose -f docker-compose.dev.yml ps`, wait for health. |
| `runflow` returns `rowCount=0` | Seed the external table: `psql "$DATABASE_URL" -f scripts/seed-employees.sql`. |
| Go downloads a toolchain on build | Expected — the module pins `toolchain go1.26.5`; needs network the first time or a preinstalled 1.26.5. |
| UI shows no data | `NEXT_PUBLIC_API_BASE` not pointing at the API, or the API isn't up on :8080. |
