# 03 — Non-Functional Requirements (NFR)

> รูปแบบ: `NFR-<area>-<no>` — ทุกข้อมีตัวเลขวัดได้ ใช้เป็น exit criteria ของ perf test / security test

## NFR-PERF — Performance

| ID | Requirement | Target |
|---|---|---|
| NFR-PERF-001 | Canvas โหลด flow ขนาด 50 nodes | < 2s (P95) |
| NFR-PERF-002 | API response หน้า list/CRUD ทั่วไป | < 500ms (P95), < 1s (P99) |
| NFR-PERF-003 | Scheduler dispatch ตรงเวลา | คลาดเคลื่อน < 30s จากเวลาที่ตั้ง |
| NFR-PERF-004 | Concurrent flow executions | ≥ 100 flows พร้อมกัน (scale worker แนวนอนได้) |
| NFR-PERF-005 | Query 100,000 แถว → gen XLSX → upload Blob | < 5 นาที ต่อ run |
| NFR-PERF-006 | Gen XLSX 1,000,000 แถว (stream mode) | < 20 นาที, memory worker < 2GB |
| NFR-PERF-007 | Preview query (50 แถว) | < 3s |
| NFR-PERF-008 | Concurrent UI users | ≥ 200 users (MyWork admin base) |
| NFR-PERF-009 | Throughput ระบบรวม | ≥ 5,000 flow runs/วัน โดย success rate ≥ 99.5% |

**Perf test (k6):** load test (baseline + 2× peak), stress test (หา breaking point), soak test 8 ชม. (memory leak), spike test scheduler (100 flows trigger นาทีเดียวกัน) — รันใน SIT ทุก release, report แนบ definition of done

## NFR-AVAIL — Availability & Reliability

| ID | Requirement | Target |
|---|---|---|
| NFR-AVAIL-001 | Uptime (business hours 06:00–22:00) | ≥ 99.9% |
| NFR-AVAIL-002 | Flow ที่รันค้างระหว่าง pod restart/deploy | ต้องรันต่อจนจบ (durable execution — Temporal) ไม่มี run หาย |
| NFR-AVAIL-003 | ระบบ recover จาก AKS node failure | อัตโนมัติ, RTO < 5 นาที |
| NFR-AVAIL-004 | Backup PostgreSQL | PITR, RPO ≤ 5 นาที; ทดสอบ restore ทุกไตรมาส |
| NFR-AVAIL-005 | ทุก delivery node (MFT/Email) | idempotent — retry แล้วไม่ส่งซ้ำซ้อน (dedup key ต่อ run+node) |
| NFR-AVAIL-006 | Graceful shutdown | worker drain งานก่อนปิด ≤ 60s |

## NFR-SEC — Security (สรุป — รายละเอียดที่ `07-security-testing.md`)

| ID | Requirement |
|---|---|
| NFR-SEC-001 | AuthN: Microsoft Entra ID (OIDC + PKCE); Local login เปิดได้เฉพาะ environment ที่ flag `AUTH_LOCAL_ENABLED=true` (Dev/Local เท่านั้น — CI บล็อกไม่ให้ flag นี้ true บนภาพ Prod) |
| NFR-SEC-002 | AuthZ: RBAC ทุก API + column-level data masking; deny by default |
| NFR-SEC-003 | Secrets ทั้งหมด (DB password, SFTP key, SMTP, AI key) อยู่ใน Azure Key Vault เท่านั้น — ห้ามอยู่ใน DB/config/env ของ app; ใช้ Workload Identity (ไม่มี client secret ใน cluster) |
| NFR-SEC-004 | Encrypt in transit: TLS 1.2+ ทุกเส้น (รวม pod-to-pod ผ่าน mesh/Kong); at rest: PostgreSQL TDE, Blob SSE |
| NFR-SEC-005 | SQL injection: parameterized query เท่านั้น; SQL mode ผ่าน parser whitelist (SELECT-only) ที่ backend |
| NFR-SEC-006 | Quality gates ใน CI/CD (ทุก PR + ทุก release): SonarQube (SAST, quality gate pass, 0 blocker/critical), **Trivy** (image + dependency CVE — fail ถ้ามี HIGH/CRITICAL ที่มี fix), secret scanning (gitleaks), SBOM (syft) |
| NFR-SEC-007 | Pen test โดยทีมภายนอก/ภายในตามรอบธนาคาร ก่อน go-live + ประจำปี — ปิด finding severity High ขึ้นไปก่อน production |
| NFR-SEC-008 | Audit log ครบ append-only, ส่งเข้า SIEM (ELK), retention ตามนโยบายธนาคาร (≥ 1 ปี online) |
| NFR-SEC-009 | PDPA: ข้อมูลส่วนบุคคลใน execution log ถูก mask ตาม rule; สิทธิ์ลบข้อมูล (data subject request) รองรับผ่าน retention + purge job |
| NFR-SEC-010 | Rate limiting ที่ Kong: ต่อ user ต่อ endpoint; brute-force protection ที่ local login (lockout 5 ครั้ง) |

## NFR-SCALE — Scalability

| ID | Requirement |
|---|---|
| NFR-SCALE-001 | แยก plane: API server กับ Execution worker เป็น deployment คนละตัว — scale อิสระ (HPA: CPU + queue depth) |
| NFR-SCALE-002 | Worker เพิ่มจาก 2 → 20 pods ได้โดยไม่ต้องแก้ config |
| NFR-SCALE-003 | รองรับ multi-tenant ในอนาคต (tenant_id ในทุกตาราง ตั้งแต่วันแรก) |

## NFR-OPS — Observability & Operations

| ID | Requirement |
|---|---|
| NFR-OPS-001 | OpenTelemetry (traces + metrics + logs) → Azure Application Insights; trace ครอบ 1 flow run ต่อ 1 trace (เห็นทุก node เป็น span) |
| NFR-OPS-002 | Metrics หลัก: runs_total{status}, run_duration, node_duration{type}, queue_depth, scheduler_lag, file_bytes_generated, ai_tokens_used |
| NFR-OPS-003 | Alert: success rate < 95% (15 นาที), scheduler lag > 2 นาที, queue depth > 500, worker OOM |
| NFR-OPS-004 | Structured JSON logs, correlation id (run_id) ทุกบรรทัด — เข้า ELK ที่มีอยู่ |
| NFR-OPS-005 | Health/readiness probes ทุก service; Helm chart + values แยก per environment; deploy ผ่าน pipeline เดียวกับ MyWork (GitOps) |

## NFR-UX — Usability

| ID | Requirement |
|---|---|
| NFR-UX-001 | UI สองภาษา (TH/EN) ตาม MyWork i18n |
| NFR-UX-002 | ผู้ใช้ non-technical (HR ops) สร้าง flow "Query → Excel → Email" ได้เองใน ≤ 15 นาที หลัง training 1 ชม. (วัดด้วย UAT usability test) |
| NFR-UX-003 | ทุก error message บอกสาเหตุ + วิธีแก้ (ไม่โชว์ stack trace ให้ end user) |
| NFR-UX-004 | รองรับ Chrome/Edge ล่าสุด 2 versions; responsive ≥ 1280px (canvas ไม่บังคับ mobile) |

## NFR-DATA — Data Management

| ID | Requirement |
|---|---|
| NFR-DATA-001 | Flow definition เก็บเป็น JSONB + schema version (รองรับ migration ของ definition format) |
| NFR-DATA-002 | ไฟล์ dev เก็บ local volume (`/data/files`) — prod เก็บ Blob (hot tier, lifecycle → cool 30 วัน) ผ่าน interface `FileStore` เดียวกัน |
| NFR-DATA-003 | Execution step I/O เก็บแบบ truncate (เก็บ 1,000 items แรกต่อ step, ที่เหลือเก็บ count) กัน DB บวม |
| NFR-DATA-004 | Data residency: ทุก resource อยู่ Azure Southeast Asia region |
