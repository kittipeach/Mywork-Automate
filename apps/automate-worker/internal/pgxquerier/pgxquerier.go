// Package pgxquerier is the real-Postgres implementation of dbquery.Querier
// (FR-DB-007 statement timeout, FR-DB-010 connection pool + concurrency cap).
//
// It is a thin adapter over pgxpool and is exercised by an integration test
// (build tag `integration`) against a live database — not by unit tests — so it
// is excluded from the unit-coverage gate, like the cmd/ entrypoints.
package pgxquerier

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
)

// Querier runs validated SELECTs against a pgx connection pool. Concurrency is
// bounded by the pool's MaxConns; each query is capped by stmtTimeout.
type Querier struct {
	pool        *pgxpool.Pool
	stmtTimeout time.Duration
}

// New builds a Querier over pool. stmtTimeout <= 0 disables the per-statement
// timeout (the caller's context still applies).
func New(pool *pgxpool.Pool, stmtTimeout time.Duration) *Querier {
	return &Querier{pool: pool, stmtTimeout: stmtTimeout}
}

// Query implements dbquery.Querier. The sql passed here has already been
// accepted by sqlguard; args are bound as $n parameters (never interpolated).
func (q *Querier) Query(ctx context.Context, sql string, args []any) (dbquery.RowSet, error) {
	if q.stmtTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, q.stmtTimeout)
		defer cancel()
	}
	rows, err := q.pool.Query(ctx, sql, args...)
	if err != nil {
		return dbquery.RowSet{}, fmt.Errorf("pgxquerier: query: %w", err)
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = fd.Name
	}

	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return dbquery.RowSet{}, fmt.Errorf("pgxquerier: read row: %w", err)
		}
		out = append(out, vals)
	}
	if err := rows.Err(); err != nil {
		return dbquery.RowSet{}, fmt.Errorf("pgxquerier: rows: %w", err)
	}
	return dbquery.RowSet{Columns: cols, Rows: out}, nil
}
