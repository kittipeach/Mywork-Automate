-- 0007_flow_grants — object-level access grants on a flow (docs/spec/08 E2-S3).
--
-- Each row grants a subject (a role, subject_type='role', or a user,
-- subject_type='user') an access level on one flow. access is viewer|editor|owner
-- (ascending privilege). The UNIQUE(flow_id, subject_type, subject_id) key means a
-- subject has at most one grant per flow — re-granting upserts the access level
-- (see AddGrant). This delivers the grant model + share API; deriving the flow
-- list a subject actually sees FROM these grants (object-level list filtering) is
-- additive and layered on later.
CREATE TABLE IF NOT EXISTS flow_grants (
    id           BIGSERIAL PRIMARY KEY,
    flow_id      TEXT NOT NULL,
    subject_type TEXT NOT NULL,
    subject_id   TEXT NOT NULL,
    access       TEXT NOT NULL,
    UNIQUE (flow_id, subject_type, subject_id)
);

CREATE INDEX IF NOT EXISTS idx_flow_grants_flow ON flow_grants (flow_id);
