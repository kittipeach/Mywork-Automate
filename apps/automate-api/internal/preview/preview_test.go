package preview

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/sqlguard"
)

// fakeRows is an in-memory Rows over a fixed column set and value matrix.
type fakeRows struct {
	cols   []string
	rows   [][]any
	i      int
	valErr error // error returned from Values() at the current row
	err    error // sticky error surfaced by Err()
	closed bool
}

func (r *fakeRows) Next() bool {
	if r.i >= len(r.rows) {
		return false
	}
	r.i++
	return true
}

func (r *fakeRows) Values() ([]any, error) {
	if r.valErr != nil {
		return nil, r.valErr
	}
	return r.rows[r.i-1], nil
}

func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	fds := make([]pgconn.FieldDescription, len(r.cols))
	for i, c := range r.cols {
		fds[i] = pgconn.FieldDescription{Name: c}
	}
	return fds
}

func (r *fakeRows) Err() error { return r.err }
func (r *fakeRows) Close()     { r.closed = true }

// fakeQuerier returns rows (or queryErr) for any query.
type fakeQuerier struct {
	rows     *fakeRows
	queryErr error
	gotSQL   string
}

func (q *fakeQuerier) Query(_ context.Context, sql string, _ ...any) (Rows, error) {
	q.gotSQL = sql
	if q.queryErr != nil {
		return nil, q.queryErr
	}
	return q.rows, nil
}

func newEngine(t *testing.T) *masking.Engine {
	t.Helper()
	eng, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	return eng
}

func TestQuery_HappyPath_MasksAndShapes(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{
		cols: []string{"name", "salary", "email"},
		rows: [][]any{
			{"Alice", 50000, "alice@example.com"},
			{"Bob", 60000, "bob@example.com"},
		},
	}}
	res, err := Query(context.Background(), q, newEngine(t), "SELECT name, salary, email FROM staff", 0, []string{"designer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.RowCount != 2 || len(res.Items) != 2 {
		t.Fatalf("rowCount = %d, items = %d, want 2/2", res.Meta.RowCount, len(res.Items))
	}
	if res.Meta.Truncated {
		t.Errorf("truncated = true, want false")
	}
	if got := res.Meta.Columns; len(got) != 3 || got[0] != "name" || got[1] != "salary" || got[2] != "email" {
		t.Errorf("columns = %v", got)
	}
	// salary is StyleFull → fixed token; name is untouched; email is hashed.
	first := res.Items[0]
	if first["name"] != "Alice" {
		t.Errorf("name masked unexpectedly: %v", first["name"])
	}
	if first["salary"] != "******" {
		t.Errorf("salary not fully masked: %v", first["salary"])
	}
	if first["email"] == "alice@example.com" {
		t.Errorf("email not masked: %v", first["email"])
	}
}

func TestQuery_InvalidSQL_Rejected(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{}}
	_, err := Query(context.Background(), q, newEngine(t), "DELETE FROM staff", 0, []string{"admin"})
	if err == nil {
		t.Fatal("expected error for non-SELECT SQL")
	}
	if !errors.Is(err, ErrInvalidSQL) {
		t.Errorf("error = %v, want ErrInvalidSQL", err)
	}
	// The underlying sqlguard sentinel message is preserved.
	if !errors.Is(err, sqlguard.ErrNotSelect) {
		// errors.Is won't match through fmt %s, but the message must contain it.
		if got := err.Error(); got == "" || !contains(got, "SELECT") {
			t.Errorf("error message lost sqlguard detail: %v", got)
		}
	}
	if q.gotSQL != "" {
		t.Errorf("querier was called for invalid SQL: %q", q.gotSQL)
	}
}

func TestQuery_CapsAtMaxRows(t *testing.T) {
	rows := make([][]any, MaxRows+10) // more than the cap available
	for i := range rows {
		rows[i] = []any{i}
	}
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: rows}}
	res, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM series", 1000, []string{"admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.RowCount != MaxRows {
		t.Errorf("rowCount = %d, want %d (capped)", res.Meta.RowCount, MaxRows)
	}
	if !res.Meta.Truncated {
		t.Errorf("truncated = false, want true (more rows than cap)")
	}
}

func TestQuery_CapsAtRequestedMaxRows(t *testing.T) {
	rows := make([][]any, 20)
	for i := range rows {
		rows[i] = []any{i}
	}
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: rows}}
	res, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM series", 5, []string{"admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.RowCount != 5 || !res.Meta.Truncated {
		t.Errorf("rowCount = %d truncated = %v, want 5/true", res.Meta.RowCount, res.Meta.Truncated)
	}
}

func TestQuery_ExactlyMaxRows_NotTruncated(t *testing.T) {
	rows := make([][]any, MaxRows)
	for i := range rows {
		rows[i] = []any{i}
	}
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: rows}}
	res, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM series", 0, []string{"admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Meta.RowCount != MaxRows || res.Meta.Truncated {
		t.Errorf("rowCount = %d truncated = %v, want %d/false", res.Meta.RowCount, res.Meta.Truncated, MaxRows)
	}
}

func TestQuery_EmptyResult_ReturnsEmptyItems(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: [][]any{}}}
	res, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM series", 0, []string{"admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Items == nil {
		t.Fatal("items must be non-nil (serialise as [])")
	}
	if res.Meta.RowCount != 0 {
		t.Errorf("rowCount = %d, want 0", res.Meta.RowCount)
	}
}

func TestQuery_QueryError(t *testing.T) {
	q := &fakeQuerier{queryErr: errors.New("connection refused")}
	_, err := Query(context.Background(), q, newEngine(t), "SELECT 1", 0, []string{"admin"})
	if err == nil {
		t.Fatal("expected error from querier")
	}
	if errors.Is(err, ErrInvalidSQL) {
		t.Errorf("a query error must not masquerade as invalid_sql: %v", err)
	}
}

func TestQuery_ValuesError(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: [][]any{{1}}, valErr: errors.New("decode fail")}}
	_, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM t", 0, []string{"admin"})
	if err == nil {
		t.Fatal("expected error from Values()")
	}
}

func TestQuery_RowsError(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"n"}, rows: [][]any{{1}}, err: errors.New("stream broke")}}
	_, err := Query(context.Background(), q, newEngine(t), "SELECT n FROM t", 0, []string{"admin"})
	if err == nil {
		t.Fatal("expected sticky rows error")
	}
}

func TestFetchSchema_Shaping(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{
		cols: []string{"table_name", "column_name", "data_type"},
		rows: [][]any{
			{"employees", "id", "integer"},
			{"employees", "name", "text"},
			{"employees", "salary", "numeric"},
			{"payroll", "run_id", "uuid"},
			{"payroll", "amount", "numeric"},
		},
	}}
	sch, err := FetchSchema(context.Background(), q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sch.Tables) != 2 {
		t.Fatalf("tables = %d, want 2", len(sch.Tables))
	}
	if sch.Tables[0].Name != "employees" || len(sch.Tables[0].Columns) != 3 {
		t.Fatalf("employees shape wrong: %+v", sch.Tables[0])
	}
	if sch.Tables[0].Columns[0].Name != "id" || sch.Tables[0].Columns[0].Type != "integer" {
		t.Errorf("first column wrong: %+v", sch.Tables[0].Columns[0])
	}
	if sch.Tables[1].Name != "payroll" || len(sch.Tables[1].Columns) != 2 {
		t.Errorf("payroll shape wrong: %+v", sch.Tables[1])
	}
}

func TestFetchSchema_Empty(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"table_name", "column_name", "data_type"}, rows: [][]any{}}}
	sch, err := FetchSchema(context.Background(), q)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sch.Tables == nil || len(sch.Tables) != 0 {
		t.Errorf("tables = %v, want empty non-nil", sch.Tables)
	}
}

func TestFetchSchema_QueryError(t *testing.T) {
	q := &fakeQuerier{queryErr: errors.New("no db")}
	_, err := FetchSchema(context.Background(), q)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFetchSchema_ValuesError(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"table_name", "column_name", "data_type"}, rows: [][]any{{"t", "c", "int"}}, valErr: errors.New("boom")}}
	_, err := FetchSchema(context.Background(), q)
	if err == nil {
		t.Fatal("expected error from Values()")
	}
}

func TestFetchSchema_RowsError(t *testing.T) {
	q := &fakeQuerier{rows: &fakeRows{cols: []string{"table_name", "column_name", "data_type"}, rows: [][]any{{"t", "c", "int"}}, err: errors.New("stream broke")}}
	_, err := FetchSchema(context.Background(), q)
	if err == nil {
		t.Fatal("expected sticky rows error")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
