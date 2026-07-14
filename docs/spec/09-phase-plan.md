# 09 — Phase Plan: MVP → Phase 2 → 3 → 4

> หลักการแบ่ง: **MVP = ใช้งานจริงได้ end-to-end ตาม requirement หลัก** — มี account/สิทธิ์, SSO login ได้, สร้างและรัน automate ได้จริง (Trigger → Query → Gen File → MFT/Email/Download) พร้อม versioning + สถานะครบ และผ่าน quality gate ที่ธนาคารบังคับ (pen test, Sonar, Trivy) — ส่วนที่ "ทำให้ดีขึ้น/ฉลาดขึ้น/กว้างขึ้น" ถัดไป Phase 2–4

## ภาพรวม

| Phase | ธีม | ระยะเวลา | Effort | ผลลัพธ์ที่ผู้ใช้ได้ |
|---|---|---|---|---|
| **1 — MVP** | ใช้งานจริงได้ end-to-end | ~9 sprints (~4.5 เดือน) | ~258 MD | Login SSO, สร้าง flow ลาก-วาง, query DB, ออกไฟล์, ส่ง MFT/Email/Download, publish/pause/stop + version |
| **2 — Designer & AI** | Template Designer เต็มรูปแบบ + AI | ~5 sprints (~2.5 เดือน) | ~130 MD | ออกแบบ template Excel/PDF/TXT fixed-width (TIS-620), AI node, AI SQL assist, masking ปรับแต่งได้ |
| **3 — Ecosystem** | ต่อโลกภายนอก + ความคล่องตัว | ~4 sprints (~2 เดือน) | ~85 MD | Webhook, HTTP node, Teams, Switch/Delay, ย้าย flow ข้าม env, dashboard เต็ม |
| **4 — Enterprise Scale** | Event-driven + intelligence | ต่อเนื่อง | ~90 MD+ | Event trigger จาก MyWork, AI anomaly, NL→Flow, multi-tenant, approval node |

---

## Phase 1 — MVP (~258 MD, ~9 sprints)

### เป้าหมาย (Definition of Success)
ผู้ใช้ HR ops ทำ **user journey นี้ได้จริงบน Production**:
1. Login ด้วย Microsoft Entra ID (Dev ใช้ local user ได้)
2. ได้รับสิทธิ์ตาม role (Admin / Flow Designer / Operator / Viewer) — เห็นเฉพาะ flow ที่มีสิทธิ์, ข้อมูล sensitive ถูก mask ตาม default rules
3. สร้าง flow บน canvas: **Schedule ทุกวัน 06:00 → Query Azure PostgreSQL (builder หรือ SQL) → If (rowCount > 0) → Gen ไฟล์ XLSX/CSV/TXT → ส่ง MFT + Email + Download**
4. Test run เห็นผลต่อ node → Publish (เกิด version v1) → ระบบรันตาม schedule จริง
5. ดู Run History drill-down ต่อ step, cancel ได้, fail แล้วมี email แจ้ง
6. Pause / Resume / Stop, แก้ draft แล้ว publish เป็น v2, rollback กลับ v1 ได้

### Scope (map จาก backlog `08`)

| Epic | Stories ที่เข้า MVP | MD | ตัดออกไป Phase |
|---|---|---|---|
| E1 Foundation | ครบทุก story (S1–S7) | 38 | — |
| E2 Auth & RBAC | S1 Entra SSO, S2 Local login, S3 Roles+grants, S5 Audit | 24 | — |
| E2-S4 Masking | **เฉพาะ engine + default rule set** (salary, citizen_id, bank_account, phone) บังคับที่ preview/log — Admin UI ปรับ rule เอง → P2 | 6 | Admin UI + custom rules → **P2** |
| E3 Canvas | S1 Canvas core, S2 Config panel, S3 Expression+picker, S4 Validation+Test run, S6 Flow list/folders | 40 | S5 Polish (copy/paste, sticky note, autosave*) → **P2** (*autosave ดึงเข้า MVP ถ้าเวลาเหลือ) |
| E4 Lifecycle | S1 State machine 4 สถานะ, S2 Publish+versioning, Rollback แบบง่าย (publish version เก่าซ้ำ) | 16 | S3 Diff viewer, S4 Export/Import → **P2/P3** |
| E5 Engine/History | S1 Schedule trigger (simple+cron), S2 Manual run, S3 Recording (masked), S4 History UI, S5 Cancel + Re-run ทั้ง flow, S6 Fail email แจ้งเตือน | 34 | Resume-from-failed-node → **P2**, Dashboard เต็ม + Teams → **P3** |
| E6 DB Query | S1 Connections (Key Vault), S2 Visual builder, S3 SQL mode + SELECT-only guard, S4 Executor + preview (masked) | 26 | S5 AI SQL Assist → **P2** |
| E7 File Gen | S1 Core: **auto layout XLSX / CSV / TXT-delimited** + dynamic filename + retention + **Excel template แบบพื้นฐาน** (header + column mapping + number format) | 18 | Designer เต็ม (Excel styles/summary, **PDF + ฟอนต์ไทย**, **TXT fixed-width + TIS-620**), template versioning/export, 1M rows/split → **P2** |
| E8 Delivery | S1 MFT/SFTP (atomic + retry + idempotent), S2 Email (SMTP/Graph + attach), S3 Download + My Files — ต่อแบบ **sequential branch** | 24 | S4 Parallel branches → **P2**, HTTP/Blob node → **P3** |
| E9 Logic | S1 If node, S3 Transform พื้นฐาน (select/rename/filter/sort), S4 Error handling per node | 12 | For-Each, aggregate/join, Switch/Delay/Merge → **P2/P3** |
| E11 Observability | S1 OTel→App Insights (trace ต่อ run + metrics หลัก), S3 JSON logs→ELK | 8 | Alert rules ละเอียด → **P2** |
| E12 Security gates | Perf smoke (k6 baseline: 100 concurrent, 100k-row file), **Pen test ก่อน go-live + ปิด Critical/High**, Sonar/Trivy อยู่ใน CI ตั้งแต่ E1-S4 | 12 | Soak 8 ชม., chaos, DR drill เต็ม → **P2** |
| E13 Rollout | UAT กับ HR ops (สร้าง flow ≤ 15 นาที) + Pilot 2 use cases จริง | 12 | Use case ที่ต้องใช้ PDF/fixed-width → รอ **P2** |
| **รวม** | | **~258** | |

### เงื่อนไขที่ MVP "ยังไม่มี" (สื่อสารผู้ใช้ให้ชัด)
- ไฟล์ PDF และ TXT แบบ fixed-width (ไฟล์ส่งแบงก์ชาติ) → Phase 2
- AI ทุกอย่าง → Phase 2
- Webhook/Event trigger, ต่อ API ภายนอก → Phase 3/4
- ปรับ masking rule เอง → Phase 2 (MVP ใช้ default set ที่ Security review แล้ว)

### Exit Criteria Phase 1
- [ ] User journey ข้อ 1–6 ผ่าน UAT โดย HR ops จริง (NFR-UX-002: flow แรก ≤ 15 นาที)
- [ ] Pilot 2 use cases รันบน Prod ต่อเนื่อง 2 สัปดาห์ success rate ≥ 99%
- [ ] Pen test: ไม่มี Critical/High ค้าง | SonarQube gate ผ่าน | Trivy ไม่มี HIGH/CRITICAL fixable
- [ ] Perf: 100 concurrent runs + query 100k แถว → XLSX < 5 นาที (NFR-PERF-005)
- [ ] `AUTH_LOCAL_ENABLED=false` บน Prod พิสูจน์ด้วย CI policy check + runtime guard
- [ ] Audit log ครบ + เข้า SIEM | Runbook on-call ฉบับแรก

### Sprint map (9 sprints)
| Sprint | โฟกัส |
|---|---|
| S1–S2 | E1 ทั้งหมด + E2-S1/S2 (login ได้ตั้งแต่ S2) |
| S3–S4 | E2-S3/S5 + masking default, E3-S1/S2/S6, E5-S1 โครง schedule |
| S5–S6 | E3-S3/S4, E4-S1/S2, E6-S1/S2 |
| S7 | E6-S3/S4, E7-S1, E9 |
| S8 | E8 ทั้งสาม node, E5-S3/S4/S5/S6, E11 |
| S9 | Hardening: E12 (perf+pen test window), UAT, pilot cutover |

---

## Phase 2 — Template Designer เต็มรูปแบบ + AI (~130 MD, ~5 sprints)

**ธีม:** เปลี่ยนจาก "ออกไฟล์ได้" เป็น "ออกไฟล์ตามฟอร์แมตองค์กร/ธนาคารได้ทุกแบบ" + ฉลาดขึ้นด้วย AI

| กลุ่ม | รายการ | MD |
|---|---|---|
| Template Designer | Excel designer เต็ม (styles, summary, freeze, multi-sheet), **PDF designer + ฟอนต์ไทย TH Sarabun**, **TXT fixed-width + TIS-620 + header/trailer record**, template versioning + export/import `.mwt`, template library + RBAC, large file 1M rows + auto-split | 48 |
| AI | AI Task node (presets + structured output + masking-before-AI + quota), AI SQL Assist, Flow Explain, AI admin/usage | 20 |
| RBAC/Masking | Masking Admin UI + custom rules + partial/hash styles ครบ | 6 |
| Engine/UX ที่ค้าง | Resume-from-failed-node, Parallel delivery branches, For-Each node, Transform ครบ (aggregate/join), Diff viewer, Canvas polish (copy/paste, autosave, sticky), Alert rules ละเอียด | 44 |
| Quality | Soak test 8 ชม., DR restore drill, chaos test (kill worker) พิสูจน์ durable | 12 |

**Exit:** ออกไฟล์ TXT fixed-width TIS-620 ตรง byte-exact กับ spec ธนาคารจริง 1 ฟอร์แมต + PDF ภาษาไทย render ถูกต้อง + AI node ใช้จริงใน 1 use case (เช่น classify คำขอ HR)

---

## Phase 3 — Ecosystem & Operations (~85 MD, ~4 sprints)

| กลุ่ม | รายการ |
|---|---|
| Triggers/Nodes | Webhook trigger (HMAC), HTTP node (allowlist + SSRF guard), Switch, Delay (durable timer), Merge, Teams notification node |
| ALM | Export/Import flow ข้าม environment (Dev→SIT→Prod pipeline), environment banner + per-env config |
| Ops | Dashboard เต็ม (success rate, upcoming, slow runs), usage quota ต่อ flow/ทีม, next-node suggest (AI) |
| Governance | Custom roles, connection-level approval workflow, สรุปรายงาน usage ต่อเดือนให้ผู้บริหาร |

**Exit:** ระบบภายนอก trigger flow ผ่าน webhook ได้จริง + ย้าย flow ข้าม env ผ่าน pipeline โดยไม่ manual แก้ config

---

## Phase 4 — Enterprise Scale & Intelligence (~90 MD+, ต่อเนื่อง)

| กลุ่ม | รายการ |
|---|---|
| Event-driven | Event trigger จาก MyWork event bus (employee.created, payroll.closed) — ปลดล็อก automation แบบ real-time |
| Formats | DOCX / JSON / XML, util.archive (zip+password), delivery.blob |
| AI ขั้นสูง | ai.anomaly (ตรวจ data ผิดปกติ), **NL→Flow** (gen ทั้ง flow จากคำอธิบาย) |
| Platform | Multi-tenant เต็มรูปแบบ (แยกต่อบริษัทลูกค้า MyWork), Template/Flow marketplace ภายใน, Approval/Human-task node (BPMN-style) |
| Scale | Worker autoscale ทดสอบถึง 20 pods, cross-region DR ถ้านโยบายต้องการ |

**Exit:** MyWork Automate เป็น automation layer กลางของ MyWork ecosystem — ลูกค้า enterprise สร้าง automation เองได้โดยไม่พึ่งทีม dev

---

## หลักการตัดสินใจเวลาของบีบ (ถ้า MVP ต้องเร็วกว่านี้)
ตัดตามลำดับนี้โดยยังรักษา user journey หลัก: (1) TXT delimited → เหลือ XLSX/CSV ก่อน (2) Visual query builder → เหลือ SQL mode + guard (builder ตามใน P2) (3) Re-run → เหลือ cancel อย่างเดียว — **ห้ามตัด:** SSO, RBAC+masking default, versioning/สถานะ, MFT atomic+retry, pen test gate (ของบังคับธนาคารทั้งหมด)
