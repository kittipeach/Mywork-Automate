# 10 — Claude Code Playbook: Multi-Agent Parallel Development

> วิธีใช้ Claude Code ทำงาน **หลาย task หลาย agent พร้อมกัน** สำหรับ MyWork Automate — coverage ใกล้ 100% + test scripts automate ครบทุกชั้น
> รองรับทั้ง Anthropic endpoint และ **Azure AI Foundry mode** (env: `CLAUDE_CODE_USE_FOUNDRY=1` + Foundry endpoint vars ตามที่องค์กร config)

## 0. โครงการทำงานแบบขนาน (Parallel Model)

Claude Code รันขนานได้ 2 ระดับ — ใช้ร่วมกัน:

1. **Subagents ใน session เดียว** — orchestrator สั่ง Task tool แตกงานให้ subagent เฉพาะทาง (BE/FE/QA/DevOps) ทำพร้อมกัน เหมาะกับงานใน scope เดียวกัน
2. **หลาย session ด้วย git worktree** — 1 story = 1 worktree = 1 Claude Code session แยก branch กัน ไม่ชนไฟล์ เหมาะกับหลาย story พร้อมกัน (เช่น 4 devs หรือ 1 คนเปิด 4 terminal)

```bash
# ตั้ง worktree ต่อ story (รันจาก repo root)
git worktree add ../automate-e2s1 -b feature/E2-S1-entra-sso
git worktree add ../automate-e3s1 -b feature/E3-S1-canvas-core
git worktree add ../automate-e6s3 -b feature/E6-S3-sql-guard
# เปิด Claude Code ในแต่ละโฟลเดอร์ → รันพร้อมกันได้ทันที
```

---

## 1. `CLAUDE.md` — วางที่ root ของ repo (ตัว agent ทุกตัวอ่านก่อนทำงาน)

```markdown
# MyWork Automate — Project Memory

## What this is
Node-based workflow automation engine (spec ทั้งหมดอยู่ที่ docs/spec/*.md — อ่าน 09-phase-plan.md ก่อนเสมอ, งานทั้งหมดคือ Phase 1 MVP เว้นแต่สั่งเป็นอย่างอื่น)

## Stack & Layout
- apps/automate-api    — Go 1.22+ (Gin), REST control plane
- apps/automate-worker — Go, Temporal worker (node executors)
- apps/automate-web    — Next.js 14 App Router, @xyflow/react, @mywork/ui, TanStack Query, Zustand
- pkg/                 — shared Go packages (expression, masking, sqlguard, filestore, secrets)
- deploy/charts/automate — Helm
- tests/e2e            — Playwright | tests/perf — k6

## Commands (ห้ามเดา — ใช้ตามนี้)
- make dev            # docker-compose: postgres, temporal-dev, azurite, sftp-mock, mailhog
- make test           # unit ทั้งหมด + coverage report
- make test-int       # integration (testcontainers)
- make test-e2e       # Playwright
- make coverage-gate  # fail ถ้าต่ำกว่า threshold
- make lint           # golangci-lint + eslint + tsc --noEmit

## Non-negotiable rules
1. **TDD เท่านั้น**: เขียน test ก่อน → เห็น fail → implement → เห็น pass → refactor ห้ามเขียน implementation ก่อน test
2. **Coverage gates** (บังคับใน CI, ดู scripts/coverage-gate.sh):
   - pkg/masking, pkg/sqlguard, pkg/expression, pkg/template-render = **100%** (statement)
   - pkg/*, internal/* อื่นๆ ≥ **95%** | apps/automate-web components/hooks ≥ **90%**
3. SQL: parameterized เท่านั้น; SQL mode ผ่าน pkg/sqlguard (SELECT-only) — ทุก query path ต้องมี injection test
4. Secrets: ผ่าน pkg/secrets (Key Vault resolver) เท่านั้น — ห้าม hardcode/env ตรง; gitleaks รันทุก commit
5. ทุก API handler ต้องผ่าน RBAC middleware + มี test ต่อ role (deny-by-default test บังคับ)
6. Response ที่มี data จาก DB ภายนอกต้องผ่าน masking interceptor — มี test พิสูจน์
7. โครง test: Go = table-driven + testify; ตั้งชื่อ Test<Func>_<Case>; ไฟล์ *_test.go คู่ทุกไฟล์ .go
8. ห้ามแตะไฟล์นอก scope ของ task ตัวเอง (กัน conflict ระหว่าง agent) — ถ้าจำเป็นให้หยุดแล้วรายงาน
9. Commit: conventional commits, prefix ด้วย story id เช่น `feat(E6-S3): add SELECT-only parser guard`
10. เมื่อจบ task: รัน make lint && make test && make coverage-gate ต้องเขียวทั้งหมดก่อนสรุปงาน

## Definition of Done (ทุก task)
- [ ] Tests เขียนก่อนและครอบ: happy path, edge cases, error paths, security cases
- [ ] Coverage ผ่าน gate | Lint ผ่าน | ไม่มี TODO ที่ไม่มี ticket
- [ ] Integration test ถ้าแตะ DB/Temporal/SFTP/Blob (ใช้ testcontainers)
- [ ] อัปเดต docs ถ้า API/schema เปลี่ยน
```

---

## 2. Subagent Definitions — วางที่ `.claude/agents/*.md`

### `.claude/agents/backend-go.md`
```markdown
---
name: backend-go
description: Go backend specialist for automate-api and automate-worker. Use for any Go implementation task — handlers, services, node executors, Temporal workflows.
tools: Read, Edit, Write, Bash, Grep, Glob
---
You are a senior Go engineer on MyWork Automate (banking-grade).
Workflow ทุก task: (1) อ่าน spec ที่เกี่ยวข้องใน docs/spec (2) เขียน table-driven tests ก่อน ครอบ happy/edge/error/security (3) รัน test เห็น fail (4) implement ให้ pass ด้วยโค้ดที่เรียบง่ายที่สุด (5) refactor (6) `make test && make coverage-gate` ต้องผ่าน
Special rules: Temporal workflow code ต้อง deterministic (ห้าม time.Now/rand ตรงๆ — ใช้ workflow.Now/SideEffect); ทุก executor implement NodeExecutor interface; ทุก error ห่อด้วย context (fmt.Errorf %w)
Coverage: อย่า chase ตัวเลขด้วย test ปลอม — test ต้อง assert พฤติกรรมจริง ถ้าถึง 100% ไม่ได้เพราะโค้ด unreachable ให้ refactor โค้ดแทน
```

### `.claude/agents/frontend-next.md`
```markdown
---
name: frontend-next
description: Next.js/React specialist for automate-web. Use for canvas, config panels, forms, pages, UI state.
tools: Read, Edit, Write, Bash, Grep, Glob
---
You are a senior frontend engineer. Stack: Next.js 14 App Router, @xyflow/react, @mywork/ui, TanStack Query, Zustand, react-hook-form+zod, vitest + React Testing Library.
Workflow: vitest tests ก่อน (component behavior + hooks), MSW mock ทุก API, implement, `pnpm test --coverage` ≥ 90% ของไฟล์ที่แตะ
Rules: ห้าม fetch ตรงใน component (ผ่าน TanStack Query hooks ใน src/api); ทุก form validate ด้วย zod schema ที่ generate จาก node JSON Schema; ทุก text ผ่าน i18n (th/en); accessibility: interactive elements มี aria labels
```

### `.claude/agents/qa-automation.md`
```markdown
---
name: qa-automation
description: Test automation specialist. Use for E2E Playwright suites, integration test harnesses, k6 perf scripts, RBAC matrix tests, injection/security test suites.
tools: Read, Edit, Write, Bash, Grep, Glob
---
You are a QA automation engineer. Own: tests/e2e (Playwright), tests/perf (k6), integration harness (testcontainers), security test suites.
Deliver per story: (1) E2E จาก Acceptance Criteria ทุกข้อของ story (AC 1 ข้อ = อย่างน้อย 1 test) (2) negative tests (3) RBAC: ทุก endpoint × ทุก role จาก 07-security-testing.md matrix — deny case ต้องมีเสมอ (4) สำหรับ sqlguard/expression: fuzz + injection corpus (comment tricks, multi-statement, CTE-DML, unicode)
Playwright: ใช้ fixtures ต่อ role (admin/designer/operator/viewer), ห้าม sleep — ใช้ web-first assertions, test แยกอิสระ (ไม่ depend ลำดับ)
k6: scenario ตาม NFR-PERF-001..009, thresholds ใส่ในสคริปต์ (fail อัตโนมัติ), export summary JSON เข้า CI artifact
```

### `.claude/agents/devops-azure.md`
```markdown
---
name: devops-azure
description: DevOps specialist. Use for Helm charts, AKS, CI/CD pipelines, Key Vault/Workload Identity, Trivy/SonarQube gates, docker-compose dev stack.
tools: Read, Edit, Write, Bash, Grep, Glob
---
You own deploy/, .github|azure-pipelines, scripts/, docker-compose.
Rules: ทุก chart ผ่าน `helm lint` + `helm template | kubeconform`; ทุก pipeline change ต้อง validate ด้วย dry-run; **policy check: AUTH_LOCAL_ENABLED ต้องเป็น false ใน values ของ sit/uat/prod — เขียน CI step ที่ fail ถ้าเจอ true**; containers: non-root, read-only rootfs, resource limits เสมอ; ทุก script มี bats test หรือ shellcheck ผ่าน
```

### `.claude/agents/spec-reviewer.md`
```markdown
---
name: spec-reviewer
description: Read-only reviewer. Use PROACTIVELY after each story completes — verifies code matches spec, coverage gates, security rules. MUST be used before marking any story done.
tools: Read, Bash, Grep, Glob
---
You are a strict reviewer (no write access). Given a story id: (1) อ่าน AC จาก docs/spec/08 + FR ที่อ้างถึง (2) ตรวจว่า implementation ตรง AC ทีละข้อ (3) รัน make lint, make test, make coverage-gate แล้วรายงานตัวเลขจริง (4) ตรวจ security rules ข้อ 3–6 ใน CLAUDE.md (5) หา test ปลอม (assert ที่ไม่มีความหมาย, ไม่มี negative case) — รายงานเป็น checklist PASS/FAIL พร้อมไฟล์:บรรทัด ห้ามแก้โค้ดเอง
```

---

## 3. Master Orchestrator Prompt (copy ไปวางใน Claude Code)

ใช้เมื่อจะรัน **หลาย task ใน session เดียว** — Claude Code จะแตก subagent ขนานให้เอง:

```text
คุณคือ Tech Lead orchestrator ของโปรเจกต์ MyWork Automate

Context:
- อ่าน CLAUDE.md และ docs/spec/09-phase-plan.md ก่อน — เราทำ Phase 1 (MVP) เท่านั้น
- Backlog อยู่ที่ docs/spec/08-backlog-epics-stories-tasks.md (Phase Assignment table บอกว่า story ไหนอยู่ P1)

งานรอบนี้ — ทำ 4 stories ต่อไปนี้แบบขนาน:
1. E6-S3: SQL mode + SELECT-only guard (pkg/sqlguard) → มอบ backend-go
2. E2-S4: Masking engine + default rules (pkg/masking) → มอบ backend-go (อีก instance)
3. E3-S2: Node config panel (JSON Schema-driven form) → มอบ frontend-next
4. Test harness: RBAC matrix E2E + injection corpus สำหรับ 2 ข้อแรก → มอบ qa-automation

วิธีทำงาน:
- แตกงานด้วย Task tool ให้ subagent ตาม mapping ข้างบน รันขนานกัน
- แต่ละ subagent: TDD เท่านั้น, จบด้วย make test + make coverage-gate เขียว, ห้ามแตะไฟล์นอก scope ตัวเอง
- Scope ไฟล์: (1)=pkg/sqlguard/** (2)=pkg/masking/** (3)=apps/automate-web/src/features/node-config/** (4)=tests/**
- เมื่อ subagent ไหนเสร็จ → ส่ง spec-reviewer ตรวจทันที (story id + AC) — ถ้า FAIL ให้ส่งกลับไปแก้ วนจนกว่า PASS
- ห้ามสรุปว่า "เสร็จ" จนกว่า: ทุก story ผ่าน reviewer + รายงาน coverage ตัวเลขจริงต่อ package + ลิสต์ test files ที่เพิ่ม

รายงานสุดท้ายที่ต้องการ: ตาราง story | สถานะ | coverage | จำนวน tests | ไฟล์หลักที่แก้ | ประเด็นที่ต้องให้คนตัดสินใจ
```

### Prompt ย่อยรายพวก (ใช้กับ worktree แยก session — 1 prompt = 1 terminal)

```text
# Terminal 1 — Story E6-S3 (SQL Guard)
อ่าน CLAUDE.md, docs/spec/08 (E6-S3), docs/spec/02 (FR-DB-004, FR-DB-005), docs/spec/07 §4
สร้าง pkg/sqlguard: parser guard ด้วย pg_query_go — อนุญาตเฉพาะ single-statement SELECT
TDD: เริ่มจาก test corpus ≥ 40 cases: valid SELECTs (CTE read-only, JOIN, subquery, window), invalid (INSERT/UPDATE/DELETE/DDL, multi-statement, `;--` comment tricks, CTE ที่ซ่อน DML, EXPLAIN ANALYZE ที่มี side effect, SET/COPY, function ที่ mutate)
เป้า coverage: 100% statement ของ pkg/sqlguard | จบด้วย make test && make coverage-gate
```

```text
# Terminal 2 — Story E2-S4 (Masking Engine)
อ่าน CLAUDE.md, docs/spec/08 (E2-S4), docs/spec/07 §2.2, docs/spec/06 (masking_rules)
สร้าง pkg/masking: rule engine (column_name/regex match), styles full|partial_last4|hash, exempt roles, applies_to (preview|log|ai)
TDD ก่อน: default rule set (salary, citizen_id, bank_account, phone, email — ไทย+อังกฤษ variants), nested JSON items, ตัวเลข/สตริง/null, performance test 100k items < 1s, exempt role เห็นจริง + non-exempt เห็น mask, idempotency (mask ซ้ำไม่พัง)
เป้า: 100% statement | ห้าม mask แล้ว recover ได้ (ยกเว้น style hash เทียบเท่านั้น)
```

```text
# Terminal 3 — Story E3-S2 (Config Panel)
อ่าน CLAUDE.md, docs/spec/08 (E3-S2), docs/spec/05 (schema ต่อ node), docs/spec/06 (GET /nodes)
สร้าง features/node-config: SchemaForm component แปลง JSON Schema → RHF+zod form (string/number/enum/array/nested/conditional "dependentSchemas")
Vitest ก่อน: render ถูกต่อ schema ทุก type, validation error แสดง, conditional field โผล่/หายตามค่า, expression field เปิด editor, MSW mock GET /nodes
เป้า coverage ≥ 90% ของ feature นี้ | Storybook stories ต่อ field type
```

---

## 4. Test Automation Scripts (วางใน repo ได้ทันที)

### `Makefile`
```makefile
.PHONY: dev test test-int test-e2e test-all coverage-gate lint perf

dev:
	docker compose -f docker-compose.dev.yml up -d

test:            ## unit + coverage
	go test ./... -race -coverprofile=coverage.out -covermode=atomic
	cd apps/automate-web && pnpm vitest run --coverage

test-int:        ## integration (testcontainers — ต้องมี docker)
	go test ./... -tags=integration -race -p 4 -timeout 20m

test-e2e:
	cd tests/e2e && pnpm playwright test

test-all: test test-int test-e2e coverage-gate

coverage-gate:
	bash scripts/coverage-gate.sh

lint:
	golangci-lint run ./...
	cd apps/automate-web && pnpm eslint . && pnpm tsc --noEmit

perf:
	k6 run tests/perf/scenario-concurrent-runs.js
	k6 run tests/perf/scenario-filegen-100k.js
	k6 run tests/perf/scenario-scheduler-spike.js
```

### `scripts/coverage-gate.sh`
```bash
#!/usr/bin/env bash
set -euo pipefail
# ---- Go: per-package thresholds ----
declare -A CRITICAL=( [pkg/masking]=100 [pkg/sqlguard]=100 [pkg/expression]=100 [pkg/templaterender]=100 )
DEFAULT_MIN=95
FAIL=0
go tool cover -func=coverage.out | grep -v "^total" | awk '{print $1,$3}' | sed 's/%//' | \
while read -r file pct; do
  pkg=$(dirname "$file" | sed 's|^github.com/[^/]*/[^/]*/||')
  min=$DEFAULT_MIN
  for c in "${!CRITICAL[@]}"; do [[ "$pkg" == "$c"* ]] && min=${CRITICAL[$c]}; done
  awk -v p="$pct" -v m="$min" 'BEGIN{exit !(p<m)}' && { echo "FAIL $file: ${pct}% < ${min}%"; FAIL=1; }
done
TOTAL=$(go tool cover -func=coverage.out | awk '/^total/{sub("%","",$3);print $3}')
awk -v t="$TOTAL" 'BEGIN{exit !(t<95)}' && { echo "FAIL total Go coverage ${TOTAL}% < 95%"; FAIL=1; }
echo "Go total coverage: ${TOTAL}%"
# ---- Frontend: vitest json summary ----
node scripts/check-fe-coverage.mjs --min 90   # อ่าน coverage/coverage-summary.json
exit $FAIL
```

### `tests/perf/scenario-concurrent-runs.js` (k6 — NFR-PERF-004/009)
```javascript
import http from 'k6/http';
import { check } from 'k6';
export const options = {
  scenarios: { concurrent_runs: { executor: 'constant-vus', vus: 100, duration: '10m' } },
  thresholds: {
    http_req_duration: ['p(95)<500', 'p(99)<1000'],   // NFR-PERF-002
    http_req_failed:  ['rate<0.005'],                  // success ≥ 99.5%
  },
};
export default function () {
  const res = http.post(`${__ENV.BASE_URL}/api/automate/v1/flows/${__ENV.FLOW_ID}/run`,
    JSON.stringify({ params: {} }),
    { headers: { Authorization: `Bearer ${__ENV.TOKEN}`, 'Content-Type': 'application/json' } });
  check(res, { 'accepted': (r) => r.status === 202 });
}
```

### `tests/e2e/rbac-matrix.spec.ts` (โครง — qa-automation agent เติมเต็ม)
```typescript
import { test, expect } from './fixtures/roles';   // fixtures login ต่อ role
const matrix = [
  { role: 'viewer',   action: 'publishFlow',  allowed: false },
  { role: 'viewer',   action: 'viewHistory',  allowed: true  },
  { role: 'operator', action: 'manualRun',    allowed: true  },
  { role: 'operator', action: 'editFlow',     allowed: false },
  { role: 'designer', action: 'publishFlow',  allowed: true  },
  // ... generate ครบจาก 07-security-testing.md §2.1 (agent ต้อง cover ทุก cell)
] as const;
for (const { role, action, allowed } of matrix)
  test(`RBAC: ${role} → ${action} = ${allowed ? 'allow' : 'deny'}`, async ({ pageAs }) => {
    const page = await pageAs(role);
    await expect(actions[action](page)).resolves.toBe(allowed);
  });
```

### CI snippet (เพิ่มเข้า pipeline E1-S4)
```yaml
- run: make lint
- run: make test
- run: make coverage-gate            # <- fail build ถ้า coverage ตก
- run: make test-int
- run: gitleaks detect --no-banner
- run: trivy fs --exit-code 1 --severity HIGH,CRITICAL .
- run: sonar-scanner                 # quality gate ผูกกับ coverage.out + lcov
- run: bash scripts/policy-check-local-auth.sh   # AUTH_LOCAL_ENABLED=false บน sit/uat/prod
```

---

## 5. นโยบาย "Coverage ใกล้ 100% แบบไม่หลอกตัวเอง"

| ชั้น | เป้า | วิธีบังคับ |
|---|---|---|
| Core security packages (masking, sqlguard, expression, template render) | **100%** statement | coverage-gate per-package + fuzz tests |
| Go ที่เหลือ (services, executors, handlers) | ≥ 95% | coverage-gate default + reviewer หา test ปลอม |
| Frontend components/hooks | ≥ 90% | vitest coverage gate |
| Acceptance Criteria | **100% ของ AC มี E2E** | qa-automation: AC 1 ข้อ ≥ 1 Playwright test, reviewer ตรวจ mapping |
| RBAC endpoints | 100% ของ endpoint × role matrix | rbac-matrix.spec.ts generate จากตาราง |
| กันเทสปลอม | — | spec-reviewer ตรวจ: ทุก test ต้องมี meaningful assertion + negative case; ห้าม `t.Skip` ค้าง; mutation spot-check (go-mutesting) รายเดือนบน core packages |

**ข้อเตือนจากประสบการณ์จริง:** สั่ง agent "ทำ coverage 100%" เฉยๆ จะได้ test ที่ execute โค้ดแต่ไม่ assert อะไร — จึงต้องมี (1) TDD บังคับใน CLAUDE.md (2) spec-reviewer agent แยกตรวจ (3) mutation testing spot-check เป็นตาข่ายชั้นสุดท้าย

## 6. Cheat Sheet — ลำดับใช้งานจริงวันแรก

```bash
# 1) วาง docs/spec/*.md (ไฟล์ชุดนี้), CLAUDE.md, .claude/agents/*.md ลง repo
# 2) เปิด Claude Code → prompt แรก:
"อ่าน CLAUDE.md และ docs/spec ทั้งหมด สรุปความเข้าใจ + สร้าง scaffold ตาม E1-S1
(monorepo apps/pkg/deploy/tests + Makefile + docker-compose.dev + coverage-gate script)
TDD ตั้งแต่ไฟล์แรก จบด้วย make test เขียว"
# 3) พอ scaffold เสร็จ → ใช้ Master Orchestrator Prompt (§3) แตกงานขนาน
# 4) ทุก story จบ → spec-reviewer ตรวจ → commit → PR (CI รัน gate ซ้ำทั้งหมด)
```
