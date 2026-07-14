# 07 — Security, RBAC & Testing Strategy

## 1. Authentication

### 1.1 Production — Microsoft Entra ID
- OIDC Authorization Code + PKCE ผ่าน Kong/Next.js → API validate JWT (JWKS, issuer/audience pinning, clock skew ≤ 60s)
- Group→Role sync: AD security groups (เช่น `SG-MyWork-Automate-Admin`) map เข้า role ตอน login + sync job รายวัน
- Token: access 1 ชม., refresh ตาม Entra policy; logout = revoke session ฝั่งแอป
- Conditional Access / MFA เป็นไปตามนโยบาย Entra ขององค์กร (ไม่ implement เอง)

### 1.2 Development — Local User Login
- เปิดด้วย `AUTH_LOCAL_ENABLED=true` **เท่านั้น** — Helm values ของ SIT/UAT/Prod fix เป็น `false` + **CI policy check** (fail pipeline ถ้า values prod มี flag true) + runtime guard (ถ้า env=production และ flag=true → แอปไม่ start)
- Password: argon2id, นโยบาย ≥ 12 ตัว, lockout 5 ครั้ง/15 นาที, audit ทุก attempt
- JWT ออกโดย issuer แยก (`mywork-automate-local`) อายุสั้น 8 ชม. — API แยกแยะ provider ได้ใน claims
- Seed script สร้าง local admin สำหรับ dev (`make seed-dev`)

## 2. RBAC Model

### 2.1 System Roles (baseline — custom role ได้ใน Phase 2)

| Permission | Admin | Flow Designer | Operator | Viewer |
|---|---|---|---|---|
| จัดการ users/roles/masking/connections | ✅ | ❌ | ❌ | ❌ |
| สร้าง/แก้ flow (draft) | ✅ | ✅ | ❌ | ❌ |
| Publish / Rollback | ✅ | ✅ (flow ที่เป็น owner/editor) | ❌ | ❌ |
| Pause / Resume / Stop / Cancel run | ✅ | ✅ | ✅ | ❌ |
| Manual run | ✅ | ✅ | ✅ | ❌ |
| ดู run history + log | ✅ | ✅ | ✅ | ✅ |
| ดาวน์โหลดไฟล์ผลลัพธ์ | ✅ | ✅ | ✅ | ตาม flow grant |
| สร้าง/แก้ template | ✅ | ✅ | ❌ | ❌ |
| ใช้ AI Assist | ✅ | ✅ | ✅ | ❌ |
| เห็นข้อมูล sensitive (unmasked) | ตาม masking exempt | ตาม exempt | ตาม exempt | ❌ เสมอ |

- **Resource-level:** `flow_grants` — viewer/editor/owner ต่อ flow หรือ folder (inherit); connection มี `allowed_role_ids`
- Deny by default: ไม่มี grant = มองไม่เห็น flow นั้นใน list

### 2.2 Column-level Data Masking (จุดต่างจากตลาด)
- `masking_rules`: pattern จับชื่อคอลัมน์ (เช่น `(?i)(salary|citizen|bank_account|phone)`) หรือผูก connection+column เจาะจง
- Mask styles: `full` (`******`), `partial_last4` (`***-**-6789`), `hash`
- บังคับใช้ที่ 4 จุด (server-side เสมอ ไม่ใช่ CSS ซ่อน):
  1. **Query Preview** — ตาม role ผู้กด preview
  2. **Execution step I/O log** — snapshot ที่เก็บลง DB ถูก mask ก่อนเขียน (unmask ไม่ได้ย้อนหลัง = ปลอดภัยสุด)
  3. **AI Node / AI Assist** — mask ก่อนส่งเข้า LLM ทุกกรณี (ปิดไม่ได้)
  4. **ไฟล์ผลลัพธ์** — *ไม่ mask* (จุดประสงค์คือออกรายงานจริง) แต่คุมด้วยสิทธิ์ download + sensitive flag + audit
- การดู unmasked (role ที่ exempt) ถูก audit เป็น action `masking.bypass_view`

## 3. Secrets & Infra Security
- Azure Key Vault + AKS Workload Identity (federated) — ไม่มี credential ตกค้างใน cluster/etcd
- Key Vault: RBAC mode, soft-delete + purge protection, private endpoint, access log → SIEM
- Blob: private container เท่านั้น, SAS อายุสั้นเฉพาะ download URL, SSE (+ CMK ถ้านโยบายกำหนด)
- Target DB: private endpoint/VNet, **read-only user** เป็นมาตรฐานสำหรับ connection ที่ใช้ query
- Network policy: default-deny egress ที่ worker, whitelist ปลายทาง (MFT hosts, Graph, Azure OpenAI, KV, Blob)
- Container: distroless/base minimal, non-root, read-only rootfs, no privilege escalation

## 4. Application Security Controls (OWASP mapping)

| ความเสี่ยง | Control |
|---|---|
| SQL Injection | parameterized เท่านั้น + SQL parser whitelist (SELECT-only, single statement) ฝั่ง backend |
| SSRF (HTTP node/webhook) | URL allowlist ต่อ environment, บล็อก private IP ranges, no redirect follow ข้าม host |
| XSS | React escaping + CSP strict; expression ไม่ render เป็น HTML |
| Broken Access Control | RBAC middleware ทุก endpoint + object-level check (flow grant) + integration tests ต่อ role |
| Path traversal (MFT/file) | sanitize remotePath, filename; no `..` |
| Expression sandbox escape | engine ไม่มี IO, timeout 100ms, fuzz test |
| Insecure deserialization | JSON schema validation ทุก config ก่อนเก็บ/รัน |
| Secrets leak in logs | log scrubber (pattern ของ secret refs/tokens) + masked I/O snapshots |

## 5. Quality Gates & Testing Pipeline

### 5.1 CI (ทุก PR)
```
lint (golangci-lint / eslint) → unit tests (Go ≥ 80% coverage engine & masking; FE vitest)
→ SonarQube quality gate (0 blocker/critical, coverage gate) → gitleaks (secret scan)
→ build images → Trivy scan (fs + image): fail on HIGH/CRITICAL fixable → SBOM (syft) เก็บเป็น artifact
```
### 5.2 CD (Dev → SIT → UAT → Prod)
- Dev: auto deploy + smoke e2e (Playwright: login, สร้าง flow, run, ดู history)
- SIT: full regression + **perf test (k6)** ตาม NFR-PERF (scenario: 100 concurrent runs, 100k-row file gen, scheduler spike) — report เทียบ baseline, fail ถ้า regress > 20%
- UAT: usability test กับ HR ops ตาม NFR-UX-002
- Prod gate: change approval + Trivy re-scan ของ image ที่จะ deploy (CVE ใหม่หลัง build)

### 5.3 Pen Test
- ก่อน go-live: external pen test (web + API + AKS config review) ตามมาตรฐานธนาคาร/BOT — scope รวม: authz bypass (IDOR บน flow/execution/file), SQL mode escape, expression sandbox, webhook HMAC, local-login ปิดจริงบน prod, masking bypass
- ปิด finding: Critical/High = block go-live; Medium = แผนแก้ ≤ 30 วัน
- ประจำปี + หลัง major release; DAST (OWASP ZAP baseline) รันใน SIT ทุก sprint

### 5.4 Test Matrix ต่อชั้น
| ชั้น | เครื่องมือ | ครอบคลุม |
|---|---|---|
| Unit | Go test + testify, vitest | executors, expression engine, masking, template render (golden files: XLSX/PDF/TXT เทียบ checksum/snapshot) |
| Integration | testcontainers (Postgres, SFTP server, Azurite, Temporal dev server) | db.query จริง, MFT atomic rename, blob store, schedule fire |
| E2E | Playwright | user journeys ต่อ role (RBAC matrix test อัตโนมัติ) |
| Perf | k6 | ตาม NFR-PERF-001..009 + soak 8 ชม. |
| Security | SonarQube, Trivy, gitleaks, ZAP, external pen test | ตามข้อ 5.1–5.3 |
| Chaos (Phase 2) | kill worker pod ระหว่าง run | พิสูจน์ durable execution (NFR-AVAIL-002) |

## 6. Audit Requirements (สรุป event ที่ต้องเก็บ)
`auth.login/logout/failed`, `flow.create/update/publish/pause/resume/stop/delete/rollback/import/export`, `template.*`, `connection.create/update/delete/test`, `execution.manual_run/cancel/retry`, `file.download`, `masking.rule_change`, `masking.bypass_view`, `rbac.grant_change`, `ai.call`
— append-only, มี ip + user agent, stream เข้า ELK/SIEM, retention ≥ 1 ปี online
