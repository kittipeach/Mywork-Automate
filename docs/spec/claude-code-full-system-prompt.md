# Claude Code CLI — Full System Build (MyWork Automate)

Based on `10-claude-code-playbook.md`. Two parts: (A) work-dir setup commands, (B) the master prompt to paste into Claude Code.

---

## A. Work Directory Setup (run in Terminal first)

```bash
# 1) Create the repo working directory
mkdir -p ~/dev/mywork-automate && cd ~/dev/mywork-automate
git init -b main

# 2) Copy all spec files into docs/spec/
mkdir -p docs/spec
cp "/Users/peach/Documents/Claude/Projects/Mywork Automate"/*.md docs/spec/

# 3) Launch Claude Code from the repo root
claude
```

Optional — Azure AI Foundry mode:

```bash
export CLAUDE_CODE_USE_FOUNDRY=1
# plus your org's Foundry endpoint vars, then run: claude
```

---

## B. Master Prompt — paste into Claude Code (builds the ENTIRE system)

```text
You are the Tech Lead orchestrator for the MyWork Automate project — a banking-grade,
node-based workflow automation engine. Your mission: build the ENTIRE Phase 1 (MVP)
system end-to-end, following docs/spec/10-claude-code-playbook.md exactly.

═══ PHASE 0 — BOOTSTRAP (do this first, sequentially) ═══
1. Read docs/spec/10-claude-code-playbook.md in full. It contains embedded file
   contents you must materialize into the repo:
   - Extract the CLAUDE.md block (§1) → write to ./CLAUDE.md
   - Extract all 5 subagent definitions (§2) → write to .claude/agents/backend-go.md,
     frontend-next.md, qa-automation.md, devops-azure.md, spec-reviewer.md
   - Extract the Makefile, scripts/coverage-gate.sh, tests/perf/*.js,
     tests/e2e/rbac-matrix.spec.ts, and CI snippet (§4) → write to matching paths
2. Read docs/spec/09-phase-plan.md — we build Phase 1 (MVP) ONLY.
3. Read docs/spec/08-backlog-epics-stories-tasks.md — list every story assigned to P1.
4. Read docs/spec/04-architecture.md, 05-node-catalog.md, 06-data-model-api.md,
   07-security-testing.md for implementation reference.
5. Summarize your understanding: stack, layout, P1 story list, dependency order.

═══ PHASE 1 — SCAFFOLD (story E1-S1) ═══
Create the monorepo skeleton per CLAUDE.md layout:
  apps/automate-api (Go 1.22+, Gin) | apps/automate-worker (Go, Temporal)
  apps/automate-web (Next.js 14 App Router, @xyflow/react, @mywork/ui,
  TanStack Query, Zustand) | pkg/ (expression, masking, sqlguard, filestore,
  secrets, templaterender) | deploy/charts/automate | tests/e2e | tests/perf
Include Makefile, docker-compose.dev.yml (postgres, temporal-dev, azurite,
sftp-mock, mailhog), scripts/coverage-gate.sh, lint configs, CI pipeline.
TDD from the very first file. Finish with `make lint && make test` green.

═══ PHASE 2 — PARALLEL BUILD (all remaining P1 stories) ═══
Work through the P1 backlog in dependency order. For each batch of independent
stories, use the Task tool to spawn subagents IN PARALLEL with this mapping:
  - Go backend / node executors / Temporal workflows → backend-go
  - Canvas, config panels, forms, pages, UI state     → frontend-next
  - E2E, integration harness, k6, RBAC matrix,
    injection/security suites                          → qa-automation
  - Helm, CI/CD, Key Vault, policy checks, dev stack   → devops-azure
Assign each subagent an explicit, non-overlapping file scope. A subagent must
NEVER touch files outside its scope — if it needs to, it stops and reports.

Non-negotiable rules for every subagent (from CLAUDE.md):
  - TDD only: write failing tests first → implement → pass → refactor
  - Coverage gates: pkg/masking, pkg/sqlguard, pkg/expression,
    pkg/templaterender = 100% statement; other Go ≥ 95%; FE components/hooks ≥ 90%
  - SQL parameterized only; SQL mode via pkg/sqlguard (SELECT-only) with
    injection test corpus (comment tricks, multi-statement, CTE-hidden DML, unicode)
  - Secrets via pkg/secrets only; every API handler behind RBAC middleware with
    per-role deny-by-default tests; external-DB responses through masking
    interceptor with proving tests
  - Temporal workflow code deterministic (workflow.Now/SideEffect, never
    time.Now/rand directly)
  - Conventional commits prefixed with story id, e.g. feat(E6-S3): ...
  - End every task with `make lint && make test && make coverage-gate` all green

═══ PHASE 3 — REVIEW LOOP (mandatory per story) ═══
The moment a subagent finishes a story, dispatch spec-reviewer with the story id.
It verifies: each Acceptance Criterion implemented (AC ↔ test mapping, 1 AC ≥ 1
E2E test), real coverage numbers, security rules 3–6 of CLAUDE.md, and hunts for
fake tests (meaningless assertions, missing negative cases, lingering t.Skip).
If FAIL → send back to the owning subagent to fix → re-review. Loop until PASS.
No story is "done" without a reviewer PASS.

═══ PHASE 4 — SYSTEM VERIFICATION ═══
1. Run: make lint && make test && make test-int && make coverage-gate
2. Run gitleaks detect; verify policy check AUTH_LOCAL_ENABLED=false for
   sit/uat/prod values; helm lint + helm template | kubeconform on all charts
3. Verify RBAC matrix E2E covers every endpoint × role cell from
   docs/spec/07-security-testing.md §2.1
4. Verify 100% of P1 Acceptance Criteria have at least one E2E test

═══ FINAL REPORT (required format) ═══
Table: story | status | coverage per package (real numbers) | # tests added |
key files changed | open issues needing a human decision.
Do NOT declare completion until every P1 story passed the reviewer and all
gates are green.
```

---

## C. Optional — Parallel Worktree Sessions (multiple terminals)

For running several stories concurrently in separate Claude Code sessions:

```bash
# From the repo root — one worktree per story
git worktree add ../automate-e2s1 -b feature/E2-S1-entra-sso
git worktree add ../automate-e3s1 -b feature/E3-S1-canvas-core
git worktree add ../automate-e6s3 -b feature/E6-S3-sql-guard

# Open a terminal per worktree, cd into it, run `claude`, then paste a
# story-scoped prompt, e.g.:
```

```text
Read CLAUDE.md, docs/spec/08 (E6-S3), docs/spec/02 (FR-DB-004, FR-DB-005),
docs/spec/07 §4. Build pkg/sqlguard: parser guard using pg_query_go — allow
single-statement SELECT only. TDD: start with a test corpus of ≥ 40 cases —
valid SELECTs (read-only CTE, JOIN, subquery, window) and invalid ones
(INSERT/UPDATE/DELETE/DDL, multi-statement, `;--` comment tricks, CTE hiding
DML, EXPLAIN ANALYZE with side effects, SET/COPY, mutating functions).
Target: 100% statement coverage of pkg/sqlguard.
Finish with `make test && make coverage-gate` green.
```

---

## D. Optional — Headless One-Shot (non-interactive CI style)

```bash
cd ~/dev/mywork-automate
claude -p "$(cat claude-prompt.txt)" --dangerously-skip-permissions
# where claude-prompt.txt contains the Master Prompt from section B
```
