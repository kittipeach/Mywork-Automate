# 01 — Market Research: Workflow Automation Platforms ทั่วโลก

> เป้าหมาย: สำรวจระบบ Automation ชั้นนำทั่วโลก แล้ว map feature เข้ากับ Requirement ของ MyWork Automate เพื่อ (1) ยืนยันว่า requirement ครบตามมาตรฐานตลาด (2) หยิบ pattern ที่พิสูจน์แล้วมาใช้ (3) หา gap ที่เป็น differentiator ของเรา

## 1. Landscape Overview (สำรวจ ก.ค. 2026)

### กลุ่ม A — No-code / Low-code iPaaS (ใกล้เคียง MyWork Automate ที่สุด)

| Platform | จุดเด่น | สถาปัตยกรรม/โมเดล |
|---|---|---|
| **Microsoft Power Automate** | Cloud flows 3 แบบ (Automated / Scheduled / Instant), Connectors มหาศาล, Approvals, AI Builder + Copilot, DLP policy, Solutions สำหรับ ALM/versioning | SaaS, ผูก Microsoft 365, คิดเงิน per user/flow |
| **n8n** | Node-based canvas, 1,000+ integrations, HTTP node ต่อ API อะไรก็ได้, **70+ AI nodes (LangChain native)**, RBAC, Git-based source control + environments, **ดึง secret จาก Azure Key Vault ได้โดยตรง**, queue mode ~220 exec/sec, SOC2 | Self-host ได้ (fair-code), Node.js, Postgres backend |
| **Make (Integromat)** | Visual canvas สวยที่สุด, Router/Iterator/Aggregator สำหรับ branching-loop, error handler ต่อ node, Maia AI assistant, AI Agents | SaaS, คิดเงิน per operation |
| **Zapier** | 8,000+ apps, ง่ายสุดสำหรับ non-tech, AI Copilot สร้าง Zap จาก natural language, Zapier Agents, Tables/Interfaces | SaaS, linear flow (ไม่ใช่ canvas เต็มรูปแบบ), แพงเมื่อ scale |
| **Activepieces** | Open source (MIT), AI-native, ~400 MCP servers, pieces เขียนด้วย TypeScript, UI เป็นมิตรกว่า n8n | Self-host, TypeScript |
| **Automatisch** | Zapier-like open source (AGPL), 250+ integrations | Self-host |

### กลุ่ม B — Developer-first Orchestration (ดี​สำหรับศึกษา engine ภายใน)

| Platform | จุดเด่น | เหมาะศึกษาเรื่อง |
|---|---|---|
| **Temporal.io** | Durable execution — workflow ไม่หายแม้ pod ตาย, retry/timeout/saga เป็น first-class, Go SDK | **Execution engine ของเรา (มีใน MyWork stack อยู่แล้ว)** |
| **Kestra** | Declarative YAML, event-driven triggers, 700–1,200+ plugins, Helm on K8s, scale 100k concurrent tasks | โครงสร้าง flow definition, trigger model |
| **Windmill** | Code-first (รองรับ Go!), แปลง script เป็น UI/API, worker fleet auto-scale, per-step input/output history | Worker architecture, execution log per step |
| **Apache Airflow** | DAG scheduler มาตรฐาน data engineering, backfill, cron expression | Scheduler semantics, cron |
| **Prefect / Dagster** | Data pipeline orchestration, observability ดี | Run states, retry policies |
| **Node-RED** | Flow-based ต้นตำรับ, function node | Canvas UX แบบเบา |
| **Camunda 8** | BPMN 2.0, human task, decision table (DMN) | ถ้าอนาคตต้องมี approval step |
| **Argo Workflows** | K8s-native DAG | ถ้าจะรัน step เป็น container |

### กลุ่ม C — Report / Document Generation (สำหรับ Template Designer ของเรา)

| Product | จุดเด่น |
|---|---|
| **JasperReports / Jaspersoft Studio** | Report template designer (band-based layout), export PDF/XLSX/CSV/TXT — มาตรฐาน enterprise reporting |
| **Carbone.io** | Template จากไฟล์ DOCX/XLSX จริง + placeholder `{d.field}` render เป็น PDF/XLSX — pattern ที่เรียบง่ายมาก |
| **JSReport** | Template engine (handlebars) + chrome-pdf, xlsx recipe, มี studio designer |
| **Crystal Reports / SSRS** | Legacy enterprise report designer — ผู้ใช้ HR/Finance คุ้นเคย pattern นี้ |
| **Excelize (Go lib)** | สร้าง XLSX ใน Go — ตัวเลือกหลักฝั่ง implementation ของเรา |
| **Maroto / gopdf / chromedp** | สร้าง PDF ใน Go |

> **Insight สำคัญ:** ไม่มี automation platform ตัวไหนในกลุ่ม A ที่มี **Template Designer สำหรับวางโครงเอกสาร Excel/PDF ในตัว** (ทุกตัวต้องต่อ 3rd-party เช่น Google Docs, Carbone) — **นี่คือ differentiator ของ MyWork Automate** เพราะ use case หลักของเราคือ "Query DB → ออกรายงานตามฟอร์แมตธนาคาร → ส่ง MFT" ซึ่ง Power Automate ทำได้ยาก/แพง

## 2. Feature Matrix — เทียบ Requirement ของเรา

Legend: ✅ มี native | 🟡 มีบางส่วน/ต้องประกอบเอง | ❌ ไม่มี

| Requirement ของ MyWork Automate | Power Automate | n8n | Make | Kestra | Windmill | **MyWork Automate (target)** |
|---|---|---|---|---|---|---|
| Drag & drop canvas วาด flow | ✅ | ✅ | ✅ | ❌ (YAML) | 🟡 | ✅ React Flow |
| Recurring trigger (รันทุกๆ X นาที/ชม./วัน, cron) | ✅ Scheduled flow | ✅ Schedule node | ✅ | ✅ | ✅ | ✅ |
| Event/Manual/Webhook trigger | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ (Manual, Webhook, Event เฟสถัดไป) |
| Query PostgreSQL: เลือก DB connection, เลือก table, ใส่ condition ผ่าน UI, เขียน SQL เองได้ | 🟡 (premium connector) | ✅ Postgres node | ✅ | ✅ | ✅ | ✅ + Visual query builder |
| Generate file หลาย format | 🟡 (ผ่าน Office connectors) | 🟡 Spreadsheet File node | 🟡 | 🟡 | 🟡 | ✅ Excel / PDF / TXT / CSV |
| **Template Designer วางโครงไฟล์ + export template ได้** | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ **Differentiator** |
| ส่งไฟล์ทาง MFT / SFTP | 🟡 | ✅ SFTP node | ✅ | ✅ | 🟡 | ✅ MFT Node (SFTP + องค์กร MFT) |
| ส่ง Email พร้อม attachment | ✅ | ✅ | ✅ | ✅ | 🟡 | ✅ |
| Download file จากหน้า run history | 🟡 | 🟡 | 🟡 | ✅ outputs | ✅ | ✅ Download Node + signed URL |
| Logic: If/Condition | ✅ | ✅ IF/Switch | ✅ Router | ✅ | ✅ | ✅ If / Switch |
| Loop / For-each | ✅ Apply to each | ✅ Loop Over Items | ✅ Iterator | ✅ | ✅ | ✅ For-Each Node |
| Data transform (map/filter/format) | 🟡 expressions | ✅ Set/Code node | ✅ | ✅ | ✅ | ✅ Transform Node + expression |
| **AI Node ใน flow** (summarize, classify, extract, gen text) | ✅ AI Builder | ✅ 70+ AI nodes | ✅ | 🟡 plugin | 🟡 | ✅ AI Node (Azure OpenAI / Foundry) |
| **AI ช่วยสร้าง flow / gen SQL จากภาษาคน** | ✅ Copilot | ✅ | ✅ Maia | ❌ | ✅ AI gen script | ✅ AI Assist (SQL gen, flow suggest) |
| Versioning ทุกครั้งที่ save/publish + rollback | ✅ Solutions | ✅ workflow history + Git | 🟡 | ✅ (Git) | ✅ (Git-native) | ✅ DB-backed versions + diff |
| สถานะ Draft / Published / Paused / Stopped | ✅ (on/off) | ✅ (active/inactive) | ✅ | ✅ | ✅ | ✅ 4 สถานะ + state machine |
| Run history + per-step input/output log | ✅ | ✅ | ✅ | ✅ | ✅ ดีมาก | ✅ |
| Retry / error handling ต่อ node | ✅ | ✅ | ✅ error handler | ✅ | ✅ | ✅ (Temporal retry policy) |
| RBAC (viewer/editor/admin ระดับ flow/folder) | ✅ (env roles) | ✅ Enterprise | ✅ | ✅ EE | ✅ | ✅ + **data masking ระดับ column** |
| เห็นข้อมูล sensitive ตามสิทธิ์ (column-level masking) | 🟡 DLP | ❌ | ❌ | ❌ | ❌ | ✅ **Differentiator (PDPA/BOT)** |
| SSO / Enterprise IdP | ✅ Entra ID | ✅ SAML/LDAP | ✅ | ✅ | ✅ | ✅ Entra ID OIDC + Local (Dev) |
| Secret management ภายนอก | ✅ | ✅ **Azure Key Vault** | ❌ | ✅ | ✅ | ✅ Azure Key Vault |
| Audit log | ✅ | ✅ Enterprise | ✅ | ✅ | ✅ | ✅ |
| Self-host / data residency SEA | ❌ | ✅ | ❌ | ✅ | ✅ | ✅ AKS Southeast Asia |

## 3. Pattern ที่ควร "ยืม" มาใช้

1. **n8n — โครง node & data model:** ทุก node รับ/ส่งข้อมูลเป็น `items[]` (array of JSON objects) ทำให้ For-each, Transform, File Gen ต่อกันได้อัตโนมัติ → ใช้ contract เดียวกันนี้
2. **n8n — External secrets:** credential เก็บเป็น reference ไป Key Vault ไม่เก็บค่าในระบบ → ตรง requirement เราพอดี
3. **Temporal — Durable execution:** flow ที่รันค้างอยู่ไม่ตายตาม pod, retry/timeout ประกาศเป็น policy → ใช้เป็น execution engine (MyWork มี Temporal ใน stack แล้วจาก saga orchestration)
4. **Make — Router UX:** เส้น branch ที่มี label เงื่อนไขบนเส้น อ่าน flow ง่าย → ใช้กับ If/Switch node
5. **Windmill — Per-step I/O history:** เก็บ input/output ทุก step ของทุก run (พร้อม masking) → ช่วย debug intermittent failure
6. **Power Automate — Flow checker:** validate flow ก่อน publish (node ลอย, ไม่มี trigger, credential หาย) → ทำเป็น pre-publish validation
7. **Carbone/JasperReports — Template pattern:** template = layout + placeholder binding กับ dataset → ใช้ออกแบบ Template Designer
8. **Zapier/Make — AI Copilot:** สร้าง flow จากภาษาธรรมชาติ → เฟส 2 ของ AI Assist

## 4. สิ่งที่ตลาด "ไม่มี" = จุดขายของ MyWork Automate

1. **Template Designer ในตัว** — วางโครง Excel/PDF/TXT (fixed-width สำหรับส่งแบงก์ชาติ/cross-bank ได้) พร้อม export/import template
2. **Column-level data masking ตาม RBAC** — เหมาะกับข้อมูล HR/Payroll (เงินเดือน, เลขบัตรประชาชน) ตาม PDPA และ compliance ธนาคาร
3. **MFT Node ระดับองค์กร** — ต่อ MFT ที่ธนาคารใช้จริง ไม่ใช่แค่ SFTP ทั่วไป
4. **ฝังใน MyWork ecosystem** — ใช้ user/role/tenant เดียวกับ MyWork ไม่ต้องซื้อ license per-user แบบ Power Automate
