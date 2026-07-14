package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Seed inserts the exact mock dataset from
// apps/automate-web/src/lib/mock/store.ts (5 flows, 3 executions, 4
// connections) so the real API returns the same rows the UI was built against.
//
// It is idempotent: it only inserts when the flows table is empty, so it is
// safe to call on every startup after Migrate.
//
// The timestamps are the pre-computed outputs of the TS iso() helper
// (base 2026-07-14T00:00:00+07:00 == 2026-07-13T17:00:00Z, then
// -daysAgo*1d +h*1h, rendered as .toISOString()). They are stored as text so
// they round-trip byte-for-byte to the UI.
func Seed(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM flows`).Scan(&count); err != nil {
		return fmt.Errorf("postgres: seed count flows: %w", err)
	}
	if count > 0 {
		return nil // already seeded
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: seed begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on early return

	// Timestamps (see doc comment for derivation).
	const (
		iso1  = "2026-07-12T23:00:00.000Z" // iso(1)
		iso0  = "2026-07-13T23:00:00.000Z" // iso(0)
		iso2  = "2026-07-11T23:00:00.000Z" // iso(2)
		iso5  = "2026-07-08T23:00:00.000Z" // iso(5)
		iso3  = "2026-07-10T23:00:00.000Z" // iso(3)
		iso9  = "2026-07-04T23:00:00.000Z" // iso(9)
		iso8  = "2026-07-05T23:00:00.000Z" // iso(8)
		iso09 = "2026-07-14T02:00:00.000Z" // iso(0, 9)
	)

	folders := []struct{ id, name string }{
		{"fld_finance", "Finance"},
		{"fld_hrops", "HR Ops"},
		{"fld_compliance", "Compliance"},
	}
	for _, f := range folders {
		if _, err := tx.Exec(ctx,
			`INSERT INTO folders (id, name, parent_id) VALUES ($1, $2, NULL)`,
			f.id, f.name); err != nil {
			return fmt.Errorf("postgres: seed folder %s: %w", f.id, err)
		}
	}

	type flowRow struct {
		id, name, folder, status string
		version                  int
		updatedAt                string
		lastRunStatus            *string
		lastRunAt                *string
	}
	sp := func(s string) *string { return &s }
	flowRows := []flowRow{
		{"flw_payroll", "Payroll → Bank MFT", "Finance", "published", 3, iso1, sp("success"), sp(iso0)},
		{"flw_headcount", "Daily Headcount → Email", "HR Ops", "published", 2, iso2, sp("success"), sp(iso0)},
		{"flw_leave", "Leave Balance Report", "HR Ops", "paused", 1, iso5, sp("failed"), sp(iso3)},
		{"flw_newhire", "New Hire Onboarding Export", "HR Ops", "draft", 0, iso0, nil, nil},
		{"flw_gov", "Gov Submission (TIS-620)", "Compliance", "stopped", 4, iso9, sp("cancelled"), sp(iso8)},
	}
	for _, f := range flowRows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO flows (id, name, folder, status, current_version, updated_at, last_run_status, last_run_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			f.id, f.name, f.folder, f.status, f.version, f.updatedAt, f.lastRunStatus, f.lastRunAt); err != nil {
			return fmt.Errorf("postgres: seed flow %s: %w", f.id, err)
		}
	}

	type connRow struct {
		id, name, typ, host, status string
		roles                       []string
	}
	connRows := []connRow{
		{"conn_hr", "HR Postgres (read-only)", "postgres", "hr-db.internal:5432", "ok", []string{"admin", "designer"}},
		{"conn_pay", "Payroll Postgres", "postgres", "pay-db.internal:5432", "ok", []string{"admin"}},
		{"conn_bankmft", "Bank MFT", "sftp", "mft.bank.co.th:22", "ok", []string{"admin", "designer"}},
		{"conn_smtp", "Corp SMTP", "smtp", "smtp.mywork.co:587", "untested", []string{"admin", "designer", "operator"}},
	}
	for _, c := range connRows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO connections (id, name, type, host, status, allowed_roles)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			c.id, c.name, c.typ, c.host, c.status, c.roles); err != nil {
			return fmt.Errorf("postgres: seed connection %s: %w", c.id, err)
		}
	}

	type execRow struct {
		id, flowID, flowName, status, trigger string
		startedAt                             string
		durationMs                            int64
		version                               int
	}
	execRows := []execRow{
		{"exe_1001", "flw_payroll", "Payroll → Bank MFT", "success", "schedule", iso0, 48213, 3},
		{"exe_1002", "flw_leave", "Leave Balance Report", "failed", "schedule", iso3, 9210, 1},
		{"exe_1003", "flw_headcount", "Daily Headcount → Email", "running", "manual", iso09, 0, 2},
	}
	for _, e := range execRows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO executions (id, flow_id, flow_name, status, trigger_type, started_at, duration_ms, version)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			e.id, e.flowID, e.flowName, e.status, e.trigger, e.startedAt, e.durationMs, e.version); err != nil {
			return fmt.Errorf("postgres: seed execution %s: %w", e.id, err)
		}
	}

	type stepRow struct {
		execID                     string
		seq                        int
		nodeID, nodeName, nodeType string
		status                     string
		durationMs                 int64
		inputCount, outputCount    int64
		errMsg                     *string
	}
	stepRows := []stepRow{
		// exe_1001
		{"exe_1001", 0, "t1", "Schedule 06:00", "trigger.schedule", "success", 5, 0, 1, nil},
		{"exe_1001", 1, "q1", "Query Payroll", "db.query", "success", 2103, 1, 45012, nil},
		{"exe_1001", 2, "if1", "Has rows?", "logic.if", "success", 2, 45012, 45012, nil},
		{"exe_1001", 3, "f1", "Gen XLSX", "file.generate", "success", 41880, 45012, 1, nil},
		{"exe_1001", 4, "m1", "MFT to Bank", "delivery.mft", "success", 4120, 1, 1, nil},
		// exe_1002
		{"exe_1002", 0, "t1", "Schedule 07:00", "trigger.schedule", "success", 4, 0, 1, nil},
		{"exe_1002", 1, "q1", "Query Leave", "db.query", "failed", 9200, 1, 0, sp("connection timeout to hr-db")},
		// exe_1003
		{"exe_1003", 0, "t1", "Manual", "trigger.manual", "success", 2, 0, 1, nil},
		{"exe_1003", 1, "q1", "Query Headcount", "db.query", "running", 0, 1, 0, nil},
	}
	for _, s := range stepRows {
		if _, err := tx.Exec(ctx,
			`INSERT INTO execution_steps
			 (execution_id, seq, node_id, node_name, node_type, status, duration_ms, input_count, output_count, error_message)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			s.execID, s.seq, s.nodeID, s.nodeName, s.nodeType, s.status,
			s.durationMs, s.inputCount, s.outputCount, s.errMsg); err != nil {
			return fmt.Errorf("postgres: seed step %s/%s: %w", s.execID, s.nodeID, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: seed commit: %w", err)
	}
	return nil
}
