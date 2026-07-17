#!/usr/bin/env bash
#
# migrate.sh — apply the automate-api database migrations to a target Postgres,
# standalone (without starting the API). Use this to pre-provision or migrate a
# database on another machine, in CI, or by a DBA.
#
# It is byte-for-byte compatible with the Go startup migrator
# (apps/automate-api/internal/store/postgres/migrate.go): it applies each
# apps/automate-api/internal/store/migrations/*.sql exactly once, in numbered
# order, recording the filename in the schema_migrations table. Running this and
# then starting the API (which also migrates) is safe — each side skips what the
# other already applied. Re-running is a no-op.
#
# IMPORTANT: migration 0004 recreates flow_versions (DROP+CREATE), so it is NOT
# idempotent on its own. The schema_migrations bookkeeping here is what makes the
# whole set safe to re-run — do not apply the .sql files by hand without it.
#
# Usage:
#   scripts/migrate.sh "postgres://user:pass@host:5432/dbname?sslmode=disable"
#   DATABASE_URL="postgres://..." scripts/migrate.sh
#
# Requires: psql (PostgreSQL client) on PATH.
set -euo pipefail

DB="${1:-${DATABASE_URL:-}}"
if [[ -z "$DB" ]]; then
  echo "migrate.sh: no database URL given" >&2
  echo "usage: scripts/migrate.sh <postgres-url>   (or set DATABASE_URL)" >&2
  exit 2
fi

if ! command -v psql >/dev/null 2>&1; then
  echo "migrate.sh: psql not found on PATH (install the postgresql client)" >&2
  exit 3
fi

MIG_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/apps/automate-api/internal/store/migrations"
if [[ ! -d "$MIG_DIR" ]]; then
  echo "migrate.sh: migrations dir not found: $MIG_DIR" >&2
  exit 4
fi

echo "migrate.sh: target = $(echo "$DB" | sed -E 's#(://[^:/@]+):[^@/]*@#\1:****@#')"

# Mirror the Go migrator's tracking table.
psql "$DB" -v ON_ERROR_STOP=1 -q -c \
  "CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now());"

applied=0 skipped=0
shopt -s nullglob
for f in "$MIG_DIR"/*.sql; do
  version="$(basename "$f")"
  already="$(psql "$DB" -tAc "SELECT 1 FROM schema_migrations WHERE version = '$version'")"
  if [[ "$already" == "1" ]]; then
    printf '  skip  %s (already applied)\n' "$version"
    skipped=$((skipped + 1))
    continue
  fi
  printf '  apply %s\n' "$version"
  # Apply the migration and record it atomically: --single-transaction wraps the
  # -f file and the tracking INSERT so a failed migration rolls back and is not
  # recorded.
  psql "$DB" -v ON_ERROR_STOP=1 -q --single-transaction \
    -f "$f" \
    -c "INSERT INTO schema_migrations (version) VALUES ('$version');"
  applied=$((applied + 1))
done

echo "migrate.sh: done — applied $applied, skipped $skipped. Schema is up to date."
