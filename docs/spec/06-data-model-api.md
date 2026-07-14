# 06 — Data Model & API Contracts

## 1. Database Schema (PostgreSQL — schema `automate`)

```sql
-- ===== Core: Flows & Versions =====
CREATE TABLE flows (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id     UUID NOT NULL,                    -- future multi-tenant (default tenant เดียว)
  folder_id     UUID REFERENCES folders(id),
  name          VARCHAR(200) NOT NULL,
  description   TEXT,
  status        VARCHAR(20) NOT NULL DEFAULT 'draft',   -- draft|published|paused|stopped
  current_draft JSONB NOT NULL,                   -- definition ที่กำลังแก้
  published_version_id UUID REFERENCES flow_versions(id),
  schema_version INT NOT NULL DEFAULT 1,          -- version ของ definition format
  created_by/updated_by UUID, created_at/updated_at TIMESTAMPTZ,
  deleted_at    TIMESTAMPTZ                       -- soft delete
);

CREATE TABLE flow_versions (                       -- immutable snapshots
  id UUID PK, flow_id UUID NOT NULL REFERENCES flows(id),
  version_no INT NOT NULL,                         -- v1, v2, ...
  definition JSONB NOT NULL,                       -- graph เต็ม (nodes, edges, config)
  change_note TEXT NOT NULL,
  published_by UUID, published_at TIMESTAMPTZ,
  UNIQUE (flow_id, version_no)
);

-- flow definition JSONB shape:
-- { "nodes":[{"id","type","name","position":{x,y},"config":{...},"onError":"fail|continue|errorBranch","retry":{...}}],
--   "edges":[{"id","source","sourcePort","target","targetPort","label"}],
--   "variables":[...], "settings":{"notification":{...},"retention":{...}} }

CREATE TABLE folders (id UUID PK, tenant_id UUID, name VARCHAR(120), parent_id UUID NULL);

-- ===== Connections & Secrets (ref เท่านั้น — ค่าจริงอยู่ Key Vault) =====
CREATE TABLE connections (
  id UUID PK, tenant_id UUID,
  type VARCHAR(30) NOT NULL,                       -- postgres|sftp|smtp|graph|azure_openai
  name VARCHAR(120) NOT NULL,
  config JSONB NOT NULL,                           -- host, port, database, username, path... (non-secret)
  keyvault_secret_name VARCHAR(200) NOT NULL,      -- ชี้ secret ใน AKV
  allowed_role_ids UUID[] NOT NULL DEFAULT '{}',   -- RBAC ระดับ connection
  created_by UUID, created_at, updated_at, deleted_at
);

-- ===== Templates =====
CREATE TABLE templates (
  id UUID PK, tenant_id UUID, name VARCHAR(200), format VARCHAR(10),  -- xlsx|pdf|txt|csv
  category VARCHAR(80), current_version_id UUID, created_by, timestamps, deleted_at
);
CREATE TABLE template_versions (
  id UUID PK, template_id UUID REFERENCES templates(id),
  version_no INT, definition JSONB NOT NULL,       -- template model (ดู 05-node-catalog)
  change_note TEXT, created_by UUID, created_at,
  UNIQUE (template_id, version_no)
);

-- ===== Executions =====
CREATE TABLE executions (
  id UUID PK, flow_id UUID, flow_version_id UUID,
  temporal_workflow_id VARCHAR(200) NOT NULL,
  trigger_type VARCHAR(20),                        -- schedule|manual|webhook|event
  triggered_by UUID NULL,                          -- user (กรณี manual)
  status VARCHAR(20) NOT NULL,                     -- queued|running|success|failed|cancelled|skipped
  started_at, finished_at TIMESTAMPTZ, duration_ms BIGINT,
  error_summary TEXT,
  input_params JSONB
) PARTITION BY RANGE (started_at);                 -- partition รายเดือน + retention job

CREATE TABLE execution_steps (
  id UUID PK, execution_id UUID REFERENCES executions(id),
  node_id VARCHAR(60), node_type VARCHAR(60), node_name VARCHAR(200),
  status VARCHAR(20), attempt INT DEFAULT 1,
  started_at, finished_at, duration_ms,
  input_sample JSONB,                              -- truncated ≤1000 items + masked snapshot
  output_sample JSONB, input_count INT, output_count INT,
  error_message TEXT, error_detail JSONB
) PARTITION BY RANGE (started_at);

CREATE TABLE files (
  id UUID PK, tenant_id UUID, execution_id UUID, node_id VARCHAR(60),
  filename VARCHAR(300), format VARCHAR(10), size_bytes BIGINT,
  checksum_sha256 CHAR(64), storage VARCHAR(20),   -- local|azureblob
  storage_path TEXT, sensitive BOOL DEFAULT false,
  retention_until DATE, created_at
);
CREATE TABLE file_downloads (id UUID PK, file_id UUID, user_id UUID, downloaded_at, ip INET);

-- ===== Identity & RBAC =====
CREATE TABLE users (
  id UUID PK, tenant_id UUID,
  auth_provider VARCHAR(20) NOT NULL,              -- entra|local
  external_id VARCHAR(200),                        -- Entra object id
  email VARCHAR(200) UNIQUE, display_name VARCHAR(200),
  is_active BOOL, created_at, last_login_at
);
CREATE TABLE local_users (                          -- Dev only
  user_id UUID PK REFERENCES users(id),
  password_hash TEXT NOT NULL,                      -- argon2id
  failed_attempts INT DEFAULT 0, locked_until TIMESTAMPTZ
);
CREATE TABLE roles (id UUID PK, code VARCHAR(50) UNIQUE, name VARCHAR(120), is_system BOOL);
CREATE TABLE permissions (id UUID PK, code VARCHAR(80) UNIQUE);       -- flow.create, flow.publish, ...
CREATE TABLE role_permissions (role_id UUID, permission_id UUID, PRIMARY KEY(role_id, permission_id));
CREATE TABLE user_roles (user_id UUID, role_id UUID, PRIMARY KEY(user_id, role_id));
CREATE TABLE flow_grants (                          -- สิทธิ์ระดับ flow/folder
  id UUID PK, subject_type VARCHAR(10),             -- user|role
  subject_id UUID, resource_type VARCHAR(10),       -- flow|folder
  resource_id UUID, access VARCHAR(10)              -- viewer|editor|owner
);

-- ===== Data Masking =====
CREATE TABLE masking_rules (
  id UUID PK, tenant_id UUID,
  name VARCHAR(120),
  match_type VARCHAR(20),                           -- column_name|regex|connection_column
  pattern VARCHAR(300),                             -- เช่น '(?i)(salary|citizen_id|bank_account)'
  mask_style VARCHAR(20),                           -- full|partial_last4|hash
  exempt_role_ids UUID[] NOT NULL DEFAULT '{}',     -- roles ที่เห็นค่าจริง
  applies_to VARCHAR(20)[] DEFAULT '{preview,log,ai}', -- จุดที่บังคับใช้
  is_active BOOL
);

-- ===== Audit =====
CREATE TABLE audit_logs (
  id BIGSERIAL PK, tenant_id UUID, user_id UUID,
  action VARCHAR(80),                               -- flow.publish, file.download, auth.login, masking.bypass_view ...
  resource_type VARCHAR(40), resource_id UUID,
  detail JSONB, ip INET, user_agent TEXT, created_at TIMESTAMPTZ
) PARTITION BY RANGE (created_at);                  -- append-only; DB role ของ app ไม่มี UPDATE/DELETE
```

Indexes สำคัญ: `executions(flow_id, started_at DESC)`, `execution_steps(execution_id)`, `files(retention_until)`, `flows(tenant_id, status)`, GIN บน `flows.current_draft` (ค้นหา node type)

## 2. REST API Contracts (`/api/automate/v1`)

Auth: `Authorization: Bearer <JWT>` (Entra หรือ local-issued) | ทุก response ผ่าน masking interceptor | Error format: `{ "error": { "code", "message", "detail" } }`

### Flows
```
GET    /flows?folder=&status=&q=&page=          list (RBAC-filtered)
POST   /flows                                    create draft
GET    /flows/{id}                               get (draft + published info)
PUT    /flows/{id}/draft                         save draft (autosave ใช้ตัวนี้, optimistic lock ด้วย updated_at)
POST   /flows/{id}/validate                      pre-publish validation report
POST   /flows/{id}/publish        body:{changeNote}   → สร้าง version + สร้าง/อัปเดต Temporal Schedule
POST   /flows/{id}/pause | /resume | /stop
POST   /flows/{id}/run            body:{params}       manual run → 202 {executionId}
GET    /flows/{id}/versions
GET    /flows/{id}/versions/{v}/diff?against={v2}
POST   /flows/{id}/rollback       body:{toVersion, changeNote}
GET    /flows/{id}/export         → JSON  |  POST /flows/import
DELETE /flows/{id}                soft delete
```

### Executions
```
GET    /flows/{id}/executions?status=&from=&to=&page=
GET    /executions/{id}                          รวม steps
POST   /executions/{id}/cancel
POST   /executions/{id}/retry     body:{fromNodeId?}   re-run / resume
GET    /executions/{id}/files
GET    /files/{id}/download-url                  → signed URL (audit)
```

### Nodes / Templates / Connections
```
GET    /nodes                                    node registry (type, category, JSON Schema, docs)
POST   /nodes/{type}/test                        test node เดี่ยวด้วย sample input (sandbox run)
GET|POST|PUT /templates ... /templates/{id}/versions /preview (sample data → render) /export /import
GET|POST|PUT|DELETE /connections ... POST /connections/{id}/test
GET    /connections/{id}/schema?db=              table/column metadata (สำหรับ query builder)
```

### AI
```
POST   /ai/sql-assist        body:{connectionId, prompt}         → {sql, explanation}   (ส่งเฉพาะ schema)
POST   /ai/flow-explain      body:{flowId}                        → {summary}
```

### Admin & Auth
```
GET|POST users, roles, permissions, masking-rules, audit-logs?action=&from=
POST   /auth/local/login     (เฉพาะ AUTH_LOCAL_ENABLED)  body:{email,password} → {token}
GET    /auth/config          → { providers: ["entra"|"entra","local"] }   (frontend ใช้ตัดสินใจแสดงฟอร์ม)
GET    /me                   → profile + effective permissions
```

### Webhook (public ผ่าน Kong route แยก + rate limit)
```
POST   /hooks/{token}        HMAC header validate → 202 {executionId}
```

## 3. Expression Language (สรุป contract)
- Syntax: `{{ ... }}` — อ้าง `$node["Query Payroll"].items`, `$node["X"].meta.rowCount`, `$flow.runDate`, `$flow.params.period`, `$vars.foo`
- Pipe functions: `format`, `upper/lower`, `addDays`, `tz`, `number`, `padLeft/padRight`, `sum/count/avg`
- Sandbox: ไม่มี IO/network/loop — evaluate ด้วย expression engine (เช่น expr-lang/expr) จำกัด timeout 100ms
