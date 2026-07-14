# 08 — Backlog: Epics → Stories → Tasks

> พร้อม import เข้า Jira | Estimate เป็น **man-days (MD)** ระดับ story (รวม dev+test ของ story นั้น) — ปรับใน grooming
> Story format: `As a <role>, I want <capability>, so that <benefit>` + Acceptance Criteria (AC) | Task ระบุ discipline: [BE] [FE] [DevOps] [QA] [SA]
> **รวมทั้งหมด ~466 MD** — แบ่งเฟสตาม `09-phase-plan.md`: **Phase 1 (MVP) ~258 MD** ที่เหลือ Phase 2–4

## Phase Assignment (ใช้เป็นค่า field "Phase" ตอน import Jira)

| Phase | Stories |
|---|---|
| **P1 (MVP)** | E1-S1..S7 ทั้งหมด · E2-S1, S2, S3, S5 · E2-S4 *(เฉพาะ engine + default rules — Admin UI ไป P2)* · E3-S1, S2, S3, S4, S6 · E4-S1, S2 *(+rollback แบบง่าย)* · E5-S1..S6 *(S5 ไม่รวม resume-from-failed)* · E6-S1, S2, S3, S4 · E7-S1 *(+Excel template พื้นฐาน)* · E8-S1, S2, S3 *(sequential)* · E9-S1, S3 *(พื้นฐาน)*, S4 · E11-S1, S3 · E12-S1 *(smoke)*, S2 · E13-S1, S2, S3 |
| **P2** | E2-S4 *(Admin UI + custom rules)* · E3-S5 · E4-S3 · E5 resume-from-failed · E6-S5 · E7-S2..S6 · E8-S4 · E9-S2, S3 *(aggregate/join)* · E10-S1..S3 · E11-S2 · E12-S1 *(soak)*, S3, S4, S5 |
| **P3** | E4-S4 · trigger.webhook, logic.switch/delay/merge, delivery.http, notify.teams *(เพิ่ม story ใหม่ตอน grooming P3)* · dashboard เต็ม, quota, custom roles |
| **P4** | trigger.event, delivery.blob, ai.anomaly, util.archive, NL→Flow, DOCX/JSON/XML, multi-tenant, approval node |

---

## EPIC E1 — Platform Foundation & Infrastructure (~38 MD)
> โครงระบบ, AKS, CI/CD, Key Vault, Blob, Temporal — ทุก epic อื่น depend ตัวนี้

**E1-S1. Project scaffold & monorepo integration (5 MD)**
As a developer, I want automate-web/api/worker scaffolded in the MyWork monorepo, so that ทีมเริ่มงาน feature ได้บนโครงเดียวกัน
- AC: `automate-api` (Gin) + `automate-worker` + `automate-web` (Next.js route `/automate`) build & รัน local ผ่าน docker-compose (Postgres, Temporal dev, Azurite, SFTP mock) ครบ; README onboarding ≤ 30 นาที setup
- Tasks: [BE] scaffold api+worker, go module, config loader (env-based) · [FE] Next.js route + `@mywork/ui` shell · [DevOps] docker-compose dev stack · [SA] repo structure ADR

**E1-S2. Database schema & migrations (4 MD)**
- AC: ตารางตาม `06-data-model-api.md` ด้วย golang-migrate; partition executions/steps/audit; seed roles/permissions
- Tasks: [BE] migrations, sqlc/pgx layer, partition maintenance job · [QA] migration up/down test

**E1-S3. Helm chart & AKS deployment (6 MD)**
- AC: chart `deploy/charts/automate` (web/api/worker/ingress/hpa/networkpolicy), values per env, deploy Dev namespace ผ่าน pipeline สำเร็จ; probes ครบ
- Tasks: [DevOps] chart + values + Kong route · [DevOps] KEDA scaler (Temporal queue depth) · [BE] health endpoints

**E1-S4. CI/CD pipeline + quality gates (6 MD)**
- AC: pipeline ตาม `07-security-testing.md` §5.1 — lint→test→SonarQube gate→gitleaks→build→**Trivy (fail HIGH/CRITICAL)**→SBOM→deploy Dev; badge/report มองเห็นได้
- Tasks: [DevOps] pipeline yaml, SonarQube project, Trivy step, syft SBOM · [DevOps] policy check `AUTH_LOCAL_ENABLED` ห้าม true บน SIT/UAT/Prod values

**E1-S5. Azure Key Vault integration (5 MD)**
- AC: Workload Identity federated กับ SA ของ api/worker; `SecretResolver` interface (AKV + local file impl); ไม่มี secret ใน env/values (ตรวจด้วย gitleaks + review)
- Tasks: [DevOps] AKV + WI setup (Terraform) · [BE] resolver + cache 5 นาที + unit tests

**E1-S6. File storage abstraction (4 MD)**
- AC: `FileStore` interface; `LocalFileStore` (PVC) ใช้บน Dev, `AzureBlobStore` บน SIT+; signed URL รองรับทั้งสอง; สลับด้วย env เดียว
- Tasks: [BE] interface + 2 impls + checksum + retention field · [DevOps] Blob container + lifecycle policy (Terraform) · [QA] integration test (Azurite)

**E1-S7. Temporal setup & workflow skeleton (8 MD)**
- AC: Temporal namespace `automate`; generic interpreter workflow เดินกราฟ 3 node dummy ได้ (sequential + branch); payload > 512KB เก็บ Blob ส่ง ref; pod kill กลางทาง run รันต่อจนจบ (พิสูจน์ durable)
- Tasks: [BE] worker bootstrap, interpreter workflow, activity dispatcher, payload offloading · [BE] cancel/signal handling · [QA] kill-pod test script

---

## EPIC E2 — Authentication & RBAC (~34 MD)

**E2-S1. Entra ID OIDC login (6 MD)**
- AC: login ผ่าน Entra (PKCE) จาก UI สำเร็จ; JWT validate (JWKS, iss/aud); user provision อัตโนมัติครั้งแรก; group→role mapping ทำงาน; logout ล้าง session
- Tasks: [FE] NextAuth OIDC flow · [BE] JWT middleware + JWKS cache + user provisioning · [BE] group-role sync job · [QA] token expiry/refresh tests

**E2-S2. Local user login for Dev (4 MD)**
- AC: เมื่อ `AUTH_LOCAL_ENABLED=true` มีฟอร์ม login local; argon2id, lockout 5/15นาที; runtime guard: env=production + flag=true → app ไม่ start; audit ทุก attempt
- Tasks: [BE] local auth endpoints + guard + seed script · [FE] conditional login form (`GET /auth/config`) · [QA] lockout + prod-guard tests

**E2-S3. Roles, permissions & flow grants (8 MD)**
- AC: 4 system roles ตาม matrix ใน `07`; RBAC middleware ทุก endpoint (deny by default); flow/folder grants (viewer/editor/owner + inherit); list เห็นเฉพาะที่มีสิทธิ์; e2e test matrix ต่อ role ผ่านครบ
- Tasks: [BE] permission model + middleware + object-level checks · [FE] Admin UI: users/roles/grants · [FE] share dialog ต่อ flow · [QA] RBAC matrix automated tests (Playwright)

**E2-S4. Column-level data masking engine (10 MD)** ⭐ differentiator
- AC: masking_rules CRUD (Admin UI); บังคับใช้ 3 จุด server-side — query preview, step I/O ก่อนเขียน DB, ก่อนส่ง AI; styles full/partial_last4/hash; exempt roles เห็นจริง + audit `masking.bypass_view`; unit coverage ≥ 90% ของ engine
- Tasks: [BE] rule engine (regex/column match) + interceptors 3 จุด · [FE] Admin masking UI + preview ตัวอย่าง mask · [QA] bypass attempt tests · [SA] default rule set (salary, citizen_id, bank_account, phone, email)

**E2-S5. Audit logging (6 MD)**
- AC: ทุก event ใน `07 §6` ถูกเก็บ append-only (DB role ไม่มี UPDATE/DELETE); viewer UI filter action/user/date + export CSV; stream เข้า ELK
- Tasks: [BE] audit service + middleware hooks · [FE] audit viewer · [DevOps] ELK shipping

---

## EPIC E3 — Flow Designer Canvas (~46 MD)

**E3-S1. Canvas core: drag & drop + connect (10 MD)**
- AC: FR-CANVAS-002,003 — palette (จัดกลุ่ม+ค้นหา), ลากวาง, ต่อเส้น (validate port type), ลบ, undo/redo ≥ 20, pan/zoom/fit/mini-map
- Tasks: [FE] React Flow setup + custom node/edge components · [FE] undo/redo store · [FE] palette from `GET /nodes`

**E3-S2. Node config panel — JSON Schema-driven (8 MD)**
- AC: FR-CANVAS-005 — เปิด panel จาก node, ฟอร์ม render จาก schema (string/number/enum/array/nested/conditional fields), validate real-time, badge แดงเมื่อไม่ครบ
- Tasks: [FE] schema→form renderer (RHF+zod) · [BE] node registry endpoint (schema ต่อ type) · [QA] schema edge cases

**E3-S3. Expression editor + data picker (8 MD)**
- AC: FR-CANVAS-006, FR-LOGIC-007 — ทุก expression field มี autocomplete `$node/$flow/$vars` + pipe functions; picker แสดง field จริงจาก sample output ของ node ก่อนหน้า; invalid expression ขึ้น error inline
- Tasks: [BE] expression engine (expr-lang) + sandbox + function lib + unit/fuzz tests · [FE] editor (CodeMirror) + picker tree · [BE] sample-data propagation API

**E3-S4. Flow validation & test run (8 MD)**
- AC: FR-CANVAS-007,008 — `POST /validate` รายงานปัญหา (no trigger, node ลอย, config invalid, credential หาย); Test Run ทั้ง flow (sandbox, ไม่นับ schedule) เห็นสถานะ realtime ต่อ node บน canvas; Test node เดี่ยวด้วย sample input
- Tasks: [BE] validator (graph + config + refs) · [BE] test-run mode (temporal หรือ inline) + SSE/websocket status · [FE] run overlay บน canvas + step result viewer

**E3-S5. Canvas UX polish (6 MD)**
- AC: FR-CANVAS-009,010 — copy/paste/duplicate, autosave 30s + dirty indicator, sticky notes; i18n TH/EN
- Tasks: [FE] ทั้งหมด + [QA] usability checklist

**E3-S6. Folder & flow list (6 MD)**
- AC: FR-CANVAS-001 — list/ค้นหา/filter status, folders, สร้าง flow ใหม่, soft delete + restore (Admin)
- Tasks: [FE] list UI · [BE] CRUD + soft delete + RBAC filter

---

## EPIC E4 — Flow Lifecycle & Versioning (~24 MD)

**E4-S1. State machine Draft/Published/Paused/Stopped (6 MD)**
- AC: FR-LIFE-001..005 — transitions ตามตารางเท่านั้น (อื่น 409); Pause = pause Temporal schedule (run ค้างรันจบ); Stop = delete schedule + graceful cancel runs; UI badge สถานะ + ปุ่มตาม role
- Tasks: [BE] state machine + temporal schedule ops · [FE] lifecycle controls + confirm dialogs · [QA] transition matrix tests

**E4-S2. Publish & versioning (8 MD)**
- AC: FR-LIFE-003,006 — publish → validate → snapshot immutable version (running ใช้ pinned version); change note บังคับ; version list พร้อม metadata
- Tasks: [BE] version service + pin ใน workflow start · [FE] publish dialog + version list

**E4-S3. Diff viewer & rollback (6 MD)**
- AC: FR-LIFE-007,008 — diff 2 versions (nodes added/removed/changed + config field diff); rollback สร้าง version ใหม่
- Tasks: [BE] structural diff · [FE] diff UI (side-by-side graph highlight) · [QA] rollback tests

**E4-S4. Export / Import flow (4 MD)**
- AC: FR-LIFE-009 — export JSON (strip credentials → placeholder), import พร้อม remap connections/templates + validation report
- Tasks: [BE] export/import + remap wizard API · [FE] wizard UI

---

## EPIC E5 — Execution Engine, Scheduler & Run History (~40 MD)

**E5-S1. Recurring trigger via Temporal Schedules (8 MD)**
- AC: FR-TRIG-001,002,006,007 — simple mode + cron mode (preview next 5, ภาษาคน), timezone Asia/Bangkok default, overlap policy skip/queue/parallel, effective range; scheduler lag < 30s (วัดใน SIT)
- Tasks: [BE] schedule sync on publish/pause/stop · [FE] trigger config UI (simple + cron helper) · [QA] timezone & DST tests, spike test 100 flows/นาทีเดียว

**E5-S2. Manual trigger + input params (4 MD)**
- AC: FR-TRIG-003 — Run now + ฟอร์ม parameter ตาม definition; 202 + ติดตามสถานะ
- Tasks: [BE] run endpoint · [FE] run dialog

**E5-S3. Execution recording (per-step I/O) (8 MD)**
- AC: FR-RUN-001,002 — executions/steps ถูกเขียนครบทุก run; I/O snapshot truncate 1,000 items + **masked ก่อนเขียน**; duration/attempt/error detail ครบ
- Tasks: [BE] recorder activity interceptor · [BE] truncation + masking hook · [QA] volume test (100k rows step)

**E5-S4. Run history UI + drill-down (8 MD)**
- AC: list runs (filter สถานะ/ช่วงเวลา/trigger/version) + drill-down เห็น graph ทับสถานะต่อ node, I/O viewer (masked), error + retry count; live update ของ run ที่กำลังรัน
- Tasks: [FE] history list + run detail (canvas replay) · [BE] SSE status stream

**E5-S5. Cancel / Re-run / Resume-from-failed (6 MD)**
- AC: FR-RUN-003,004 — cancel graceful; re-run ทั้ง flow; resume จาก node fail ใช้ output เดิม (ต้องมี snapshot ครบ ถ้า truncated → เตือนว่า re-run เต็มเท่านั้น)
- Tasks: [BE] cancel signal + resume logic · [FE] ปุ่ม + confirm

**E5-S6. Failure notifications + dashboard (6 MD)**
- AC: FR-RUN-005,006 — email/Teams webhook เมื่อ fail (config ต่อ flow); dashboard: runs วันนี้, success rate, recent failures, upcoming schedules
- Tasks: [BE] notifier + metrics endpoints · [FE] dashboard page

---

## EPIC E6 — Database Query Node (~30 MD)

**E6-S1. Connection management (Admin) (6 MD)**
- AC: FR-DB-001,002 — CRUD postgres connections (secret → Key Vault ผ่านฟอร์ม ไม่แตะ DB เรา), test connection, allowed roles; connection dropdown ใน node เห็นตามสิทธิ์
- Tasks: [BE] connection service + AKV write + test · [FE] Admin connections UI

**E6-S2. Visual query builder (8 MD)**
- AC: FR-DB-003 — schema/table/column pickers จาก information_schema (cache), WHERE builder (AND/OR groups), orderBy, limit; gen SQL ให้ดูได้
- Tasks: [BE] metadata endpoint + builder→SQL compiler (parameterized) · [FE] builder UI

**E6-S3. SQL mode + SELECT-only guard (6 MD)**
- AC: FR-DB-004,005 — editor (highlight + autocomplete), backend parse ด้วย pg_query: บล็อก non-SELECT/multi-statement/DDL; params bind $n เท่านั้น; pen-test style unit cases (comment tricks, CTE with DML, `;`) ผ่านครบ
- Tasks: [BE] parser guard + params · [FE] editor · [QA] injection test suite

**E6-S4. Query executor + preview (6 MD)**
- AC: FR-DB-006..008,010 — execute เป็น activity: timeout/maxRows enforced, pool ต่อ connection + concurrency cap; preview 50 แถว (masked) < 3s; output `items[]`+meta
- Tasks: [BE] executor + pool manager · [FE] preview table ใน panel

**E6-S5. AI SQL Assist (4 MD)**
- AC: FR-AI-003 — NL→SQL ส่งเฉพาะ schema names; แสดงให้ review + explanation; insert เข้า editor; audit ai.call
- Tasks: [BE] assist endpoint (Azure OpenAI) + prompt template · [FE] assist drawer

---

## EPIC E7 — File Generation & Template Designer (~56 MD) ⭐ หัวใจของระบบ

**E7-S1. File Gen node core + auto layout (8 MD)**
- AC: FR-FILE-001..003,011,012 — gen XLSX/CSV/TXT/PDF แบบ auto layout จาก items; dynamic filename expression; onEmpty policy; เก็บ FileStore + metadata/checksum/retention; purge job
- Tasks: [BE] node executor + writers (excelize stream / csv / txt / maroto) · [BE] retention purge cron · [QA] golden-file tests

**E7-S2. Template model + CRUD + versioning (8 MD)**
- AC: FR-FILE-007,009 — templates + versions (change note), library (ค้นหา/หมวด/RBAC), export `.mwt` / import พร้อม validation
- Tasks: [BE] template service · [FE] library UI + import/export

**E7-S3. Excel Template Designer (12 MD)**
- AC: FR-FILE-004,008 — designer: sheets, header/footer rows (static+expression), column mapping (source→title/width/align/numFmt/date), styles (สี/ฟอนต์/หนา/border), summary rows (SUM/COUNT/AVG), freeze; **live preview จาก sample 50 แถว render จริง**
- Tasks: [FE] designer UI (grid-based) · [BE] renderer + preview endpoint · [QA] format matrix tests (number/date/thai text)

**E7-S4. PDF Template Designer (10 MD)**
- AC: FR-FILE-005,008 — page setup, header (logo upload, title, date expr), table mapping, footer (page no.), **ฟอนต์ไทย TH Sarabun render ถูกต้อง** (สระ/วรรณยุกต์), preview PDF จริง
- Tasks: [FE] designer UI (page preview) · [BE] maroto renderer + thai font embed + line-break ไทย · [QA] visual regression (pdf→png diff)

**E7-S5. TXT Template Designer — Fixed-width & Delimited (10 MD)**
- AC: FR-FILE-006,008 — delimited (ตัวคั่น custom) + fixed-width (width/pad/align ต่อ field พร้อม ruler UI), encoding UTF-8/**TIS-620**, header/trailer record (expression: count, sum, checksum); preview raw + ruler; ตัวอย่างไฟล์ตรง spec ธนาคารตัวอย่าง 1 ฟอร์แมต end-to-end
- Tasks: [FE] fixed-width designer (ruler) · [BE] encoder + TIS-620 + trailer functions · [QA] byte-exact golden tests

**E7-S6. Large file handling (8 MD)**
- AC: FR-FILE-010, NFR-PERF-005,006 — stream write 1M rows XLSX < 20 นาที / mem < 2GB; auto-split `_partN`; sensitive flag → encrypted path
- Tasks: [BE] streaming + split + memory profiling · [QA] k6/perf harness สำหรับ file gen

---

## EPIC E8 — Delivery Nodes (~30 MD)

**E8-S1. MFT/SFTP node (10 MD)**
- AC: FR-DELIV-001,002,005 — ส่งไฟล์: auth password/ssh-key (KV), hostkey verify, atomic .tmp→rename, mkdir -p, checksum verify, retry+backoff, idempotent (มีไฟล์ checksum ตรง = success); แจ้งเตือนเมื่อ fail ครบ retry; ทดสอบกับ MFT จริงขององค์กร 1 ปลายทาง
- Tasks: [BE] sftp executor (pkg/sftp) + idempotency · [FE] config UI · [DevOps] egress whitelist · [QA] integration (sftp container + fault injection)

**E8-S2. Email node (8 MD)**
- AC: FR-DELIV-003 — SMTP org + MS Graph transports; to/cc/bcc static+dynamic; subject/body expression + HTML template; attach (≤ limit, เกิน → signed link); sendMode single/perItem; rate limit ต่อ run
- Tasks: [BE] executor 2 transports · [FE] config UI + HTML template editor · [QA] mailhog integration tests

**E8-S3. Download node + My Files (6 MD)**
- AC: FR-DELIV-004 — ไฟล์โผล่ run history + หน้า My Files; signed URL expiry config; RBAC + audit ทุก download; notify users ว่าไฟล์พร้อม
- Tasks: [BE] signed URL + audit · [FE] My Files page + download UI

**E8-S4. Parallel delivery branches (6 MD)**
- AC: FR-DELIV-005 — File Gen ต่อออก MFT+Email พร้อมกัน; branch หนึ่ง fail ไม่ block อีก branch (นโยบายรวมผลตาม onError); merge/wait-all ทำงานถูก
- Tasks: [BE] parallel dispatch ใน interpreter · [QA] partial-failure scenarios

---

## EPIC E9 — Logic & Transform Nodes (~22 MD)

**E9-S1. If node (4 MD)** — AC: FR-LOGIC-001; UI condition builder; edge label True/False; unit ครบ operator matrix
**E9-S2. For-Each node (8 MD)** — AC: FR-LOGIC-003; item/batch, parallelism 1–10, maxIterations, error policy stop/collect; implement child-workflow/batched activities; perf test 10k items
**E9-S3. Transform node (8 MD)** — AC: FR-LOGIC-004; pipeline ops (select/rename/filter/sort/aggregate/computed/join 2 sources); preview ผลบน sample; unit ครบทุก op
**E9-S4. Error handling per node (2 MD ผูกกับ interpreter)** — AC: FR-LOGIC-008; onError fail/continue/errorBranch + retry per node จาก config → Temporal retry policy; error edge สีแดงบน canvas

---

## EPIC E10 — AI Node & AI Assist (~20 MD)

**E10-S1. AI Task node (10 MD)**
- AC: FR-AI-001,002,006 — presets 5 แบบ + custom; Azure OpenAI/Foundry เท่านั้น; structured output ตาม schema → items; batch; **masking ก่อนส่งเสมอ**; token/cost log ต่อ run + per-flow quota; graceful degrade เมื่อ AI ล่ม (retry → fail ตาม policy)
- Tasks: [BE] executor + structured output + quota · [FE] config UI (preset wizard) · [QA] masking-into-AI tests

**E10-S2. Flow Explain (4 MD)** — AC: FR-AI-004; ปุ่ม "อธิบาย flow" → สรุป TH/EN; ใช้ definition ไม่ใช้ data
**E10-S3. AI observability & admin (6 MD)** — AC: admin เปิด/ปิด model, ดู usage/cost ต่อ flow/ทีม; audit ai.call ครบ

---

## EPIC E11 — Observability & Operations (~14 MD)

**E11-S1. OpenTelemetry → App Insights (6 MD)** — AC: NFR-OPS-001,002; 1 run = 1 trace (node = span); metrics ครบ; dashboards เริ่มต้น
**E11-S2. Alerts (4 MD)** — AC: NFR-OPS-003; alert rules + routing (Teams/Email on-call)
**E11-S3. Structured logging → ELK (4 MD)** — AC: NFR-OPS-004; JSON + run_id correlation; scrubber ทำงาน

---

## EPIC E12 — Security Hardening & Quality Gates (~28 MD)
> งานตัดขวางที่ต้อง "จบเป็นชิ้น" ก่อน go-live (นอกเหนือจาก gate ใน E1-S4)

**E12-S1. Perf test suite (k6) + baseline (8 MD)** — AC: scenario ครบ NFR-PERF-001..009 + soak 8 ชม.; รันใน SIT อัตโนมัติทุก release; baseline + regression gate 20%
**E12-S2. DAST + pen test readiness (6 MD)** — AC: ZAP baseline ใน pipeline; pen test scope doc + test accounts + env พร้อม; ปิด finding Critical/High ก่อน prod
**E12-S3. Security review chain (6 MD)** — AC: threat model workshop (STRIDE) มี mitigations mapped; SSRF/path traversal/sandbox controls มี tests; secrets audit (ไม่มีหลุด)
**E12-S4. DR & backup validation (4 MD)** — AC: PITR restore drill ผ่าน; runbook: worker คั่ง, schedule ค้าง, KV ล่ม
**E12-S5. Compliance pack (4 MD)** — AC: PDPA data-flow diagram, audit retention config, เอกสารส่งทีม compliance/BOT review

---

## EPIC E13 — UAT, Docs & Rollout (~18 MD)
**E13-S1. User docs + in-app guide (6 MD)** — คู่มือ TH: สร้าง flow แรก, template designer, FAQ; tooltip/onboarding tour
**E13-S2. UAT กับ HR ops (6 MD)** — AC: NFR-UX-002 ผ่าน (สร้าง flow Query→Excel→Email ใน ≤ 15 นาที); feedback log + fix รอบ 1
**E13-S3. Pilot rollout (6 MD)** — 3 use cases จริง (เช่น payroll report → MFT ธนาคาร, headcount รายวัน → email ผบห., ไฟล์ TIS-620 ส่งราชการ) รันจริง 2 สัปดาห์ monitor

---

## สรุป Estimate & ลำดับ

| Epic | MD | Sprint (ทีม ~8 คน/สาย, sprint 2 สัปดาห์) |
|---|---|---|
| E1 Foundation | 38 | S1–S2 |
| E2 Auth & RBAC | 34 | S2–S4 |
| E3 Canvas | 46 | S3–S6 |
| E4 Lifecycle/Versioning | 24 | S5–S7 |
| E5 Engine/Scheduler/History | 40 | S4–S8 |
| E6 DB Query Node | 30 | S5–S8 |
| E7 File Gen + Templates | 56 | S6–S10 |
| E8 Delivery | 30 | S8–S11 |
| E9 Logic | 22 | S7–S9 |
| E10 AI | 20 | S9–S11 |
| E11 Observability | 14 | S8–S10 |
| E12 Security/Quality | 28 | S10–S12 |
| E13 UAT/Rollout | 18 | S12–S13 |
| **รวมทุกเฟส** | **~466 MD** | Full scope ~13 sprints — **Phase 1 (MVP) = ~258 MD / ~9 sprints** ตาม `09-phase-plan.md` ด้วย squad 1 ทีม (4 BE, 3 FE, 1 DevOps, 2 QA) |

**Dependency หลัก:** E1 → ทุกอย่าง | E2-S4 (masking) ก่อน E5-S3 (recorder), E6-S4 (preview), E10-S1 (AI) | E3-S3 (expression) ก่อน E7/E8 config | E5-S1 (schedule) หลัง E4-S2 (publish)

**ความเสี่ยง top 3 (ยกจากบทเรียน sunset analysis):**
1. E7 Template Designer ใหญ่สุดและเป็น differentiator — แตก spike ต้นทาง (S5–S6) เพื่อพิสูจน์ renderer ไทย/TIS-620 ก่อนลงทุน UI เต็ม
2. Dev Lead ครอบหลาย squad — กำหนด owner ต่อ epic ชัดเจนตั้งแต่ grooming
3. MFT จริงขององค์กร (E8-S1) ต้องขอ access/firewall ล่วงหน้า — เปิด ticket infra ตั้งแต่ S1
