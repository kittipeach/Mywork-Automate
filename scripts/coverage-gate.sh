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
# Critical security packages require 100% statement coverage; everything else
# 95%. Implemented as a case function (not an associative array) so the gate
# runs on bash 3.2 (macOS) as well as bash 5 (CI).
DEFAULT_MIN=95
FAIL=0

min_for_pkg() {
  case "$1" in
    pkg/masking* | pkg/sqlguard* | pkg/expression* | pkg/templaterender*) echo 100 ;;
    *) echo "$DEFAULT_MIN" ;;
  esac
}

if [[ ! -f coverage.out ]]; then
  echo "coverage-gate: coverage.out not found — run 'make test' first" >&2
  exit 1
fi

# Excluded from the UNIT-coverage gate (validated by integration/e2e instead):
#   /cmd/            — thin main-package wiring (server start, signal handling)
#   /pgxquerier/     — real-Postgres adapter, covered by the `integration` test
#   /store/postgres/ — real-Postgres control-plane store, `integration`-tested
# Build a filtered profile that keeps the mode header.
FILTERED=coverage.filtered.out
grep -vE '/cmd/|/pgxquerier/|/store/postgres/' coverage.out > "$FILTERED"

while read -r file pct; do
  [[ -z "$file" ]] && continue
  # strip module prefix: github.com/<org>/<repo>/pkg/masking/foo.go -> pkg/masking
  pkg=$(dirname "$file" | sed 's|^[^/]*/[^/]*/[^/]*/||')
  min=$(min_for_pkg "$pkg")
  if awk -v p="$pct" -v m="$min" 'BEGIN{exit !(p<m)}'; then
    echo "FAIL $file: ${pct}% < ${min}%"
    FAIL=1
  fi
done < <(go tool cover -func="$FILTERED" | grep -v "^total" | awk '{print $1, $3}' | sed 's/%//')

TOTAL=$(go tool cover -func="$FILTERED" | awk '/^total/{sub("%","",$3);print $3}')
if awk -v t="$TOTAL" 'BEGIN{exit !(t<95)}'; then
  echo "FAIL total Go coverage ${TOTAL}% < 95%"
  FAIL=1
fi
echo "Go total coverage: ${TOTAL}%"

# ---- Frontend: vitest json summary ----
if [[ -f apps/automate-web/coverage/coverage-summary.json ]]; then
  node scripts/check-fe-coverage.mjs --min 90 || FAIL=1
else
  echo "WARN: apps/automate-web/coverage/coverage-summary.json missing — run FE tests to enforce FE gate" >&2
fi

exit $FAIL
