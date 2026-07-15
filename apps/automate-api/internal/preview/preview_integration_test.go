//go:build integration

// These tests exercise the real pgx query path (PoolQuerier over a live
// *pgxpool.Pool) for both query preview and schema introspection. They are
// excluded from the unit-coverage gate (like the store/postgres integration
// suite) and run under `go test -tags=integration`.
package preview

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/pkg/masking"
)

func dsn() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://automate:automate@localhost:5432/automate?sslmode=disable"
}

func freshPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(ctx, dsn())
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	return pool
}

func TestIntegration_Query_RealPool(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t, ctx)
	defer pool.Close()

	// Seed a throwaway table with a sensitive column to prove masking runs.
	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS preview_it`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE preview_it (name text, salary integer)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO preview_it (name, salary) VALUES ('Alice', 50000), ('Bob', 60000)`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	defer pool.Exec(ctx, `DROP TABLE IF EXISTS preview_it`) //nolint:errcheck

	eng, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		t.Fatalf("engine: %v", err)
	}

	res, err := Query(ctx, PoolQuerier{Pool: pool}, eng, `SELECT name, salary FROM preview_it ORDER BY name`, 0, []string{"designer"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if res.Meta.RowCount != 2 {
		t.Fatalf("rowCount = %d, want 2", res.Meta.RowCount)
	}
	if res.Items[0]["name"] != "Alice" {
		t.Errorf("name = %v", res.Items[0]["name"])
	}
	if res.Items[0]["salary"] != "******" {
		t.Errorf("salary not masked over the real pool: %v", res.Items[0]["salary"])
	}
	if len(res.Meta.Columns) != 2 || res.Meta.Columns[0] != "name" {
		t.Errorf("columns = %v", res.Meta.Columns)
	}
}

func TestIntegration_Query_RejectsNonSelect_RealPool(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t, ctx)
	defer pool.Close()

	eng, _ := masking.NewEngine(masking.DefaultRules())
	if _, err := Query(ctx, PoolQuerier{Pool: pool}, eng, `DROP TABLE preview_it`, 0, []string{"admin"}); err == nil {
		t.Fatal("expected sqlguard rejection for DDL")
	}
}

func TestIntegration_FetchSchema_RealPool(t *testing.T) {
	ctx := context.Background()
	pool := freshPool(t, ctx)
	defer pool.Close()

	if _, err := pool.Exec(ctx, `DROP TABLE IF EXISTS schema_it`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE schema_it (id integer, label text)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	defer pool.Exec(ctx, `DROP TABLE IF EXISTS schema_it`) //nolint:errcheck

	sch, err := FetchSchema(ctx, PoolQuerier{Pool: pool})
	if err != nil {
		t.Fatalf("schema: %v", err)
	}
	var found *Table
	for i := range sch.Tables {
		if sch.Tables[i].Name == "schema_it" {
			found = &sch.Tables[i]
		}
	}
	if found == nil {
		t.Fatalf("schema_it not present in %d tables", len(sch.Tables))
	}
	if len(found.Columns) != 2 || found.Columns[0].Name != "id" || found.Columns[0].Type != "integer" {
		t.Errorf("schema_it columns wrong: %+v", found.Columns)
	}
}
