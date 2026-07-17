-- 0001_init — control-plane read model served by automate-api (docs/spec/06 §1).
-- Focused subset of the full schema: the tables the REST read endpoints serve.
-- Text ids are used to stay consistent with the seed (flw_payroll, exe_1001,
-- conn_hr, ...). Timestamps that must round-trip byte-for-byte to the UI are
-- stored as text so the exact ISO strings from the mock are preserved.

CREATE TABLE IF NOT EXISTS folders (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    parent_id TEXT NULL REFERENCES folders(id)
);

CREATE TABLE IF NOT EXISTS flows (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    folder          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft',
    current_version INT  NOT NULL DEFAULT 0,
    updated_at      TEXT NOT NULL,
    last_run_status TEXT NULL,
    last_run_at     TEXT NULL
);

CREATE TABLE IF NOT EXISTS flow_versions (
    id          TEXT PRIMARY KEY,
    flow_id     TEXT NOT NULL REFERENCES flows(id),
    version_no  INT  NOT NULL,
    change_note TEXT NOT NULL DEFAULT '',
    UNIQUE (flow_id, version_no)
);

CREATE TABLE IF NOT EXISTS connections (
    id            TEXT PRIMARY KEY,
    name          TEXT   NOT NULL,
    type          TEXT   NOT NULL,
    host          TEXT   NOT NULL,
    status        TEXT   NOT NULL,
    allowed_roles TEXT[] NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS executions (
    id           TEXT PRIMARY KEY,
    flow_id      TEXT NOT NULL,
    flow_name    TEXT NOT NULL,
    status       TEXT NOT NULL,
    trigger_type TEXT NOT NULL,
    started_at   TEXT NOT NULL,
    duration_ms  BIGINT NOT NULL DEFAULT 0,
    version      INT  NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS execution_steps (
    id            BIGSERIAL PRIMARY KEY,
    execution_id  TEXT NOT NULL REFERENCES executions(id),
    seq           INT  NOT NULL,
    node_id       TEXT NOT NULL,
    node_name     TEXT NOT NULL,
    node_type     TEXT NOT NULL,
    status        TEXT NOT NULL,
    duration_ms   BIGINT NOT NULL DEFAULT 0,
    input_count   BIGINT NOT NULL DEFAULT 0,
    output_count  BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_executions_flow ON executions (flow_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_exec_steps_exec ON execution_steps (execution_id, seq);
