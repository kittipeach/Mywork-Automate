// Package preview runs read-only query previews and schema introspection for a
// connection's database (stories E6-S4 and E6-S2 backend). It sits behind a tiny
// Querier interface so the httpapi handlers can inject the API's pgx pool in
// production and a fake in unit tests: the SQL-guard reject, row-shaping, cap and
// masking paths are all exercised without a live database, while the real pgx
// query path is covered by the integration suite.
//
// Every preview query is validated by pkg/sqlguard (SELECT-only) before it
// touches the database, and every returned row is masked by the injected masking
// engine at the preview enforcement point for the caller's roles
// (docs/spec/07 §2.2). SQL is always parameterised where user values are
// involved; the introspection queries below take no user input.
package preview

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/sqlguard"
)

// MaxRows is the hard cap on rows returned by a preview regardless of the
// requested maxRows, and the default when the request omits it.
const MaxRows = 50

// Rows is the minimal subset of pgx.Rows the preview logic consumes. *pgxpool.Pool
// returns a pgx.Rows which satisfies this natively; unit tests supply a fake.
type Rows interface {
	Next() bool
	Values() ([]any, error)
	FieldDescriptions() []pgconn.FieldDescription
	Err() error
	Close()
}

// Querier is the seam over the database. In production it is a *pgxpool.Pool
// (its Query returns a pgx.Rows, which is assignable to Rows); in tests it is a
// fake. NOTE (demo): the handlers inject the same pool the store uses. A
// production build would instead resolve each connection's credentials from Key
// Vault (pkg/secrets) and build a dedicated, least-privilege pool per connection.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (Rows, error)
}

// Meta is the query-preview response metadata: the ordered column names, the
// number of rows actually returned, and whether the result was truncated at the
// row cap.
type Meta struct {
	Columns   []string `json:"columns"`
	RowCount  int      `json:"rowCount"`
	Truncated bool     `json:"truncated"`
}

// Result is a masked query preview: the rows as generic maps plus the metadata.
type Result struct {
	Items []map[string]any `json:"items"`
	Meta  Meta             `json:"meta"`
}

// ErrInvalidSQL wraps a sqlguard rejection so the handler can map it to a 400
// with the invalid_sql code while preserving the sentinel message (errors.Is
// still matches the underlying sqlguard sentinel).
var ErrInvalidSQL = errors.New("invalid_sql")

// Query validates sql (SELECT-only), runs it via q, caps the result at
// min(maxRows, MaxRows) (a non-positive maxRows means MaxRows), maps rows to
// []map[string]any keyed by column name, and masks them at the preview point for
// viewerRoles. A sqlguard rejection is returned wrapped in ErrInvalidSQL (the
// sentinel message is preserved). The caller supplies a masking engine so the
// rule set is configured once at the composition root.
func Query(ctx context.Context, q Querier, eng *masking.Engine, sql string, maxRows int, viewerRoles []string) (Result, error) {
	if err := sqlguard.Validate(sql); err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrInvalidSQL, err.Error())
	}

	limit := maxRows
	if limit <= 0 || limit > MaxRows {
		limit = MaxRows
	}

	rows, err := q.Query(ctx, sql)
	if err != nil {
		return Result{}, fmt.Errorf("preview: query: %w", err)
	}
	defer rows.Close()

	cols := columnNames(rows.FieldDescriptions())

	items := make([]map[string]any, 0)
	truncated := false
	for rows.Next() {
		if len(items) == limit {
			// One row beyond the cap exists → the result is truncated. Stop reading.
			truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return Result{}, fmt.Errorf("preview: read row: %w", err)
		}
		items = append(items, rowToMap(cols, vals))
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("preview: rows: %w", err)
	}

	// items is always a non-nil slice (make above), so MaskItems returns a
	// non-nil masked copy — the response serialises as [] for an empty result.
	masked := eng.MaskItems(items, masking.PointPreview, viewerRoles)

	return Result{
		Items: masked,
		Meta:  Meta{Columns: cols, RowCount: len(masked), Truncated: truncated},
	}, nil
}

// columnNames extracts the ordered column names from the field descriptions.
func columnNames(fds []pgconn.FieldDescription) []string {
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}
	return cols
}

// rowToMap pairs column names with a row's values. Extra values without a column
// name (should not happen for a real result set) are ignored.
func rowToMap(cols []string, vals []any) map[string]any {
	m := make(map[string]any, len(cols))
	for i, c := range cols {
		if i < len(vals) {
			m[c] = vals[i]
		}
	}
	return m
}
