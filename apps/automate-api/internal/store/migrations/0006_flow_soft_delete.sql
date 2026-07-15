-- 0006_flow_soft_delete — reversible flow deletion (docs/spec/08 E3-S6).
--
-- Adds a soft-delete marker to flows. A NULL deleted_at is a live flow; a
-- non-NULL value is the instant it was deleted. Soft delete leaves status
-- untouched so a restore returns the flow to exactly its prior lifecycle state.
-- The default /flows list excludes rows where deleted_at IS NOT NULL; the
-- admin-only ?includeDeleted=true path includes them. IF NOT EXISTS keeps the
-- migration idempotent and safe to re-apply.
ALTER TABLE flows ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_flows_deleted_at ON flows (deleted_at);
