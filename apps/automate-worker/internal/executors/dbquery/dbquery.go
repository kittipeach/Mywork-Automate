// Package dbquery is the db.query node executor (spec 08 E6-S4, FR-DB-006..008,010).
//
// It is the security-critical read path and composes three verified packages:
//   - pkg/sqlguard  — the SQL is validated as a single read-only SELECT BEFORE
//     the database is ever touched (injection/DML is rejected up front).
//   - pkg/secrets   — the connection credential is resolved through the Resolver
//     (Key Vault in prod, file in dev); it is never read from env/config here.
//   - pkg/masking   — result rows pass through the masking engine (preview point)
//     for the viewer's roles before leaving this package.
//
// The actual database call is behind the Querier seam so the executor logic is
// deterministically unit-testable; the pgx-backed Querier is supplied by the
// worker activity that wires this to a real connection pool.
package dbquery

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
	"github.com/mywork/automate/pkg/sqlguard"
)

// ErrMaskingRequired is returned when Execute is called without a masking
// engine. The executor fails closed: external-DB rows are never returned
// without an explicit masking decision. To intentionally apply no rules, pass
// an empty engine (masking.NewEngine(nil)).
var ErrMaskingRequired = errors.New("dbquery: masking engine is required")

// defaultMaxRows caps a query when neither the request nor Deps specify a limit.
const defaultMaxRows = 50

// RowSet is a materialized query result handed back by a Querier.
type RowSet struct {
	Columns []string
	Rows    [][]any
}

// Querier runs an already-validated, parameterized SELECT. Implementations must
// only ever be called with SQL that sqlguard has accepted.
type Querier interface {
	Query(ctx context.Context, sql string, args []any) (RowSet, error)
}

// Deps are the collaborators the executor composes.
type Deps struct {
	Secrets   secrets.Resolver
	Querier   Querier
	Masking   *masking.Engine
	MaskPoint masking.Point
	// OpenQuerier returns a Querier for an external connection's DSN (a per-DSN
	// pooled connection, cached by the worker). When nil, or when the Input has
	// no connection dial fields, the executor falls back to the default Querier.
	OpenQuerier    func(ctx context.Context, dsn string) (Querier, error)
	DefaultMaxRows int
}

// Input is one db.query invocation.
type Input struct {
	SQL         string
	Params      []any
	MaxRows     int      // <=0 => Deps.DefaultMaxRows, else defaultMaxRows
	ViewerRoles []string // roles of the user this result is being prepared for
	ConnSecret  string   // secret name of the connection password (resolved via Secrets)

	// Connection dial target (E6-S1). When Host/Database/Username are all set and
	// Deps.OpenQuerier is provided, the query runs against THIS external database
	// (password resolved from ConnSecret); otherwise it uses Deps.Querier.
	ConnHost     string
	ConnPort     int
	ConnDatabase string
	ConnUsername string
	ConnSSLMode  string
}

// dialsExternal reports whether the input names a specific external database.
func (in Input) dialsExternal() bool {
	return in.ConnHost != "" && in.ConnDatabase != "" && in.ConnUsername != ""
}

// dsn builds the pgx connection string for the external target using password.
func (in Input) dsn(password string) string {
	port := in.ConnPort
	if port == 0 {
		port = 5432
	}
	ssl := in.ConnSSLMode
	if ssl == "" {
		ssl = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(in.ConnUsername), url.QueryEscape(password), in.ConnHost, port, in.ConnDatabase, ssl)
}

// Meta describes the shaped result.
type Meta struct {
	RowCount  int
	Truncated bool
	Columns   []string
}

// Output is the executor result: masked items plus metadata.
type Output struct {
	Items []map[string]any
	Meta  Meta
}

// Execute validates, runs, caps, shapes and masks a db.query node.
func Execute(ctx context.Context, in Input, deps Deps) (Output, error) {
	if err := ctx.Err(); err != nil {
		return Output{}, err
	}
	// 1. SELECT-only guard BEFORE any DB access — injection/DML never reaches the DB.
	if err := sqlguard.Validate(in.SQL); err != nil {
		return Output{}, fmt.Errorf("dbquery: rejected sql: %w", err)
	}
	// Fail closed: never query external data without an explicit masking decision.
	if deps.Masking == nil {
		return Output{}, ErrMaskingRequired
	}
	// 2. Resolve the connection credential and select the target Querier. When the
	// input names an external database (E6-S1), resolve its password and open a
	// per-connection pool against it; otherwise use the default Querier (the demo
	// pool) after validating any provided credential still resolves.
	querier := deps.Querier
	switch {
	case in.dialsExternal() && deps.OpenQuerier != nil:
		password := ""
		if in.ConnSecret != "" {
			p, err := deps.Secrets.Resolve(ctx, in.ConnSecret)
			if err != nil {
				return Output{}, fmt.Errorf("dbquery: resolve credential: %w", err)
			}
			password = p
		}
		q, err := deps.OpenQuerier(ctx, in.dsn(password))
		if err != nil {
			return Output{}, fmt.Errorf("dbquery: open connection: %w", err)
		}
		querier = q
	case in.ConnSecret != "":
		if _, err := deps.Secrets.Resolve(ctx, in.ConnSecret); err != nil {
			return Output{}, fmt.Errorf("dbquery: resolve credential: %w", err)
		}
	}
	if querier == nil {
		return Output{}, fmt.Errorf("dbquery: no querier available")
	}
	// 3. Execute the parameterized query.
	rs, err := querier.Query(ctx, in.SQL, in.Params)
	if err != nil {
		return Output{}, fmt.Errorf("dbquery: query failed: %w", err)
	}
	// 4. Enforce the row cap.
	limit := rowLimit(in.MaxRows, deps.DefaultMaxRows)
	rows := rs.Rows
	truncated := false
	if len(rows) > limit {
		rows = rows[:limit]
		truncated = true
	}
	// 5. Shape rows into items[].
	items := toItems(rs.Columns, rows)
	// 6. Mask before the data leaves this package (engine guaranteed non-nil above).
	items = deps.Masking.MaskItems(items, deps.MaskPoint, in.ViewerRoles)
	return Output{
		Items: items,
		Meta:  Meta{RowCount: len(items), Truncated: truncated, Columns: rs.Columns},
	}, nil
}

// rowLimit picks the effective cap: request override, else Deps default, else 50.
func rowLimit(reqMax, depsMax int) int {
	if reqMax > 0 {
		return reqMax
	}
	if depsMax > 0 {
		return depsMax
	}
	return defaultMaxRows
}

// toItems maps column-oriented rows into []map[string]any (the node data contract).
func toItems(cols []string, rows [][]any) []map[string]any {
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if i < len(row) {
				m[c] = row[i]
			}
		}
		items = append(items, m)
	}
	return items
}
