-- 0004_flow_versions — immutable published-version snapshots (docs/spec/08 E4-S2).
--
-- 0001_init created a minimal flow_versions(id TEXT PK, flow_id, version_no,
-- change_note, UNIQUE(flow_id, version_no)) placeholder. Publish/versioning needs
-- to pin the exact definition that went live plus publish metadata, so this
-- migration brings the table up to the E4-S2 shape by adding the missing columns
-- idempotently (the table may already hold placeholder rows from an earlier env).
--
-- Target shape (E4-S2):
--   flow_versions(
--     id BIGSERIAL PK, flow_id TEXT NOT NULL, version_no INT NOT NULL,
--     definition JSONB NOT NULL, change_note TEXT NOT NULL DEFAULT '',
--     published_by TEXT, published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
--     UNIQUE(flow_id, version_no))
--
-- The id column stays as-is (0001 declared it TEXT PRIMARY KEY); CreateVersion
-- supplies a generated id so the not-null PK is always satisfied. On a fresh DB
-- the CREATE below wins and matches the spec DDL verbatim.

CREATE TABLE IF NOT EXISTS flow_versions (
    id           BIGSERIAL PRIMARY KEY,
    flow_id      TEXT NOT NULL,
    version_no   INT  NOT NULL,
    definition   JSONB NOT NULL,
    change_note  TEXT NOT NULL DEFAULT '',
    published_by TEXT,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (flow_id, version_no)
);

-- Bring a pre-existing (0001) table up to shape. definition is added nullable
-- then backfilled + constrained so the migration succeeds against rows that
-- predate it.
ALTER TABLE flow_versions ADD COLUMN IF NOT EXISTS definition JSONB;
UPDATE flow_versions SET definition = '{}'::jsonb WHERE definition IS NULL;
ALTER TABLE flow_versions ALTER COLUMN definition SET NOT NULL;

ALTER TABLE flow_versions ADD COLUMN IF NOT EXISTS change_note  TEXT NOT NULL DEFAULT '';
ALTER TABLE flow_versions ADD COLUMN IF NOT EXISTS published_by TEXT;
ALTER TABLE flow_versions ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE INDEX IF NOT EXISTS idx_flow_versions_flow ON flow_versions (flow_id, version_no DESC);
