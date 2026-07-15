//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/flowspec"
)

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://automate:automate@localhost:5432/automate?sslmode=disable"
}

// freshPool connects, drops the control-plane tables so each run starts clean,
// then migrates + seeds. It returns a Store over the pool.
func freshPool(t *testing.T, ctx context.Context) (*pgxpool.Pool, *Store) {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// Reset schema so migrate/seed are exercised deterministically.
	for _, tbl := range []string{"execution_steps", "executions", "connections", "flow_versions", "flows", "folders", "schema_migrations"} {
		if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS "+tbl+" CASCADE"); err != nil {
			t.Fatalf("drop %s: %v", tbl, err)
		}
	}
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Migrate is idempotent — running twice must be a no-op.
	if err := Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
	if err := Seed(ctx, pool); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Seed is idempotent — second call must not duplicate rows.
	if err := Seed(ctx, pool); err != nil {
		t.Fatalf("seed (2nd): %v", err)
	}
	return pool, New(pool)
}

func TestIntegration_ListFlows(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	all, err := s.ListFlows(ctx, store.FlowFilter{})
	if err != nil {
		t.Fatalf("list flows: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("flows = %d, want 5", len(all))
	}

	// filter by folder + status
	hr, err := s.ListFlows(ctx, store.FlowFilter{Folder: "HR Ops"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hr) != 3 {
		t.Fatalf("HR Ops flows = %d, want 3", len(hr))
	}

	pub, err := s.ListFlows(ctx, store.FlowFilter{Status: "published"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) != 2 {
		t.Fatalf("published flows = %d, want 2", len(pub))
	}

	// substring q, case-insensitive
	pay, err := s.ListFlows(ctx, store.FlowFilter{Q: "PAYROLL"})
	if err != nil {
		t.Fatal(err)
	}
	if len(pay) != 1 || pay[0].ID != "flw_payroll" {
		t.Fatalf("q=PAYROLL = %+v, want flw_payroll", pay)
	}
}

func TestIntegration_GetFlow(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	f, err := s.GetFlow(ctx, "flw_payroll")
	if err != nil {
		t.Fatalf("get flow: %v", err)
	}
	if f.Name != "Payroll → Bank MFT" || f.Version != 3 {
		t.Fatalf("flow = %+v", f)
	}
	if f.LastRun == nil || f.LastRun.Status != "success" || f.LastRun.At != "2026-07-13T23:00:00.000Z" {
		t.Fatalf("lastRun = %+v", f.LastRun)
	}
	if f.UpdatedAt != "2026-07-12T23:00:00.000Z" {
		t.Fatalf("updatedAt = %q", f.UpdatedAt)
	}

	// draft flow has no lastRun
	draft, err := s.GetFlow(ctx, "flw_newhire")
	if err != nil {
		t.Fatal(err)
	}
	if draft.LastRun != nil {
		t.Fatalf("draft lastRun should be nil, got %+v", draft.LastRun)
	}

	if _, err := s.GetFlow(ctx, "nope"); !store.IsNotFound(err) {
		t.Fatalf("missing flow err = %v, want NotFound", err)
	}
}

func TestIntegration_ListExecutions(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	all, err := s.ListExecutions(ctx, store.ExecutionFilter{})
	if err != nil {
		t.Fatalf("list executions: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("executions = %d, want 3", len(all))
	}

	failed, err := s.ListExecutions(ctx, store.ExecutionFilter{Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ID != "exe_1002" {
		t.Fatalf("failed execs = %+v", failed)
	}

	byFlow, err := s.ListExecutions(ctx, store.ExecutionFilter{FlowID: "flw_payroll"})
	if err != nil {
		t.Fatal(err)
	}
	if len(byFlow) != 1 || byFlow[0].ID != "exe_1001" {
		t.Fatalf("by flow = %+v", byFlow)
	}
}

func TestIntegration_GetExecutionWithSteps(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	e, err := s.GetExecution(ctx, "exe_1001")
	if err != nil {
		t.Fatalf("get execution: %v", err)
	}
	if len(e.Steps) != 5 {
		t.Fatalf("steps = %d, want 5", len(e.Steps))
	}
	if e.Steps[0].NodeID != "t1" || e.Steps[4].NodeID != "m1" {
		t.Fatalf("step order wrong: %+v", e.Steps)
	}
	if e.Trigger != "schedule" || e.DurationMs != 48213 {
		t.Fatalf("exec = %+v", e)
	}

	// failed execution carries an error on the failing step
	failed, err := s.GetExecution(ctx, "exe_1002")
	if err != nil {
		t.Fatal(err)
	}
	if len(failed.Steps) != 2 {
		t.Fatalf("failed steps = %d, want 2", len(failed.Steps))
	}
	if failed.Steps[1].Error == nil || *failed.Steps[1].Error != "connection timeout to hr-db" {
		t.Fatalf("error step = %+v", failed.Steps[1])
	}
	// success step has no error
	if e.Steps[0].Error != nil {
		t.Fatalf("success step should have nil error: %+v", e.Steps[0])
	}

	if _, err := s.GetExecution(ctx, "nope"); !store.IsNotFound(err) {
		t.Fatalf("missing execution err = %v, want NotFound", err)
	}
}

func TestIntegration_ListConnectionsRBAC(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	admin, err := s.ListConnections(ctx, []string{"admin"})
	if err != nil {
		t.Fatalf("list connections: %v", err)
	}
	if len(admin) != 4 {
		t.Fatalf("admin connections = %d, want 4", len(admin))
	}

	designer, err := s.ListConnections(ctx, []string{"designer"})
	if err != nil {
		t.Fatal(err)
	}
	// designer excludes conn_pay (admin-only)
	if len(designer) != 3 {
		t.Fatalf("designer connections = %d, want 3", len(designer))
	}
	for _, c := range designer {
		if c.ID == "conn_pay" {
			t.Fatalf("designer must not see conn_pay")
		}
	}

	operator, err := s.ListConnections(ctx, []string{"operator"})
	if err != nil {
		t.Fatal(err)
	}
	if len(operator) != 1 || operator[0].ID != "conn_smtp" {
		t.Fatalf("operator connections = %+v, want [conn_smtp]", operator)
	}

	// unknown role sees none of the RBAC-scoped rows
	none, err := s.ListConnections(ctx, []string{"auditor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("auditor connections = %d, want 0", len(none))
	}
}

// TestIntegration_SetFlowStatus verifies the pause/resume/stop status write and
// the NotFound path.
func TestIntegration_SetFlowStatus(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	if err := s.SetFlowStatus(ctx, "flw_payroll", "paused"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	f, err := s.GetFlow(ctx, "flw_payroll")
	if err != nil {
		t.Fatal(err)
	}
	if f.Status != "paused" {
		t.Fatalf("status = %q, want paused", f.Status)
	}

	if err := s.SetFlowStatus(ctx, "nope", "paused"); !store.IsNotFound(err) {
		t.Fatalf("missing flow err = %v, want NotFound", err)
	}
}

// TestIntegration_Versioning exercises CreateVersion → ListVersions (newest
// first) → GetVersionDefinition round-trip against the real flow_versions table
// (migration 0004).
func TestIntegration_Versioning(t *testing.T) {
	ctx := context.Background()
	pool, s := freshPool(t, ctx)
	defer pool.Close()

	def1 := flowspec.FlowDef{Nodes: []flowspec.NodeDef{{ID: "a", Type: "trigger.manual", Name: "Start"}}}
	def2 := flowspec.FlowDef{Nodes: []flowspec.NodeDef{{ID: "b", Type: "db.query", Name: "Query"}}}

	if err := s.CreateVersion(ctx, "flw_newhire", 1, def1, "first cut", "admin"); err != nil {
		t.Fatalf("create version 1: %v", err)
	}
	if err := s.CreateVersion(ctx, "flw_newhire", 2, def2, "second cut", "designer"); err != nil {
		t.Fatalf("create version 2: %v", err)
	}

	// duplicate version_no violates the UNIQUE(flow_id, version_no) constraint.
	if err := s.CreateVersion(ctx, "flw_newhire", 2, def2, "dup", "admin"); err == nil {
		t.Fatal("duplicate version_no should error")
	}

	versions, err := s.ListVersions(ctx, "flw_newhire")
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions = %d, want 2", len(versions))
	}
	// newest first
	if versions[0].VersionNo != 2 || versions[1].VersionNo != 1 {
		t.Fatalf("version order = %+v, want [2,1]", versions)
	}
	if versions[0].ChangeNote != "second cut" || versions[0].PublishedBy != "designer" {
		t.Fatalf("version[0] = %+v", versions[0])
	}
	if versions[0].PublishedAt == "" {
		t.Fatalf("publishedAt should be set: %+v", versions[0])
	}

	got, err := s.GetVersionDefinition(ctx, "flw_newhire", 1)
	if err != nil {
		t.Fatalf("get version def: %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].ID != "a" {
		t.Fatalf("version 1 def = %+v, want pinned def1", got)
	}

	if _, err := s.GetVersionDefinition(ctx, "flw_newhire", 99); !store.IsNotFound(err) {
		t.Fatalf("missing version err = %v, want NotFound", err)
	}
}
