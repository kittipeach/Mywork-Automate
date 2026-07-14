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
