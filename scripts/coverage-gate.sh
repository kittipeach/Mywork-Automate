#!/usr/bin/env bash
set -euo pipefail
#
# Per-package coverage gate for MyWork Automate.
#
# NOTE (deviation from 10-claude-code-playbook.md §4): the playbook version set
# FAIL=1 inside a `while` loop on the RHS of a pipe. That loop body runs in a
# subshell, so FAIL never propagated to the parent and the gate could NEVER fail.
# This corrected version feeds the loop via process substitution so the flag and
# the exit status are honoured. Thresholds and intent are unchanged.
#
# ---- Go: per-package thresholds ----
# Critical security packages (masking/sqlguard/expression/templaterender) = 100%
# statement coverage; every other package >= 95% (see the awk block below).
FAIL=0

if [[ ! -f coverage.out ]]; then
  echo "coverage-gate: coverage.out not found — run 'make test' first" >&2
  exit 1
fi

# Excluded from the UNIT-coverage gate (validated by integration/e2e instead):
#   /cmd/            — thin main-package wiring (server start, signal handling)
#   /pgxquerier/     — real-Postgres adapter, covered by the `integration` test
#   /store/postgres/ — real-Postgres control-plane store, `integration`-tested
#   /audit/postgres/ — real-Postgres audit store, `integration`-tested
#   /runner/         — Temporal-client run adapter, exercised at runtime
#   scheduler/temporal.go — Temporal ScheduleClient adapter, exercised at runtime
#   preview/pool.go  — pgx preview/schema adapter, `integration`-tested
# Build a filtered profile that keeps the mode header.
FILTERED=coverage.filtered.out
grep -vE '/cmd/|/pgxquerier/|/store/postgres/|/audit/postgres/|/runner/|/scheduler/temporal\.go|/preview/pool\.go' coverage.out > "$FILTERED"

# Per-PACKAGE statement coverage from the raw profile (CLAUDE.md rule 2:
# "pkg/*, internal/* other ≥ 95%" is a package-level bar; the four critical
# security packages are 100%). Computed by summing covered/total statements per
# package directory rather than per-function, so a well-handled but hard-to-reach
# defensive branch (e.g. a crypto rand.Read failure) doesn't sink an otherwise
# fully-tested package — while the criticals still require every statement.
if ! awk '
  $1 ~ /\.go:/ {
    split($1, a, ":"); file=a[1];
    n=split(file, parts, "/"); dir="";
    for (i=4; i<n; i++) { dir = dir (dir==""?"":"/") parts[i] }   # drop github.com/org/repo + filename
    stmt=$2+0; cnt=$3+0;
    tot[dir]+=stmt; if (cnt>0) cov[dir]+=stmt;
    gtot+=stmt;     if (cnt>0) gcov+=stmt;
  }
  END {
    fail=0;
    for (p in tot) {
      if (tot[p]==0) continue;
      pct = 100*cov[p]/tot[p];
      min = (p ~ /^pkg\/(masking|sqlguard|expression|templaterender)/) ? 100 : 95;
      if (pct < min) { printf "FAIL %s: %.1f%% < %d%%\n", p, pct, min; fail=1 }
    }
    gpct = (gtot>0) ? 100*gcov/gtot : 100;
    printf "Go total coverage: %.1f%%\n", gpct;
    if (gpct < 95) { printf "FAIL total Go coverage %.1f%% < 95%%\n", gpct; fail=1 }
    exit fail;
  }
' "$FILTERED"; then FAIL=1; fi

# ---- Frontend: vitest json summary ----
if [[ -f apps/automate-web/coverage/coverage-summary.json ]]; then
  node scripts/check-fe-coverage.mjs --min 90 || FAIL=1
else
  echo "WARN: apps/automate-web/coverage/coverage-summary.json missing — run FE tests to enforce FE gate" >&2
fi

exit $FAIL
