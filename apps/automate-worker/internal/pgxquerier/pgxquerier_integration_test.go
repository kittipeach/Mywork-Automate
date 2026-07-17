//go:build integration

package pgxquerier

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/pkg/masking"
)

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://automate:automate@localhost:5432/automate?sslmode=disable"
}

func newPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(dsn())
	if err != nil {
		t.Fatalf("parse dsn: %v", err)
	}
	cfg.MaxConns = 4 // FR-DB-010 concurrency cap
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func seed(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	stmts := []string{
		`DROP TABLE IF EXISTS demo_emp`,
		`CREATE TABLE demo_emp (id int primary key, name text, salary int, citizen_id text, phone text)`,
		`INSERT INTO demo_emp VALUES
			(1,'Somchai',82000,'1103700123456','0812345678'),
			(2,'Naree',65000,'3101200987654','0899876543'),
			(3,'Anucha',54000,'1200900112233','0861112222')`,
	}
	for _, s := range stmts {
		if _, err := pool.Exec(ctx, s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

func maskEngine(t *testing.T) *masking.Engine {
	t.Helper()
	e, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// TestPgxQuerier_MaskedPreview runs a real SELECT through dbquery.Execute and
// proves salary/citizen_id come back masked while name is untouched.
func TestPgxQuerier_MaskedPreview(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx)
	defer pool.Close()
	seed(t, ctx, pool)

	out, err := dbquery.Execute(ctx, dbquery.Input{
		SQL:         `SELECT name, salary, citizen_id FROM demo_emp ORDER BY id`,
		ViewerRoles: []string{"viewer"},
	}, dbquery.Deps{
		Querier:   New(pool, 5*time.Second),
		Masking:   maskEngine(t),
		MaskPoint: masking.PointPreview,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out.Meta.RowCount != 3 {
		t.Fatalf("rowCount = %d, want 3", out.Meta.RowCount)
	}
	if out.Items[0]["name"] != "Somchai" {
		t.Errorf("name masked unexpectedly: %v", out.Items[0]["name"])
	}
	if out.Items[0]["salary"] == int32(82000) || out.Items[0]["salary"] == 82000 {
		t.Errorf("salary NOT masked: %v", out.Items[0]["salary"])
	}
	if cid, _ := out.Items[0]["citizen_id"].(string); cid == "1103700123456" {
		t.Errorf("citizen_id NOT masked: %v", cid)
	}
}

// TestPgxQuerier_MaxRows proves the row cap truncates real results.
func TestPgxQuerier_MaxRows(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx)
	defer pool.Close()
	seed(t, ctx, pool)

	out, err := dbquery.Execute(ctx, dbquery.Input{
		SQL:     `SELECT id FROM demo_emp ORDER BY id`,
		MaxRows: 2,
	}, dbquery.Deps{Querier: New(pool, 5*time.Second), Masking: maskEngine(t), MaskPoint: masking.PointPreview})
	if err != nil {
		t.Fatal(err)
	}
	if out.Meta.RowCount != 2 || !out.Meta.Truncated {
		t.Fatalf("meta = %+v, want RowCount 2 Truncated true", out.Meta)
	}
}

// TestPgxQuerier_StatementTimeout proves FR-DB-007: a slow query is cancelled.
func TestPgxQuerier_StatementTimeout(t *testing.T) {
	ctx := context.Background()
	pool := newPool(t, ctx)
	defer pool.Close()

	start := time.Now()
	_, err := New(pool, 300*time.Millisecond).Query(ctx, "SELECT pg_sleep(3)", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("statement timeout not enforced: took %v", elapsed)
	}
	// sanity: it should be a context/timeout style failure, not a parse error
	if errors.Is(err, context.Canceled) {
		t.Log("cancelled via context")
	}
}
