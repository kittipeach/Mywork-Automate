-- 0005_step_io — per-step I/O snapshots (docs/spec/08 E5-S3, FR-RUN-001,002).
--
-- Records a truncated, already-masked sample of each step's input and output
-- items alongside the existing input_count/output_count. Both columns are
-- nullable JSONB: a step may have no sample (nothing captured, or a step that
-- ran before this column existed). IF NOT EXISTS keeps the migration idempotent
-- and safe to re-apply.
ALTER TABLE execution_steps
    ADD COLUMN IF NOT EXISTS input_sample  JSONB,
    ADD COLUMN IF NOT EXISTS output_sample JSONB;
