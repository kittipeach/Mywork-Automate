// Command dbquery-demo is a dev-only demonstration of the db.query security
// path: sqlguard (SELECT-only) -> pkg/secrets (credential) -> pkg/masking.
// It uses an IN-MEMORY result set (not a real Postgres) to show the transforms;
// the pgx-backed Querier against a real DB is the deferred integration seam.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
)

// memQuerier is a stand-in for a real database: it ignores the SQL (already
// validated by sqlguard) and returns a fixed HR result set.
type memQuerier struct{}

func (memQuerier) Query(_ context.Context, _ string, _ []any) (dbquery.RowSet, error) {
	return dbquery.RowSet{
		Columns: []string{"name", "salary", "citizen_id", "phone", "email"},
		Rows: [][]any{
			{"Somchai P.", 82000, "1103700123456", "0812345678", "somchai@example.com"},
			{"Naree K.", 65000, "3101200987654", "0899876543", "naree@example.com"},
		},
	}, nil
}

func dump(label string, out dbquery.Output) {
	b, _ := json.MarshalIndent(out.Items, "  ", "  ")
	fmt.Printf("%s  (rowCount=%d truncated=%v)\n  %s\n\n", label, out.Meta.RowCount, out.Meta.Truncated, b)
}

func main() {
	ctx := context.Background()

	// Real credential resolution through pkg/secrets (file resolver).
	dir, _ := os.MkdirTemp("", "demo-secrets")
	secretsPath := filepath.Join(dir, "secrets.local.yaml")
	_ = os.WriteFile(secretsPath, []byte("hr-db-password: s3cr3t-do-not-log\n"), 0o600)
	fileRes, err := secrets.NewFileResolver(secretsPath)
	if err != nil {
		fmt.Println("secret resolver:", err)
		os.Exit(1)
	}
	resolver := secrets.NewCachingResolver(fileRes, 0, nil)

	fmt.Println("=== 1) Valid SELECT, viewer role → DEFAULT masking rules applied ===")
	defaultEngine, _ := masking.NewEngine(masking.DefaultRules())
	out, err := dbquery.Execute(ctx, dbquery.Input{
		SQL:         `SELECT name, salary, citizen_id, phone, email FROM employees WHERE dept = $1`,
		Params:      []any{"HR"},
		ViewerRoles: []string{"viewer"},
		ConnSecret:  "hr-db-password",
	}, dbquery.Deps{Secrets: resolver, Querier: memQuerier{}, Masking: defaultEngine, MaskPoint: masking.PointPreview})
	if err != nil {
		fmt.Println("unexpected:", err)
		os.Exit(1)
	}
	dump("masked for viewer:", out)

	fmt.Println("=== 2) Same query, payroll-admin EXEMPT on salary → sees cleartext salary ===")
	exemptEngine, _ := masking.NewEngine([]masking.Rule{{
		Name: "salary", MatchType: masking.MatchColumnName, Pattern: "(?i)^salary$",
		Style: masking.StyleFull, ExemptRoles: []string{"payroll-admin"},
		AppliesTo: []masking.Point{masking.PointPreview}, Active: true,
	}})
	out, _ = dbquery.Execute(ctx, dbquery.Input{
		SQL:         `SELECT name, salary FROM employees`,
		ViewerRoles: []string{"payroll-admin"},
		ConnSecret:  "hr-db-password",
	}, dbquery.Deps{Secrets: resolver, Querier: memQuerier{}, Masking: exemptEngine, MaskPoint: masking.PointPreview})
	dump("cleartext for exempt admin:", out)

	fmt.Println("=== 3) Malicious stacked statement → BLOCKED before the DB is touched ===")
	_, err = dbquery.Execute(ctx, dbquery.Input{
		SQL:         `SELECT name FROM employees; DROP TABLE employees; --`,
		ViewerRoles: []string{"viewer"},
		ConnSecret:  "hr-db-password",
	}, dbquery.Deps{Secrets: resolver, Querier: memQuerier{}, Masking: defaultEngine, MaskPoint: masking.PointPreview})
	fmt.Printf("rejected: %v\n", err)
}
