package preview

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolQuerier adapts a *pgxpool.Pool to the Querier seam. pgx.Rows already
// satisfies the Rows interface structurally (Next/Values/FieldDescriptions/Err/
// Close), so the adapter just forwards the call and widens the return type. This
// keeps the pgx dependency out of the pure preview/schema logic (unit-tested with
// a fake) while the real query path is integration-tested.
type PoolQuerier struct {
	Pool *pgxpool.Pool
}

// Query forwards to the pool; the concrete pgx.Rows is returned as a Rows.
func (p PoolQuerier) Query(ctx context.Context, sql string, args ...any) (Rows, error) {
	rows, err := p.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
