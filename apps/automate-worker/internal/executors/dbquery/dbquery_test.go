package dbquery

import (
	"context"
	"errors"
	"testing"

	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
	"github.com/mywork/automate/pkg/sqlguard"
)

// fakeQuerier returns a canned RowSet and records whether it was called (to
// prove the SQL guard blocks before any DB access).
type fakeQuerier struct {
	rs     RowSet
	err    error
	called bool
}

func (f *fakeQuerier) Query(_ context.Context, _ string, _ []any) (RowSet, error) {
	f.called = true
	return f.rs, f.err
}

// fakeResolver implements secrets.Resolver.
type fakeResolver struct {
	val    string
	err    error
	called bool
}

func (r *fakeResolver) Resolve(_ context.Context, _ string) (string, error) {
	r.called = true
	return r.val, r.err
}

func maskEngine(t *testing.T) *masking.Engine {
	t.Helper()
	e, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func baseDeps(t *testing.T, q Querier) Deps {
	t.Helper()
	return Deps{
		Secrets:   &fakeResolver{val: "pw"},
		Querier:   q,
		Masking:   maskEngine(t),
		MaskPoint: masking.PointPreview,
	}
}

func TestExecute_HappyPath_MasksSensitiveColumns(t *testing.T) {
	q := &fakeQuerier{rs: RowSet{
		Columns: []string{"employee_id", "salary", "department"},
		Rows: [][]any{
			{1, 50000, "HR"},
			{2, 60000, "IT"},
		},
	}}
	out, err := Execute(context.Background(), Input{
		SQL:        "SELECT employee_id, salary, department FROM staff",
		ConnSecret: "conn-1",
	}, baseDeps(t, q))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Meta.RowCount != 2 || out.Meta.Truncated {
		t.Fatalf("meta = %+v, want RowCount 2, Truncated false", out.Meta)
	}
	// salary must be masked; employee_id/department must be untouched.
	if out.Items[0]["salary"] == 50000 {
		t.Error("salary was not masked")
	}
	if out.Items[0]["employee_id"] != 1 || out.Items[0]["department"] != "HR" {
		t.Errorf("non-sensitive columns altered: %+v", out.Items[0])
	}
}

func TestExecute_InjectionBlockedBeforeDB(t *testing.T) {
	for _, sql := range []string{
		"DROP TABLE staff",
		"SELECT 1; DROP TABLE staff",
		"WITH x AS (DELETE FROM staff RETURNING *) SELECT * FROM x",
		"SELECT * FROM staff FOR UPDATE",
	} {
		q := &fakeQuerier{}
		_, err := Execute(context.Background(), Input{SQL: sql, ConnSecret: "c"}, baseDeps(t, q))
		if err == nil {
			t.Fatalf("Execute(%q) = nil error, want rejection", sql)
		}
		if !errors.Is(err, sqlguard.ErrNotSelect) &&
			!errors.Is(err, sqlguard.ErrMultipleStatements) &&
			!errors.Is(err, sqlguard.ErrForbiddenConstruct) {
			t.Fatalf("Execute(%q) err = %v, want a sqlguard sentinel", sql, err)
		}
		if q.called {
			t.Fatalf("Execute(%q) reached the database — guard must block first", sql)
		}
	}
}

func TestExecute_MaxRowsTruncation(t *testing.T) {
	q := &fakeQuerier{rs: RowSet{
		Columns: []string{"n"},
		Rows:    [][]any{{1}, {2}, {3}, {4}, {5}},
	}}
	out, err := Execute(context.Background(), Input{
		SQL:     "SELECT n FROM series",
		MaxRows: 2,
	}, baseDeps(t, q))
	if err != nil {
		t.Fatal(err)
	}
	if out.Meta.RowCount != 2 || !out.Meta.Truncated {
		t.Fatalf("meta = %+v, want RowCount 2, Truncated true", out.Meta)
	}
}

func TestExecute_SecretResolveFailure(t *testing.T) {
	q := &fakeQuerier{rs: RowSet{Columns: []string{"n"}, Rows: [][]any{{1}}}}
	deps := baseDeps(t, q)
	deps.Secrets = &fakeResolver{err: secrets.ErrNotFound}
	_, err := Execute(context.Background(), Input{SQL: "SELECT n FROM t", ConnSecret: "missing"}, deps)
	if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatalf("err = %v, want secrets.ErrNotFound", err)
	}
	if q.called {
		t.Fatal("query ran despite credential resolution failure")
	}
}

func TestExecute_NoSecretSkipsResolver(t *testing.T) {
	q := &fakeQuerier{rs: RowSet{Columns: []string{"n"}, Rows: [][]any{{1}}}}
	r := &fakeResolver{val: "pw"}
	deps := baseDeps(t, q)
	deps.Secrets = r
	if _, err := Execute(context.Background(), Input{SQL: "SELECT n FROM t"}, deps); err != nil {
		t.Fatal(err)
	}
	if r.called {
		t.Error("resolver called even though ConnSecret was empty")
	}
}

func TestExecute_QuerierError(t *testing.T) {
	sentinel := errors.New("conn reset")
	q := &fakeQuerier{err: sentinel}
	_, err := Execute(context.Background(), Input{SQL: "SELECT 1", ConnSecret: "c"}, baseDeps(t, q))
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want wraps conn reset", err)
	}
}

func TestExecute_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q := &fakeQuerier{}
	_, err := Execute(ctx, Input{SQL: "SELECT 1"}, baseDeps(t, q))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if q.called {
		t.Fatal("query ran on a cancelled context")
	}
}

func TestExecute_ExemptRoleSeesCleartext(t *testing.T) {
	// A rule that exempts the "admin" role; an admin viewer must see the raw value.
	e, err := masking.NewEngine([]masking.Rule{{
		Name:        "salary",
		MatchType:   masking.MatchColumnName,
		Pattern:     "(?i)^salary$",
		Style:       masking.StyleFull,
		ExemptRoles: []string{"admin"},
		AppliesTo:   []masking.Point{masking.PointPreview},
		Active:      true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	q := &fakeQuerier{rs: RowSet{Columns: []string{"salary"}, Rows: [][]any{{50000}}}}
	deps := Deps{Secrets: &fakeResolver{}, Querier: q, Masking: e, MaskPoint: masking.PointPreview}

	admin, _ := Execute(context.Background(), Input{SQL: "SELECT salary FROM t", ViewerRoles: []string{"admin"}}, deps)
	if admin.Items[0]["salary"] != 50000 {
		t.Errorf("exempt admin should see cleartext, got %v", admin.Items[0]["salary"])
	}
	viewer, _ := Execute(context.Background(), Input{SQL: "SELECT salary FROM t", ViewerRoles: []string{"viewer"}}, deps)
	if viewer.Items[0]["salary"] == 50000 {
		t.Error("non-exempt viewer should see masked salary")
	}
}

func TestExecute_NilMaskingFailsClosed(t *testing.T) {
	// Banking-grade: refuse to return external-DB rows without a masking engine.
	q := &fakeQuerier{rs: RowSet{Columns: []string{"salary"}, Rows: [][]any{{50000}}}}
	r := &fakeResolver{val: "pw"}
	deps := Deps{Secrets: r, Querier: q} // Masking nil
	_, err := Execute(context.Background(), Input{SQL: "SELECT salary FROM t", ConnSecret: "c"}, deps)
	if !errors.Is(err, ErrMaskingRequired) {
		t.Fatalf("err = %v, want ErrMaskingRequired", err)
	}
	if q.called {
		t.Fatal("query ran without a masking engine — must fail closed before the DB")
	}
	if r.called {
		t.Fatal("credential resolved before the masking-engine check")
	}
}

// An explicitly empty engine is the sanctioned way to apply no masking rules.
func TestExecute_EmptyEngineIsAllowedOptOut(t *testing.T) {
	empty, err := masking.NewEngine(nil)
	if err != nil {
		t.Fatal(err)
	}
	q := &fakeQuerier{rs: RowSet{Columns: []string{"salary"}, Rows: [][]any{{50000}}}}
	deps := Deps{Secrets: &fakeResolver{}, Querier: q, Masking: empty, MaskPoint: masking.PointPreview}
	out, err := Execute(context.Background(), Input{SQL: "SELECT salary FROM t"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[0]["salary"] != 50000 {
		t.Errorf("empty engine should apply no rules, got %v", out.Items[0]["salary"])
	}
}

func TestExecute_EmptyResultAndDefaultLimit(t *testing.T) {
	q := &fakeQuerier{rs: RowSet{Columns: []string{"n"}, Rows: nil}}
	deps := baseDeps(t, q)
	deps.DefaultMaxRows = 10 // exercise the Deps-default branch of rowLimit
	out, err := Execute(context.Background(), Input{SQL: "SELECT n FROM t"}, deps)
	if err != nil {
		t.Fatal(err)
	}
	if out.Meta.RowCount != 0 || out.Meta.Truncated {
		t.Fatalf("meta = %+v, want empty untruncated", out.Meta)
	}
}

func TestRowLimit_Fallbacks(t *testing.T) {
	if got := rowLimit(0, 0); got != defaultMaxRows {
		t.Errorf("rowLimit(0,0) = %d, want %d", got, defaultMaxRows)
	}
	if got := rowLimit(0, 25); got != 25 {
		t.Errorf("rowLimit(0,25) = %d, want 25", got)
	}
	if got := rowLimit(7, 25); got != 7 {
		t.Errorf("rowLimit(7,25) = %d, want 7", got)
	}
}

func TestToItems_RaggedRowShorterThanColumns(t *testing.T) {
	// A row with fewer values than columns must not panic; missing cols are absent.
	items := toItems([]string{"a", "b"}, [][]any{{1}})
	if len(items) != 1 || items[0]["a"] != 1 {
		t.Fatalf("items = %+v", items)
	}
	if _, ok := items[0]["b"]; ok {
		t.Error("short row should not populate missing column b")
	}
}
