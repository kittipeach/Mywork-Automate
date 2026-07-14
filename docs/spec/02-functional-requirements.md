# 02 — Functional Requirements (ละเอียด)

> รูปแบบ: `FR-<module>-<no>` | Priority: **M**ust / **S**hould / **C**ould (MoSCoW) | ทุกข้อคือ testable requirement สำหรับ QA เขียน test case ได้ทันที
> **Phase mapping:** Priority **M** = Phase 1 (MVP) เว้นแต่ระบุเฟสไว้ในข้อความ (เช่น "Phase 2") | **S** = Phase 2–3 | **C** = Phase 3–4 — การจัดสรรอย่างเป็นทางการยึดตาม `09-phase-plan.md` ซึ่ง override ตารางนี้
> ข้อยกเว้นสำคัญใน MVP: FR-FILE-005 (PDF), FR-FILE-006 (TXT fixed-width/TIS-620), FR-FILE-007 (template export), FR-AI-* ทั้งหมด, FR-LOGIC-003 (For-Each) → เลื่อนไป **Phase 2** ตามแผนเฟส

## FR-CANVAS — Flow Designer (Canvas)

| ID | Requirement | Priority |
|---|---|---|
| FR-CANVAS-001 | ผู้ใช้สร้าง Flow ใหม่ได้ โดยระบุ ชื่อ, คำอธิบาย, folder/หมวดหมู่ | M |
| FR-CANVAS-002 | Canvas แบบ drag & drop: ลาก node จาก Node Palette (ด้านซ้าย, จัดกลุ่มตาม category, ค้นหาได้) มาวางบน canvas | M |
| FR-CANVAS-003 | เชื่อม node ด้วยการลากเส้นจาก output port → input port; ลบเส้น/ลบ node ได้; รองรับ undo/redo (≥ 20 steps) | M |
| FR-CANVAS-004 | Zoom in/out, pan, fit-to-view, mini-map, auto-layout (จัดเรียง node อัตโนมัติ) | S |
| FR-CANVAS-005 | คลิก node เพื่อเปิด Config Panel (ด้านขวา) — ฟอร์ม config ตาม schema ของ node type นั้น | M |
| FR-CANVAS-006 | ทุก field ใน config อ้างอิง output ของ node ก่อนหน้าได้ผ่าน Expression `{{ $node["ชื่อnode"].output.field }}` พร้อม autocomplete picker แสดง field ที่มีจริงจาก sample data | M |
| FR-CANVAS-007 | Validation แบบ real-time: node ที่ config ไม่ครบขึ้น badge สีแดง; flow ที่ไม่มี trigger หรือมี node ลอย (ไม่เชื่อมต่อ) publish ไม่ได้ | M |
| FR-CANVAS-008 | ปุ่ม **Test Run** ทั้ง flow และ **Test Node เดี่ยว** (รันเฉพาะ node นั้นด้วย sample input) เห็นผลลัพธ์ทันทีบน canvas | M |
| FR-CANVAS-009 | Copy/paste node (พร้อม config), duplicate flow ทั้งตัว | S |
| FR-CANVAS-010 | Auto-save draft ทุก 30 วินาที + แสดงสถานะ "Saved" / "Unsaved changes" | S |
| FR-CANVAS-011 | Sticky note / annotation บน canvas สำหรับจดคำอธิบาย | C |
| FR-CANVAS-012 | รองรับ node มากกว่า requirement เริ่มต้น — Node Palette โหลดจาก **Node Registry** (เพิ่ม node ใหม่ได้โดยไม่แก้ frontend core) | M |

## FR-LIFE — Flow Lifecycle, สถานะ & Versioning

| ID | Requirement | Priority |
|---|---|---|
| FR-LIFE-001 | Flow มี 4 สถานะ: `Draft` → `Published` ⇄ `Paused` → `Stopped` (state machine ตามตาราง transition ด้านล่าง) | M |
| FR-LIFE-002 | **Draft**: แก้ไขได้อิสระ ไม่ถูก scheduler หยิบไปรัน | M |
| FR-LIFE-003 | **Published**: ระบบสร้าง version ใหม่ (immutable snapshot) — scheduler/trigger ทำงานกับ published version เท่านั้น; แก้ไขต่อได้ใน draft copy โดยไม่กระทบ version ที่รันอยู่ | M |
| FR-LIFE-004 | **Paused**: หยุด trigger ชั่วคราว (run ที่ค้างอยู่รันจนจบ), กด Resume กลับเป็น Published ได้ทันที | M |
| FR-LIFE-005 | **Stopped**: หยุดถาวร + ยกเลิก run ที่ค้างอยู่ (graceful cancel); กลับมาใช้ต้อง publish ใหม่ | M |
| FR-LIFE-006 | ทุกครั้งที่ Publish เก็บ version: หมายเลข (v1, v2, ...), ผู้ publish, เวลา, change note (บังคับกรอก) | M |
| FR-LIFE-007 | ดูรายการ version ทั้งหมด + **Diff viewer** เปรียบเทียบ 2 version (node เพิ่ม/ลบ/config เปลี่ยน) | S |
| FR-LIFE-008 | **Rollback** ไป version ก่อนหน้า (สร้างเป็น version ใหม่ ไม่เขียนทับ) | M |
| FR-LIFE-009 | Export flow เป็น JSON / Import flow จาก JSON (ใช้ย้ายข้าม environment Dev→SIT→Prod) | S |
| FR-LIFE-010 | ลบ flow = soft delete (เก็บ 90 วัน กู้คืนได้), เฉพาะ role ที่มีสิทธิ์ | S |

**State transitions ที่อนุญาต:** Draft→Published | Published→Paused | Paused→Published(Resume) | Published/Paused→Stopped | Stopped→Draft(copy ใหม่เพื่อแก้)

## FR-TRIG — Trigger Nodes

| ID | Requirement | Priority |
|---|---|---|
| FR-TRIG-001 | **Recurring Trigger**: ตั้งรอบรันแบบง่าย — ทุก N นาที / ชั่วโมง / วัน / สัปดาห์ (เลือกวัน) / เดือน (เลือกวันที่) + เวลา + timezone (default Asia/Bangkok) | M |
| FR-TRIG-002 | โหมด Advanced: ใส่ **cron expression** ได้โดยตรง พร้อมตัวช่วยแปลเป็นภาษาคน ("ทุกวันจันทร์ 08:00") และ preview เวลารัน 5 ครั้งถัดไป | M |
| FR-TRIG-003 | **Manual Trigger**: กดรันจากหน้า UI (ปุ่ม Run now) พร้อมใส่ input parameter ได้ | M |
| FR-TRIG-004 | **Webhook Trigger**: ระบบ gen URL + secret ให้ระบบภายนอกยิงเข้ามา trigger flow (HMAC validation) | S |
| FR-TRIG-005 | **Event Trigger**: รับ event จาก MyWork platform (เช่น พนักงานเข้าใหม่, payroll ปิดงวด) ผ่าน internal event bus | C (Phase 2) |
| FR-TRIG-006 | กันรันซ้อน: ถ้ารอบใหม่ถึงแต่รอบเก่ายังไม่จบ เลือกนโยบายได้ — Skip / Queue / Run parallel (default: Skip + แจ้งเตือน) | M |
| FR-TRIG-007 | ตั้งช่วงวันเริ่ม-สิ้นสุดของ schedule ได้ (effective date range) | S |

## FR-DB — Database Query Node (Azure PostgreSQL)

| ID | Requirement | Priority |
|---|---|---|
| FR-DB-001 | **Connection Management**: Admin สร้าง DB connection (host, port, database, user) — รหัสผ่าน/connection string เก็บใน **Azure Key Vault** ระบบเก็บเฉพาะ reference; ทดสอบการเชื่อมต่อ (Test connection) ได้ | M |
| FR-DB-002 | ใน Query Node ผู้ใช้เลือก connection จาก dropdown (เห็นเฉพาะ connection ที่ role ตนมีสิทธิ์ใช้) | M |
| FR-DB-003 | **Visual Query Builder**: เลือก schema → table → columns (multi-select), ใส่ WHERE condition ผ่าน UI (field + operator + value; AND/OR group ได้), ORDER BY, LIMIT | M |
| FR-DB-004 | **SQL Mode**: เขียน SQL เองใน editor (syntax highlight, autocomplete ชื่อ table/column) — อนุญาตเฉพาะ `SELECT` (บล็อก INSERT/UPDATE/DELETE/DDL ที่ engine layer, ไม่ใช่แค่ UI) | M |
| FR-DB-005 | ค่าใน condition/SQL รับ **parameter** จาก node ก่อนหน้าหรือ flow variable ผ่าน expression — bind แบบ parameterized query เท่านั้น (กัน SQL injection) | M |
| FR-DB-006 | ปุ่ม **Preview**: รัน query กับ LIMIT 50 แสดงผลเป็นตารางใน config panel (ข้อมูล sensitive ถูก mask ตาม RBAC ของผู้ที่กด preview) | M |
| FR-DB-007 | Query timeout ตั้งได้ (default 120s, max 15 นาที), จำกัดผลลัพธ์สูงสุด (default 100,000 แถว — เกินให้ error ชัดเจน + แนะนำใช้ pagination/batch) | M |
| FR-DB-008 | Output ของ node = `items[]` (array of row objects) + metadata (rowCount, columns, ชนิดข้อมูล) ส่งต่อ node ถัดไป | M |
| FR-DB-009 | รองรับ multiple query nodes ใน flow เดียว (query หลาย DB มา join ด้วย Transform node) | S |
| FR-DB-010 | Connection pool ต่อ connection + จำกัด concurrent query ต่อ DB (กัน flow ถล่ม production DB); read-only DB user เป็น default ที่แนะนำ | M |

## FR-FILE — File Generation Node + Template Designer

| ID | Requirement | Priority |
|---|---|---|
| FR-FILE-001 | File Gen Node รับ `items[]` จาก node ก่อนหน้า (เช่น Query node) แล้วสร้างไฟล์ตาม format ที่เลือก: **XLSX, PDF, TXT, CSV** (Phase 2: DOCX, JSON, XML) | M |
| FR-FILE-002 | ตั้งชื่อไฟล์แบบ dynamic ด้วย expression เช่น `payroll_{{ $flow.runDate | format "YYYYMMDD" }}.xlsx` | M |
| FR-FILE-003 | เลือกได้ 2 โหมด: **Auto layout** (gen ตาราง header = ชื่อ column อัตโนมัติ) หรือ **From Template** (เลือก template ที่ออกแบบไว้) | M |
| FR-FILE-004 | **Template Designer (Excel)**: กำหนด sheet หลายแผ่น, header/footer rows (ข้อความคงที่ + expression เช่น วันที่รัน), mapping คอลัมน์ dataset → คอลัมน์ Excel, จัดรูปแบบ (ความกว้าง, สี, ฟอนต์, ตัวหนา, border, number format, date format), แถวสรุป (SUM/COUNT/AVG), freeze panes | M |
| FR-FILE-005 | **Template Designer (PDF)**: page setup (A4/Letter, แนวตั้ง/นอน, margin), ส่วนหัว (โลโก้อัปโหลดได้, ชื่อรายงาน, วันที่), ตารางข้อมูล (เลือกคอลัมน์, ความกว้าง, alignment), ส่วนท้าย (เลขหน้า, ข้อความ), รองรับ **ฟอนต์ภาษาไทย** (TH Sarabun ฝังในระบบ) | M |
| FR-FILE-006 | **Template Designer (TXT)**: เลือกโหมด **Delimited** (กำหนดตัวคั่น: comma, pipe, tab, custom) หรือ **Fixed-width** (กำหนดความกว้าง/padding/alignment ต่อ field — สำหรับไฟล์ส่งธนาคาร/ราชการ), กำหนด encoding (UTF-8 / TIS-620), header/trailer record (เช่น จำนวนแถว, checksum) | M |
| FR-FILE-007 | Template มี **versioning** ของตัวเอง, **Export template** เป็นไฟล์ (.mwt JSON) และ **Import** กลับได้ — แชร์ template ข้าม flow / ข้าม environment | M |
| FR-FILE-008 | **Preview** ไฟล์จาก sample data (50 แถวแรก) ก่อน save template — Excel/PDF แสดง render จริง, TXT แสดง raw text | M |
| FR-FILE-009 | Template Library กลาง: ค้นหา, จัดหมวดหมู่, สิทธิ์การใช้ template ตาม RBAC | S |
| FR-FILE-010 | รองรับข้อมูลปริมาณมาก: XLSX สูงสุด 1,000,000 แถว (stream write), แจ้งเตือนถ้าเกิน + ตัวเลือก split หลายไฟล์อัตโนมัติ (`_part1`, `_part2`) | S |
| FR-FILE-011 | ไฟล์ที่ gen เสร็จเก็บใน **File Storage** (Local volume บน Dev / Azure Blob บน SIT-Prod ผ่าน abstraction เดียวกัน) พร้อม metadata (flow, run id, ขนาด, checksum SHA-256, retention) | M |
| FR-FILE-012 | Retention policy ต่อ flow: เก็บไฟล์กี่วัน (default 30, max 365) — เกินแล้วลบอัตโนมัติ + log | M |
| FR-FILE-013 | ไฟล์ที่มีข้อมูล sensitive ระบุ flag ได้ → บังคับ encrypt at rest (Blob SSE + optional password ZIP สำหรับ email) | S |

## FR-DELIV — Delivery Nodes

| ID | Requirement | Priority |
|---|---|---|
| FR-DELIV-001 | **MFT Node**: ส่งไฟล์ไปยัง MFT/SFTP server — config: connection (host, port, path ปลายทาง, credential จาก Key Vault: password หรือ SSH key), rename pattern, สร้าง directory อัตโนมัติ, ไฟล์ .tmp → rename เมื่อส่งจบ (atomic), ตรวจ checksum หลังส่ง | M |
| FR-DELIV-002 | MFT retry policy ตั้งได้ (จำนวนครั้ง, backoff), แจ้งเตือนเมื่อส่งไม่สำเร็จครบ retry | M |
| FR-DELIV-003 | **Email Node**: ส่งผ่าน SMTP องค์กร / Microsoft Graph — To/CC/BCC (รายชื่อ static + dynamic จาก data), Subject/Body รองรับ expression + HTML template, แนบไฟล์จาก node ก่อนหน้า (จำกัดขนาดรวม config ได้ default 20MB — เกินให้ส่งเป็น download link แทน) | M |
| FR-DELIV-004 | **Download Node**: ทำให้ไฟล์ดาวน์โหลดได้จากหน้า Run History — สร้าง signed URL (หมดอายุ config ได้ default 7 วัน), สิทธิ์ดาวน์โหลดตาม RBAC, log ทุกครั้งที่ดาวน์โหลด (ใคร/เมื่อไหร่) | M |
| FR-DELIV-005 | ส่งไฟล์เดียวไปหลายปลายทางได้ (ต่อ File Gen node → MFT + Email พร้อมกันแบบ parallel branch) | M |
| FR-DELIV-006 | **HTTP Node**: เรียก REST API ภายนอก (method, headers, body, auth จาก Key Vault) — เป็น escape hatch เหมือน n8n | S |
| FR-DELIV-007 | **Blob Upload Node**: อัปโหลดไฟล์ไป Azure Blob container ที่กำหนด | C |

## FR-LOGIC — Logic & Utility Nodes

| ID | Requirement | Priority |
|---|---|---|
| FR-LOGIC-001 | **If Node**: เงื่อนไข field + operator (=, ≠, >, <, ≥, ≤, contains, starts/ends with, is empty, in list) + AND/OR group → 2 ทางออก True/False | M |
| FR-LOGIC-002 | **Switch Node**: หลาย case ตามค่า field + default branch | S |
| FR-LOGIC-003 | **For-Each Node**: วนรายการ (ทั้ง item-by-item และ batch ครั้งละ N), กำหนด max iterations | M |
| FR-LOGIC-004 | **Transform Node**: map/rename field, filter rows, sort, aggregate (group by + sum/count/avg), merge/join items จาก 2 nodes, สูตรคำนวณต่อ field | M |
| FR-LOGIC-005 | **Delay Node**: หน่วงเวลา (วินาที–ชั่วโมง) — implement แบบ durable (Temporal timer) ไม่กิน worker | S |
| FR-LOGIC-006 | **Variable Node**: set/get flow variables; Flow-level constants config ต่อ environment | S |
| FR-LOGIC-007 | Expression language กลาง: อ้าง `$node`, `$flow` (runDate, runId, environment), `$vars`, ฟังก์ชัน string/number/date (format, add days, timezone), พร้อม sandbox ปลอดภัย | M |
| FR-LOGIC-008 | Error handling ต่อ node: On error → Fail flow / Continue / ไป error branch (เส้นสีแดง) — ตั้ง retry ต่อ node (ครั้ง + backoff) | M |

## FR-AI — AI Nodes & AI Assist

| ID | Requirement | Priority |
|---|---|---|
| FR-AI-001 | **AI Node (LLM Task)**: เลือก preset — Summarize / Classify / Extract fields / Generate text / Translate / Custom prompt — input จาก node ก่อนหน้า, output เป็น structured JSON (กำหนด output schema ได้) เข้า pipeline ต่อ | M |
| FR-AI-002 | ผู้ใช้เลือก model จากรายการที่ Admin เปิด (ผ่าน **Azure OpenAI / Azure AI Foundry** — สอดคล้อง compliance ธนาคาร, ไม่เรียก endpoint สาธารณะตรง) — token limit + cost tracking ต่อ flow | M |
| FR-AI-003 | **AI SQL Assist** ใน Query Node: พิมพ์ภาษาไทย/อังกฤษ ("พนักงานที่เข้างานเดือนนี้ group ตามแผนก") → gen SQL ให้ โดยส่ง schema (ชื่อ table/column เท่านั้น **ไม่ส่งข้อมูลจริง**) ให้ LLM — ผู้ใช้ต้อง review ก่อนใช้ | M |
| FR-AI-004 | **AI Flow Assist**: อธิบาย flow ที่มีอยู่เป็นภาษาคน + แนะนำ node ถัดไป; Phase 2: gen ทั้ง flow จากคำอธิบาย | S |
| FR-AI-005 | **AI Data Quality Node**: ตรวจ anomaly ในผลลัพธ์ query (เช่น แถวเป็น 0 ผิดปกติ, ค่า null พุ่ง) แล้วส่ง alert | C |
| FR-AI-006 | Guardrails: ข้อมูลที่ mark ว่า sensitive (ตาม masking rules) **ถูก mask ก่อนส่งเข้า LLM เสมอ**; log ทุก AI call (prompt hash, token, model, ผู้ใช้) เพื่อ audit | M |

## FR-RUN — Execution, Run History & Monitoring

| ID | Requirement | Priority |
|---|---|---|
| FR-RUN-001 | หน้า Run History ต่อ flow: รายการ run (เวลาเริ่ม/จบ, duration, สถานะ Success/Failed/Running/Cancelled/Skipped, trigger type, version ที่ใช้) filter/search ได้ | M |
| FR-RUN-002 | Drill-down ต่อ run: เห็นทุก node step — input/output (mask ตาม RBAC), duration, error message + stack, retry count | M |
| FR-RUN-003 | Re-run: ทั้ง flow หรือ **Resume จาก node ที่ fail** (ใช้ output เดิมของ node ก่อนหน้า) | S |
| FR-RUN-004 | Cancel run ที่กำลังรัน (graceful — node ปัจจุบันจบก่อน) | M |
| FR-RUN-005 | Dashboard รวม: จำนวน run วันนี้, success rate, flow ที่ fail ล่าสุด, run ที่ใช้เวลานานผิดปกติ, upcoming schedule | S |
| FR-RUN-006 | Notification เมื่อ flow fail: Email / Microsoft Teams webhook — config ต่อ flow | M |
| FR-RUN-007 | Execution log retention: step-level detail 30 วัน, summary 1 ปี (config ได้) | S |

## FR-ADMIN — Administration

| ID | Requirement | Priority |
|---|---|---|
| FR-ADMIN-001 | จัดการ Users/Roles/Permissions (ดู `07-security-testing.md`) | M |
| FR-ADMIN-002 | จัดการ DB Connections, MFT Connections, SMTP config, AI model config — ทั้งหมด credential ผ่าน Key Vault | M |
| FR-ADMIN-003 | จัดการ Masking Rules (ผูก column pattern → role ที่เห็นได้) | M |
| FR-ADMIN-004 | Audit log viewer: ทุก action สำคัญ (login, create/publish/delete flow, เปลี่ยน permission, ดาวน์โหลดไฟล์, ดูข้อมูล unmasked) — export ได้, ส่งเข้า SIEM | M |
| FR-ADMIN-005 | Environment banner (DEV/SIT/UAT/PROD) + config แยก per environment | S |
| FR-ADMIN-006 | Usage quota ต่อ flow/ทีม: max runs/วัน, max file size, max AI tokens (กัน cost บาน) | C |
