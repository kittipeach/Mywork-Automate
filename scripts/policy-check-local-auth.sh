#!/usr/bin/env bash
set -euo pipefail
#
# CI policy gate (docs/spec/07 §1.2): AUTH_LOCAL_ENABLED must never be true in a
# protected environment (sit/uat/prod). This is the deploy-time half of the
# defence-in-depth control; internal/config enforces the runtime half.
#
# Scans deploy/charts/**/values-{sit,uat,prod}*.y*ml for a truthy flag.

PROTECTED_REGEX='values-(sit|uat|prod)[^/]*\.ya?ml$'
FAIL=0
found_any=0

while IFS= read -r f; do
  found_any=1
  # match "AUTH_LOCAL_ENABLED: true" / authLocalEnabled: true (any case), ignoring comments
  if grep -Eiv '^\s*#' "$f" | grep -Eiq 'auth_?local_?enabled\s*:\s*("?)true\1'; then
    echo "FAIL: $f sets local auth true in a protected environment"
    FAIL=1
  else
    echo "OK:   $f"
  fi
done < <(find deploy -type f 2>/dev/null | grep -E "$PROTECTED_REGEX" || true)

if [[ "$found_any" -eq 0 ]]; then
  echo "policy-check-local-auth: no protected-env values files found yet (helm chart lands in E1-S3) — nothing to check"
fi

exit $FAIL
