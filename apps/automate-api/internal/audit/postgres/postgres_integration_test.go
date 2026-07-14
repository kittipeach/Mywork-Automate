//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	storepg "github.com/mywork/automate/apps/automate-api/internal/store/postgres"
)

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://automate:automate@localhost:5432/automate?sslmode=disable"
}

// freshService connects, drops audit_logs + schema_migrations so the 0003
// migration re-applies deterministically, migrates, and returns a Service.
func freshService(t *testing.T, ctx context.Context) (*pgxpool.Pool, *Service) {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS audit_logs CASCADE"); err != nil {
		t.Fatalf("drop audit_logs: %v", err)
	}
	// Reset migration bookkeeping so Migrate re-runs every numbered file.
	if _, err := pool.Exec(ctx, "DROP TABLE IF EXISTS schema_migrations CASCADE"); err != nil {
		t.Fatalf("drop schema_migrations: %v", err)
	}
	if err := storepg.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Migrate is idempotent — a second run must be a no-op.
	if err := storepg.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
	return pool, New(pool)
}

func TestIntegration_RecordAndList(t *testing.T) {
	ctx := context.Background()
	pool, s := freshService(t, ctx)
	defer pool.Close()

	entries := []audit.Entry{
		{UserID: "user_admin", Role: "admin", Action: audit.ActionAuthLogin, IP: "10.0.0.1", UserAgent: "curl/8"},
		{UserID: "user_admin", Role: "admin", Action: audit.ActionFlowPublish,
			ResourceType: "flow", ResourceID: "flw_payroll",
			Detail: map[string]any{"version": float64(3), "changeNote": "go live"}, IP: "10.0.0.1", UserAgent: "curl/8"},
		// nil actor + nil detail must persist as NULLs and read back empty.
		{Role: "viewer", Action: audit.ActionFileDownload, ResourceType: "file", ResourceID: "file_9"},
	}
	for i, e := range entries {
		if err := s.Record(ctx, e); err != nil {
			t.Fatalf("record[%d]: %v", i, err)
		}
	}

	all, err := s.List(ctx, audit.Filter{})
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("list all = %d, want 3", len(all))
	}
	// Most-recent first: the file.download (last inserted) leads.
	if all[0].Action != audit.ActionFileDownload {
		t.Fatalf("order wrong, head = %+v", all[0])
	}
	// Server-assigned fields are populated.
	if all[0].ID == 0 || all[0].CreatedAt == "" {
		t.Fatalf("missing server-assigned id/createdAt: %+v", all[0])
	}
	// Nil actor round-trips as empty UserID; nil detail as nil map.
	if all[0].UserID != "" || all[0].Detail != nil {
		t.Fatalf("null actor/detail not round-tripped: %+v", all[0])
	}

	// Action filter.
	pub, err := s.List(ctx, audit.Filter{Action: audit.ActionFlowPublish})
	if err != nil {
		t.Fatalf("list by action: %v", err)
	}
	if len(pub) != 1 || pub[0].ResourceID != "flw_payroll" {
		t.Fatalf("action filter = %+v", pub)
	}
	// JSONB detail round-trips.
	if pub[0].Detail["version"] != float64(3) || pub[0].Detail["changeNote"] != "go live" {
		t.Fatalf("detail not round-tripped: %+v", pub[0].Detail)
	}

	// From/To window: >= the first entry's timestamp returns everything.
	win, err := s.List(ctx, audit.Filter{From: all[len(all)-1].CreatedAt, To: all[0].CreatedAt})
	if err != nil {
		t.Fatalf("list window: %v", err)
	}
	if len(win) != 3 {
		t.Fatalf("window = %d, want 3", len(win))
	}

	// A From strictly after every entry returns an empty, non-nil slice.
	empty, err := s.List(ctx, audit.Filter{From: "2099-01-01T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("list future: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("future window = %v, want non-nil empty", empty)
	}
}

func TestIntegration_ListLimitCap(t *testing.T) {
	ctx := context.Background()
	pool, s := freshService(t, ctx)
	defer pool.Close()

	for i := 0; i < 5; i++ {
		if err := s.Record(ctx, audit.Entry{Role: "admin", Action: audit.ActionAICall}); err != nil {
			t.Fatalf("record[%d]: %v", i, err)
		}
	}
	got, err := s.List(ctx, audit.Filter{Limit: 2})
	if err != nil {
		t.Fatalf("list limit: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("limit 2 = %d rows, want 2", len(got))
	}
}
