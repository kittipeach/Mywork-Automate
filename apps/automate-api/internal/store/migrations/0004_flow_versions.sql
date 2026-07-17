-- 0004_flow_versions — immutable published-version snapshots (docs/spec/08 E4-S2).
--
-- 0001_init created a minimal flow_versions(id TEXT PK, …) placeholder that was
-- never populated (versioning is new in E4-S2). Its TEXT primary key has no
-- default, so an INSERT that relies on a generated id fails the NOT NULL PK
-- constraint. Since the table holds no real data, drop and recreate it with the
-- correct BIGSERIAL id — safe, and gives fresh DBs and re-migrations the same
-- verbatim spec DDL.

DROP TABLE IF EXISTS flow_versions;

CREATE TABLE flow_versions (
    id           BIGSERIAL PRIMARY KEY,
    flow_id      TEXT NOT NULL,
    version_no   INT  NOT NULL,
    definition   JSONB NOT NULL,
    change_note  TEXT NOT NULL DEFAULT '',
    published_by TEXT,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (flow_id, version_no)
);

CREATE INDEX IF NOT EXISTS idx_flow_versions_flow ON flow_versions (flow_id, version_no DESC);
