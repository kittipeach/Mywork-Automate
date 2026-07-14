package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/store"
)

// Store is the pgx-backed implementation of store.Store. All queries are
// parameterised ($n) — no interpolation.
type Store struct {
	pool *pgxpool.Pool
}

// New builds a Store over pool.
func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// compile-time check that *Store satisfies the interface.
var _ store.Store = (*Store)(nil)

const flowSelect = `SELECT id, name, folder, status, updated_at, last_run_status, last_run_at, current_version FROM flows`

func scanFlow(row pgx.Row) (store.FlowSummary, error) {
	var (
		f             store.FlowSummary
		lastRunStatus *string
		lastRunAt     *string
	)
	if err := row.Scan(&f.ID, &f.Name, &f.Folder, &f.Status, &f.UpdatedAt,
		&lastRunStatus, &lastRunAt, &f.Version); err != nil {
		return store.FlowSummary{}, err
	}
	if lastRunStatus != nil && lastRunAt != nil {
		f.LastRun = &store.LastRun{Status: *lastRunStatus, At: *lastRunAt}
	}
	return f, nil
}

// ListFlows returns flows matching the filter, ordered by updated_at DESC.
// Q is a case-insensitive substring match on name; Status/Folder are exact.
func (s *Store) ListFlows(ctx context.Context, f store.FlowFilter) ([]store.FlowSummary, error) {
	var (
		conds []string
		args  []any
	)
	if f.Q != "" {
		args = append(args, "%"+strings.ToLower(f.Q)+"%")
		conds = append(conds, fmt.Sprintf("lower(name) LIKE $%d", len(args)))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.Folder != "" {
		args = append(args, f.Folder)
		conds = append(conds, fmt.Sprintf("folder = $%d", len(args)))
	}

	q := flowSelect
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY updated_at DESC, id ASC"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list flows: %w", err)
	}
	defer rows.Close()

	out := make([]store.FlowSummary, 0)
	for rows.Next() {
		f, err := scanFlow(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan flow: %w", err)
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list flows rows: %w", err)
	}
	return out, nil
}

// GetFlow returns a single flow or store.NewNotFound if the id is unknown.
func (s *Store) GetFlow(ctx context.Context, id string) (store.FlowSummary, error) {
	f, err := scanFlow(s.pool.QueryRow(ctx, flowSelect+" WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.FlowSummary{}, store.NewNotFound("flow not found: " + id)
	}
	if err != nil {
		return store.FlowSummary{}, fmt.Errorf("postgres: get flow: %w", err)
	}
	return f, nil
}

const execSelect = `SELECT id, flow_id, flow_name, status, trigger_type, started_at, duration_ms, version FROM executions`

func scanExec(row pgx.Row) (store.Execution, error) {
	var e store.Execution
	if err := row.Scan(&e.ID, &e.FlowID, &e.FlowName, &e.Status, &e.Trigger,
		&e.StartedAt, &e.DurationMs, &e.Version); err != nil {
		return store.Execution{}, err
	}
	e.Steps = []store.ExecutionStep{}
	return e, nil
}

// ListExecutions returns executions matching the filter (Status/FlowID exact),
// ordered by started_at DESC. Steps are not loaded for the list view.
func (s *Store) ListExecutions(ctx context.Context, f store.ExecutionFilter) ([]store.Execution, error) {
	var (
		conds []string
		args  []any
	)
	if f.Status != "" {
		args = append(args, f.Status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if f.FlowID != "" {
		args = append(args, f.FlowID)
		conds = append(conds, fmt.Sprintf("flow_id = $%d", len(args)))
	}

	q := execSelect
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY started_at DESC, id ASC"

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list executions: %w", err)
	}
	defer rows.Close()

	out := make([]store.Execution, 0)
	for rows.Next() {
		e, err := scanExec(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan execution: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list executions rows: %w", err)
	}
	return out, nil
}

// GetExecution returns one execution with its ordered steps, or
// store.NewNotFound if the id is unknown.
func (s *Store) GetExecution(ctx context.Context, id string) (store.Execution, error) {
	e, err := scanExec(s.pool.QueryRow(ctx, execSelect+" WHERE id = $1", id))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Execution{}, store.NewNotFound("execution not found: " + id)
	}
	if err != nil {
		return store.Execution{}, fmt.Errorf("postgres: get execution: %w", err)
	}

	rows, err := s.pool.Query(ctx,
		`SELECT node_id, node_name, node_type, status, duration_ms, input_count, output_count, error_message
		 FROM execution_steps WHERE execution_id = $1 ORDER BY seq ASC`, id)
	if err != nil {
		return store.Execution{}, fmt.Errorf("postgres: get execution steps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			st     store.ExecutionStep
			errMsg *string
		)
		if err := rows.Scan(&st.NodeID, &st.NodeName, &st.NodeType, &st.Status,
			&st.DurationMs, &st.InputCount, &st.OutputCount, &errMsg); err != nil {
			return store.Execution{}, fmt.Errorf("postgres: scan step: %w", err)
		}
		if errMsg != nil && *errMsg != "" {
			st.Error = errMsg
		}
		e.Steps = append(e.Steps, st)
	}
	if err := rows.Err(); err != nil {
		return store.Execution{}, fmt.Errorf("postgres: execution steps rows: %w", err)
	}
	return e, nil
}

// ListConnections returns connections visible to any of roles: a connection is
// visible when its allowed_roles is empty (public) or intersects roles. Results
// are ordered by id for determinism.
func (s *Store) ListConnections(ctx context.Context, roles []string) ([]store.Connection, error) {
	if roles == nil {
		roles = []string{}
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, type, host, status, allowed_roles
		 FROM connections
		 WHERE cardinality(allowed_roles) = 0 OR allowed_roles && $1
		 ORDER BY id ASC`, roles)
	if err != nil {
		return nil, fmt.Errorf("postgres: list connections: %w", err)
	}
	defer rows.Close()

	out := make([]store.Connection, 0)
	for rows.Next() {
		var c store.Connection
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Status, &c.AllowedRoles); err != nil {
			return nil, fmt.Errorf("postgres: scan connection: %w", err)
		}
		if c.AllowedRoles == nil {
			c.AllowedRoles = []string{}
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list connections rows: %w", err)
	}
	return out, nil
}
