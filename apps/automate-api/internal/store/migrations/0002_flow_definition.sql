-- 0002_flow_definition — add the editable flow-definition graph to flows.
-- The definition is the nodes[]/edges[] FlowDef (docs/spec/06 §1) the worker's
-- interpreter executes. It is nullable: draft flows may exist before a runnable
-- graph is authored, and GET /flows/{id}/run 404s when it is NULL.
ALTER TABLE flows ADD COLUMN IF NOT EXISTS definition JSONB NULL;
