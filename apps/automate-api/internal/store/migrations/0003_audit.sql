-- 0003_audit — append-only audit trail served by automate-api (docs/spec/07 §6,
-- docs/spec/06 §1 "Audit"). Every security-relevant action (auth.*, flow.*,
-- connection.*, execution.*, file.download, masking.*, rbac.grant_change, ai.call)
-- is recorded here with the actor, source ip and user agent. Rows are never
-- UPDATEd or DELETEd by the app — the audit DB role is granted INSERT/SELECT only.
-- Retention is ≥ 1 year online (enforced operationally / by partition rotation in
-- the full schema; this control-plane subset keeps the flat table).
--
-- Types are TEXT (not UUID/INET) to stay consistent with the TEXT-id read model
-- used throughout automate-api (flw_*, conn_*, user subjects from JWT claims).
CREATE TABLE IF NOT EXISTS audit_logs (
    id            BIGSERIAL   PRIMARY KEY,
    user_id       TEXT        NULL,
    role          TEXT        NOT NULL DEFAULT '',
    action        TEXT        NOT NULL,
    resource_type TEXT        NOT NULL DEFAULT '',
    resource_id   TEXT        NOT NULL DEFAULT '',
    detail        JSONB       NULL,
    ip            TEXT        NOT NULL DEFAULT '',
    user_agent    TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Primary read path is "recent events for a given action" and "recent events".
CREATE INDEX IF NOT EXISTS idx_audit_action_created ON audit_logs (action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_created        ON audit_logs (created_at DESC);
