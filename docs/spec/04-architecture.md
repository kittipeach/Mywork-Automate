# 04 — System Architecture

## 1. High-Level Diagram

```
                            ┌──────────────────────── Azure AKS (Southeast Asia) ────────────────────────┐
                            │                                                                            │
 ┌──────────┐   HTTPS  ┌────┴────┐    ┌──────────────────┐        ┌──────────────────────────────┐       │
 │ Browser  │─────────▶│  Kong   │───▶│ automate-web     │        │ automate-api (Go/Gin)        │       │
 │ (Next.js │          │ Gateway │    │ (Next.js SSR)    │───────▶│  - Flow/Template CRUD        │       │
 │  UI)     │          └────┬────┘    └──────────────────┘        │  - Versioning/State machine  │       │
 └──────────┘               │                                     │  - RBAC + Masking service    │       │
      ▲                     │                                     │  - Node Registry             │       │
      │ OIDC (PKCE)         │                                     │  - Run History API           │       │
      ▼                     │                                     └──────┬───────────┬───────────┘       │
 ┌──────────────┐           │                                            │           │                   │
 │ Microsoft    │           │            ┌───────────────────────────────▼──┐   ┌────▼───────────────┐   │
 │ Entra ID     │           │            │ Temporal Server (self-host/cloud)│   │ PostgreSQL         │   │
 └──────────────┘           │            └───────────────┬──────────────────┘   │ (Flexible Server)  │   │
                            │                            │                      │  app schema +      │   │
                            │            ┌───────────────▼──────────────────┐   │  temporal schema   │   │
                            │            │ automate-worker (Go, N pods, HPA)│   └────────────────────┘   │
                            │            │  - Node executors (DB/File/MFT/  │                            │
                            │            │    Email/Logic/AI)               │──────────┐                 │
                            │            │  - Scheduler (Temporal Schedules)│          │                 │
                            │            └───────────┬──────────┬───────────┘          │                 │
                            └────────────────────────┼──────────┼──────────────────────┼─────────────────┘
                                                     │          │                      │
                              ┌──────────────┐  ┌────▼─────┐ ┌──▼───────────┐  ┌──────▼──────────┐
                              │ Azure Key    │  │ Azure    │ │ Target       │  │ MFT / SFTP,     │
                              │ Vault        │  │ Blob     │ │ PostgreSQL   │  │ SMTP/Graph,     │
                              │ (Workload    │  │ Storage  │ │ DBs (HR/Pay) │  │ Azure OpenAI /  │
                              │  Identity)   │  │ (files)  │ │  read-only   │  │ AI Foundry      │
                              └──────────────┘  └──────────┘ └──────────────┘  └─────────────────┘
```

## 2. Components

### 2.1 `automate-web` — Next.js Frontend
- อยู่ใน MyWork monorepo, ใช้ `@mywork/ui`; route `/automate/*` ใน Admin Portal
- **Canvas:** `@xyflow/react` (React Flow) — custom node components ต่อ node type, edge validation, mini-map
- **Config Panel:** ฟอร์ม render จาก **JSON Schema ของ node** (node ใหม่ = เพิ่ม schema ไม่แก้ core) ด้วย react-hook-form + zod
- **Template Designer:** โมดูลแยก — Excel designer (grid preview), PDF designer (page preview), TXT designer (fixed-width ruler)
- State: TanStack Query (server state) + Zustand (canvas state); Autosave debounce 30s
- Auth: NextAuth/OIDC กับ Entra ID; Local login form แสดงเฉพาะเมื่อ API แจ้ง `localAuthEnabled`

### 2.2 `automate-api` — Go (Gin) Control Plane
- REST API (สัญญาที่ `06-data-model-api.md`), stateless, scale แนวนอน
- Services ภายใน: FlowService (CRUD + state machine + versioning), TemplateService, ConnectionService (Key Vault ref), NodeRegistry (catalog + JSON Schema), RunService (query run history), RBACService + MaskingService (interceptor ก่อน serialize ทุก response), AuditService
- Publish flow → compile & validate definition → เก็บ version → สร้าง/อัปเดต **Temporal Schedule** (สำหรับ recurring trigger)

### 2.3 Execution Engine — Temporal + `automate-worker`
**การตัดสินใจ:** ใช้ **Temporal.io** (มีอยู่แล้วใน MyWork stack สำหรับ saga) แทนการเขียน engine เอง

| ทางเลือก | ข้อดี | ข้อเสีย | ตัดสิน |
|---|---|---|---|
| A. Custom engine (Postgres queue + goroutines) | ควบคุมเต็มที่, ไม่มี dependency | ต้องเขียน retry/timeout/resume/cancel/scheduler เอง ~เดือน+ ของงาน แล้ว bug ตามมา | ❌ |
| B. **Temporal.io** | Durable execution (pod ตาย run ไม่หาย = NFR-AVAIL-002 ฟรี), retry policy/timer/cancel/signal built-in, **Temporal Schedules แทน cron scheduler**, Go SDK ชั้นหนึ่ง, ทีมมีประสบการณ์แล้ว | เพิ่ม infra 1 ชิ้น (มีอยู่แล้ว), learning curve worker pattern | ✅ |
| C. Kafka + consumers | ทีมคุ้น | ไม่ใช่ workflow engine — state machine ต้องเขียนเองอยู่ดี | ❌ |

**Mapping:**
- 1 Flow run = 1 **Temporal Workflow execution** (workflow id = `flow-{flowId}-run-{runId}`)
- 1 Node = 1 **Activity** (executor ต่อ node type, register ใน worker) — retry policy ต่อ node จาก config
- Recurring trigger = **Temporal Schedule**; Pause/Stop flow = pause/delete schedule + cancel running workflows (graceful)
- Workflow เป็น **generic interpreter**: อ่าน flow definition (JSONB, pinned version) → เดินกราฟ topologically → เรียก activity ต่อ node → ส่ง `items[]` ต่อ (payload ใหญ่เก็บ Blob แล้วส่ง reference กัน Temporal payload limit 2MB)
- If/Switch = เลือก branch ใน workflow code; For-Each = child workflow / batched activities; Delay = Temporal timer (durable)

### 2.4 Node Executor Plugin Model (Go)
```go
type NodeExecutor interface {
    Type() string                          // "db.query", "file.generate", "delivery.mft", ...
    Schema() []byte                        // JSON Schema สำหรับ config UI
    Validate(cfg json.RawMessage) error
    Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error)
}
// ExecutionInput: Items []map[string]any, Config, FlowContext(runId, vars), Secrets(resolver→Key Vault)
// ExecutionOutput: Items []map[string]any, Files []FileRef, Metrics
```
Node ใหม่ = implement interface + register — ตอบ FR-CANVAS-012 (มี node ได้มากกว่า requirement)

### 2.5 Storage Abstraction
```go
type FileStore interface { Put/Get/SignedURL/Delete }
```
- `LocalFileStore` (`/data/files`, PVC) — ใช้ตอน Dev/local ตาม requirement "ไฟล์เก็บในเครื่องก่อน"
- `AzureBlobStore` — SIT/UAT/Prod; เลือกด้วย env `FILE_STORE=local|azureblob`

### 2.6 Secrets — Azure Key Vault
- AKS **Workload Identity** (federated credential) — ไม่มี secret ใน cluster
- ตาราง `connections` เก็บเฉพาะ metadata + `keyvault_secret_name`; worker resolve ตอน execute พร้อม cache ≤ 5 นาที
- Dev/local: `SecretResolver` แบบ file-based (`secrets.local.yaml`, gitignored) สลับด้วย env

### 2.7 Identity
- **Prod path:** Entra ID OIDC → Kong/ API validate JWT (JWKS) → map claims → MyWork user + roles (sync กลุ่ม AD → role)
- **Dev path:** Local users (ตาราง `local_users`, argon2id hash) → API ออก JWT เอง (issuer แยก) — เปิดด้วย `AUTH_LOCAL_ENABLED=true` เท่านั้น; Helm values ของ SIT/UAT/Prod hardcode `false` + CI policy check

## 3. Execution Data Flow (ตัวอย่าง flow หลัก)
```
Temporal Schedule ครบรอบ (ทุกวัน 06:00)
  → start Workflow (pinned published version v3)
  → Activity: db.query      — resolve secret จาก KV → parameterized SELECT → 45,000 rows → เก็บ Blob → ส่ง ref
  → Activity: logic.if      — rowCount > 0 ? true-branch
  → Activity: file.generate — โหลด template v2 (XLSX) → stream write ด้วย excelize → Blob: payroll_20260714.xlsx
  → Activity (parallel):
       delivery.mft   — SFTP ไป bank MFT, .tmp→rename, verify checksum, retry 3× backoff
       delivery.email — Graph API + แนบไฟล์ / link
  → บันทึก execution + steps (I/O masked+truncated) → App Insights trace, metrics
```

## 4. Deployment (AKS)

| Deployment | Replicas | HPA | หมายเหตุ |
|---|---|---|---|
| automate-web | 2 | CPU 70% | Next.js SSR |
| automate-api | 2 | CPU 70% | stateless |
| automate-worker | 2→20 | CPU + Temporal task queue depth (KEDA) | resource limit mem 2Gi (stream file gen) |
| temporal (server) | ตาม chart | — | ใช้ instance รวมของ MyWork ได้ แยก namespace `automate` |

- Helm chart ใน monorepo (`deploy/charts/automate`), values per env; Ingress ผ่าน Kong (มี route/plugin: OIDC, rate-limit)
- Network policy: worker → target DB ผ่าน private endpoint/VNet เท่านั้น; egress whitelist (MFT hosts, Graph, Azure OpenAI)
- CI/CD: build → unit test → SonarQube gate → Trivy scan → push ACR → deploy Dev → e2e → promote

## 5. ADR สรุป

| # | Decision | เหตุผลหลัก |
|---|---|---|
| ADR-01 | ใช้ Temporal เป็น execution engine | durable execution + schedules ฟรี, มีใน stack แล้ว, ลดงาน engine เองหลายสิบ man-days |
| ADR-02 | React Flow (@xyflow/react) สำหรับ canvas | mature ที่สุด, MIT, n8n-style ecosystem |
| ADR-03 | Node config เป็น JSON Schema-driven form | เพิ่ม node ไม่ต้องแตะ frontend core |
| ADR-04 | Flow definition เป็น JSONB + immutable versions | diff/rollback ง่าย, ไม่ต้อง join หนัก |
| ADR-05 | `items[]` เป็น data contract ระหว่าง node (แบบ n8n) | ต่อ node อิสระ, For-each/Transform generic ได้ |
| ADR-06 | Payload ใหญ่เก็บ Blob ส่ง reference | เลี่ยง Temporal 2MB limit + DB บวม |
| ADR-07 | Excelize (XLSX stream) / Maroto+ฟอนต์ไทย (PDF) / encoder เอง (TXT fixed-width, TIS-620) | Go-native ทั้งหมด ไม่พึ่ง headless office |
| ADR-08 | AI ผ่าน Azure OpenAI/Foundry เท่านั้น | สอดคล้อง compliance ธนาคาร + เส้นทางที่องค์กรอนุมัติแล้ว |

## 6. สิ่งที่ต้อง revisit เมื่อโต
- Event trigger จาก MyWork event bus (Phase 2) — ออกแบบ topic contract ตั้งแต่ตอนทำ Webhook
- Multi-tenant แยก tenant ต่อบริษัทลูกค้า MyWork — tenant_id ฝังไว้แล้ว รอเปิด
- Approval/Human task node (แบบ Camunda) ถ้า use case HR ต้องการคนกดอนุมัติกลาง flow
- Marketplace ของ template/flow ภายในองค์กร
