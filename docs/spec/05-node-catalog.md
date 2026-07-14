# 05 — Node Catalog Specification

> ทุก node มี: `type` (unique id), `category`, `config` (JSON Schema → auto-render form), `inputs/outputs` (ports), `retryable` — Data contract ระหว่าง node: **`items[]`** (array of JSON objects) + `files[]` (FileRef) + `meta`

## หมวด 1: Triggers (จุดเริ่ม flow — มีได้ 1 ตัวต่อ flow ใน MVP)

### 1.1 `trigger.schedule` — Recurring Trigger ⭐MVP
| Config | Type | รายละเอียด |
|---|---|---|
| mode | enum | `simple` / `cron` |
| simple.every | number+unit | ทุก N `minutes/hours/days/weeks/months` |
| simple.at | time | เวลารัน (สำหรับ days ขึ้นไป), เลือกวันในสัปดาห์/วันที่ในเดือน |
| cron.expression | string | validate + แปลเป็นภาษาคน + preview next 5 runs |
| timezone | string | default `Asia/Bangkok` |
| overlapPolicy | enum | `skip` (default) / `queue` / `parallel` |
| effectiveFrom/To | date | ช่วงวันที่ schedule ทำงาน (optional) |

Output items: `[{ scheduledTime, actualTime, runNumber }]` — Implement: Temporal Schedule

### 1.2 `trigger.manual` ⭐MVP — ปุ่ม Run now + define input parameters (name, type, required) → ฟอร์มตอนกดรัน
### 1.3 `trigger.webhook` (Phase 1.5) — gen URL `/hooks/{token}`, HMAC secret, method whitelist, payload → items
### 1.4 `trigger.event` (Phase 2) — subscribe MyWork event bus (employee.created, payroll.closed, ...)

## หมวด 2: Data (Query)

### 2.1 `db.query` — PostgreSQL Query ⭐MVP
| Config | Type | รายละเอียด |
|---|---|---|
| connectionId | ref | dropdown connection ที่มีสิทธิ์ (RBAC-filtered) |
| mode | enum | `builder` / `sql` |
| builder.schema/table | picker | โหลด metadata จาก information_schema (cache 10 นาที) |
| builder.columns | multi-select | คอลัมน์ที่เลือก (default all) |
| builder.where | condition[] | `{field, operator, value|expression}` ซ้อน AND/OR group ได้ |
| builder.orderBy / limit | — | — |
| sql.query | text (editor) | **SELECT-only** — validate ด้วย SQL parser ฝั่ง backend (pg_query_go), บล็อก multi-statement, DDL/DML, `;--` |
| parameters | map | ชื่อ param → expression; bind แบบ `$1,$2` เสมอ |
| timeoutSec | int | default 120, max 900 |
| maxRows | int | default 100,000 |

Output: `items[]` = rows; `meta`: rowCount, columns[{name,type}] | Features: **Preview 50 แถว (masked)**, **AI SQL Assist** (ส่งเฉพาะ schema ไม่ส่งข้อมูล), Test connection
Error: connection fail (retryable), timeout, maxRows exceeded (non-retryable + คำแนะนำ)

### 2.2 `data.transform` ⭐MVP — operations เป็น pipeline: select/rename fields, filter (เงื่อนไขแบบ If), sort, aggregate (groupBy + sum/count/avg/min/max), computed field (expression), merge/join (กับ output อีก node: inner/left, key mapping)
### 2.3 `data.variable` — set/get flow variables

## หมวด 3: File Generation

### 3.1 `file.generate` ⭐MVP
| Config | Type | รายละเอียด |
|---|---|---|
| format | enum | `xlsx` / `pdf` / `txt` / `csv` |
| filename | expression | เช่น `payroll_{{$flow.runDate\|format "YYYYMMDD"}}.xlsx` |
| layoutMode | enum | `auto` (ตาราง header อัตโนมัติ) / `template` |
| templateId + templateVersion | ref | pin version ตอน publish |
| onEmpty | enum | `skip` (ไม่สร้างไฟล์) / `generateEmpty` / `fail` |
| split.maxRowsPerFile | int | optional — เกินแล้วแตก `_partN` |
| sensitive | bool | บังคับ encryption path |
| retentionDays | int | default 30 |

Output: `files[]` = FileRef{fileId, name, size, checksum, storagePath}; `items` ส่งผ่านต่อ (passthrough)

**Template Model (เก็บใน `templates` + export เป็น `.mwt` JSON):**
```jsonc
{
  "type": "xlsx",
  "sheets": [{
    "name": "Payroll",
    "header": { "rows": [["บริษัท ทีทีบี", "", ""], ["งวด: {{$flow.params.period}}"]], "style": {...} },
    "columnsMap": [
      { "source": "emp_code",  "title": "รหัสพนักงาน", "width": 14, "align": "left" },
      { "source": "salary",    "title": "เงินเดือน", "width": 12, "numFmt": "#,##0.00" }
    ],
    "summary": [{ "target": "salary", "fn": "SUM", "label": "รวม" }],
    "freeze": "A2"
  }]
}
// pdf: { page:{size,orientation,margin}, header:{logoFileId,title,subtitle}, table:{columnsMap,fontThai:"THSarabun"}, footer:{pageNumber,text} }
// txt:  { mode:"fixed|delimited", encoding:"UTF-8|TIS-620", delimiter:"|",
//         fields:[{source,width,pad:"left|right",padChar:" ",format}],
//         headerRecord:"H{{count}}...", trailerRecord:"T{{sum salary}}" }
```
Implementation: XLSX = excelize StreamWriter | PDF = maroto v2 + ฝังฟอนต์ TH Sarabun | TXT = custom encoder + golang.org/x/text (TIS-620)

## หมวด 4: Delivery

### 4.1 `delivery.mft` ⭐MVP
Config: connectionId (SFTP/MFT — host, port, auth: password|ssh-key จาก Key Vault, hostKey verification), remotePath (expression), renamePattern, createDirs, transferMode: `atomic` (.tmp→rename) , verifyChecksum, retry{count:3, backoff:exponential}
Idempotency: เช็คไฟล์ปลายทาง (ชื่อ+checksum) ก่อน retry — มีแล้วถือว่าสำเร็จ

### 4.2 `delivery.email` ⭐MVP
Config: transport (SMTP org / MS Graph), to/cc/bcc (static list + expression จาก data), subject/body (expression + HTML template + i18n), attach: files จาก node ก่อนหน้า (เลือกได้), maxAttachmentMB (default 20 — เกิน → แนบเป็น signed download link), sendMode: `single` (สรุป 1 ฉบับ) / `perItem` (mail merge ต่อแถว)

### 4.3 `delivery.download` ⭐MVP
Config: linkExpiryDays (default 7), allowedRoles (จำกัดเพิ่มจาก flow RBAC), notifyUsers (แจ้งว่าไฟล์พร้อม)
Behavior: ไฟล์โผล่ในหน้า Run History + หน้า "My Files"; ดาวน์โหลดผ่าน signed URL; audit ทุก download

### 4.4 `delivery.http` (Phase 1.5) — REST call: method/url/headers/body, auth จาก KV, response → items
### 4.5 `delivery.blob` (Phase 2) — อัปโหลดตรงเข้า Blob container ภายนอก

## หมวด 5: Logic

### 5.1 `logic.if` ⭐MVP — conditions[] {left(expression), operator(=,≠,>,<,≥,≤,contains,startsWith,endsWith,isEmpty,isNotEmpty,in), right} + combinator AND/OR (ซ้อน group ได้) → outputs: `true` / `false`
### 5.2 `logic.switch` — เลือก field → cases[] + default → N outputs
### 5.3 `logic.foreach` ⭐MVP — source items, batchSize (1=ทีละแถว), maxIterations, parallelism(1-10), body = sub-graph; error policy: stopOnError / collectErrors
### 5.4 `logic.delay` — duration (sec–hours) — Temporal durable timer
### 5.5 `logic.merge` — รวม branch (append / wait-all)

## หมวด 6: AI

### 6.1 `ai.task` ⭐MVP
| Config | รายละเอียด |
|---|---|
| preset | `summarize` / `classify` (กำหนด labels) / `extract` (กำหนด output fields) / `generateText` / `translate` / `custom` |
| model | จากรายการที่ admin เปิด (Azure OpenAI / Foundry deployment) |
| inputMapping | เลือก field จาก items ที่จะส่งเข้า prompt |
| outputSchema | JSON schema ของผลลัพธ์ → บังคับ structured output → เข้า items ต่อ |
| batchSize / maxTokens / temperature | ควบคุม cost |

Guardrail (บังคับ, ปิดไม่ได้): field ที่ติด masking rule ถูก mask ก่อนส่งเข้า model; log ทุก call (model, tokens, cost est., user, flow) ; per-flow token quota

### 6.2 AI Assist (ไม่ใช่ node — ฟีเจอร์ UI)
- **SQL Assist** ใน db.query: NL→SQL (ส่งเฉพาะ table/column names), แสดง SQL ให้ review + explain
- **Flow Explain**: สรุป flow เป็นภาษาไทย/อังกฤษ (ใช้ทำเอกสาร/handover)
- **Next-node Suggest**: แนะนำ node ถัดไปตาม pattern (Phase 1.5)
- **NL→Flow**: gen ทั้ง flow จากคำอธิบาย (Phase 2)

### 6.3 `ai.anomaly` (Phase 2) — ตรวจความผิดปกติของ dataset (0 แถวผิดคาด, ค่า null พุ่ง, ยอด SUM เพี้ยนจาก baseline) → แจ้งเตือน/branch

## หมวด 7: Notification / Utility
- `notify.teams` (Phase 1.5) — ส่งการ์ดเข้า Teams webhook
- `util.archive` (Phase 2) — zip files (+password)
- `util.script` (Phase 3, ถกก่อน) — sandboxed expression script; **ไม่ทำ arbitrary code MVP** (ความเสี่ยง security ในธนาคาร)

## Roadmap สรุปต่อเฟส (align กับ `09-phase-plan.md`)
| เฟส | Nodes / Features |
|---|---|
| **Phase 1 — MVP** | trigger.schedule, trigger.manual, db.query (builder+SQL+preview), data.transform (select/rename/filter/sort), file.generate (**auto layout XLSX/CSV/TXT-delimited + Excel template พื้นฐาน**), delivery.mft, delivery.email, delivery.download, logic.if, error handling per node |
| **Phase 2** | Template Designer เต็ม (Excel styles/summary, **PDF+ฟอนต์ไทย**, **TXT fixed-width+TIS-620**), template versioning/export, ai.task, AI SQL Assist, Flow Explain, logic.foreach, data.transform เต็ม (aggregate/join), parallel delivery |
| **Phase 3** | trigger.webhook, logic.switch, logic.delay, logic.merge, delivery.http, notify.teams, Next-node suggest |
| **Phase 4** | trigger.event, delivery.blob, ai.anomaly, util.archive, NL→Flow, DOCX/JSON/XML formats, approval node |
