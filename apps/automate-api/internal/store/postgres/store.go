package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/flowspec"
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

const flowSelect = `SELECT id, name, folder, status, updated_at, last_run_status, last_run_at, current_version, deleted_at FROM flows`

func scanFlow(row pgx.Row) (store.FlowSummary, error) {
	var (
		f             store.FlowSummary
		lastRunStatus *string
		lastRunAt     *string
		deletedAt     *time.Time
	)
	if err := row.Scan(&f.ID, &f.Name, &f.Folder, &f.Status, &f.UpdatedAt,
		&lastRunStatus, &lastRunAt, &f.Version, &deletedAt); err != nil {
		return store.FlowSummary{}, err
	}
	if lastRunStatus != nil && lastRunAt != nil {
		f.LastRun = &store.LastRun{Status: *lastRunStatus, At: *lastRunAt}
	}
	if deletedAt != nil {
		s := deletedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		f.DeletedAt = &s
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
	// Soft-delete filter (E3-S6): exclude deleted rows unless the caller opted in.
	if !f.IncludeDeleted {
		conds = append(conds, "deleted_at IS NULL")
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
		`SELECT node_id, node_name, node_type, status, duration_ms, input_count, output_count, error_message, input_sample, output_sample
		 FROM execution_steps WHERE execution_id = $1 ORDER BY seq ASC`, id)
	if err != nil {
		return store.Execution{}, fmt.Errorf("postgres: get execution steps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			st            store.ExecutionStep
			errMsg        *string
			inRaw, outRaw []byte
		)
		if err := rows.Scan(&st.NodeID, &st.NodeName, &st.NodeType, &st.Status,
			&st.DurationMs, &st.InputCount, &st.OutputCount, &errMsg, &inRaw, &outRaw); err != nil {
			return store.Execution{}, fmt.Errorf("postgres: scan step: %w", err)
		}
		if errMsg != nil && *errMsg != "" {
			st.Error = errMsg
		}
		if st.InputSample, err = scanSample(inRaw); err != nil {
			return store.Execution{}, fmt.Errorf("postgres: scan step input sample: %w", err)
		}
		if st.OutputSample, err = scanSample(outRaw); err != nil {
			return store.Execution{}, fmt.Errorf("postgres: scan step output sample: %w", err)
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

// newID returns a short random id with the given prefix (e.g. "exe_1a2b...").
// crypto/rand is used so ids are unguessable; 8 bytes is plenty for uniqueness
// at control-plane volumes.
func newID(prefix string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

// nowISO renders the current instant as a millisecond ISO-8601 string, matching
// the text timestamps used throughout the read model.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00")
}

// GetFlowDefinition returns the flow's runnable graph. It maps both an unknown
// flow and a NULL definition to store.NewNotFound so the handler 404s uniformly.
func (s *Store) GetFlowDefinition(ctx context.Context, id string) (flowspec.FlowDef, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT definition FROM flows WHERE id = $1`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return flowspec.FlowDef{}, store.NewNotFound("flow not found: " + id)
	}
	if err != nil {
		return flowspec.FlowDef{}, fmt.Errorf("postgres: get flow definition: %w", err)
	}
	if len(raw) == 0 {
		return flowspec.FlowDef{}, store.NewNotFound("flow has no definition: " + id)
	}
	var def flowspec.FlowDef
	if err := json.Unmarshal(raw, &def); err != nil {
		return flowspec.FlowDef{}, fmt.Errorf("postgres: unmarshal flow definition: %w", err)
	}
	return def, nil
}

// CreateExecution inserts a new execution row. Steps are inserted later by
// FinishExecution once the workflow completes.
func (s *Store) CreateExecution(ctx context.Context, e store.Execution) error {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO executions (id, flow_id, flow_name, status, trigger_type, started_at, duration_ms, version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		e.ID, e.FlowID, e.FlowName, e.Status, e.Trigger, e.StartedAt, e.DurationMs, e.Version); err != nil {
		return fmt.Errorf("postgres: create execution: %w", err)
	}
	return nil
}

// FinishExecution updates the execution's terminal status+duration and inserts
// its ordered steps in a single transaction. It also mirrors the outcome onto
// the parent flow's last_run_status/last_run_at so the /flows list reflects it.
func (s *Store) FinishExecution(ctx context.Context, id, status string, durationMs int64, steps []store.ExecutionStep) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: finish execution begin: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best-effort rollback on early return

	tag, err := tx.Exec(ctx,
		`UPDATE executions SET status = $2, duration_ms = $3 WHERE id = $1`,
		id, status, durationMs)
	if err != nil {
		return fmt.Errorf("postgres: finish execution update: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("execution not found: " + id)
	}

	for seq, st := range steps {
		var errMsg *string
		if st.Error != nil && *st.Error != "" {
			errMsg = st.Error
		}
		// I/O snapshots (E5-S3): marshal the already-masked, already-truncated
		// samples to JSONB, or NULL when the step carries none. The items are
		// masked by the executors before they reach the store, so nothing here
		// re-masks them.
		inSample, err := sampleJSON(st.InputSample)
		if err != nil {
			return fmt.Errorf("postgres: finish execution marshal input sample step %d: %w", seq, err)
		}
		outSample, err := sampleJSON(st.OutputSample)
		if err != nil {
			return fmt.Errorf("postgres: finish execution marshal output sample step %d: %w", seq, err)
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO execution_steps
			 (execution_id, seq, node_id, node_name, node_type, status, duration_ms, input_count, output_count, error_message, input_sample, output_sample)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
			id, seq, st.NodeID, st.NodeName, st.NodeType, st.Status,
			st.DurationMs, st.InputCount, st.OutputCount, errMsg, inSample, outSample); err != nil {
			return fmt.Errorf("postgres: finish execution insert step %d: %w", seq, err)
		}
	}

	// Reflect the run outcome on the parent flow (best-effort; ignore no-op).
	if _, err := tx.Exec(ctx,
		`UPDATE flows SET last_run_status = $2, last_run_at = $3
		 WHERE id = (SELECT flow_id FROM executions WHERE id = $1)`,
		id, status, nowISO()); err != nil {
		return fmt.Errorf("postgres: finish execution update flow: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: finish execution commit: %w", err)
	}
	return nil
}

// sampleJSON marshals an I/O sample to JSONB bytes, returning nil (→ SQL NULL)
// for an empty/absent sample so the column stays NULL rather than storing "null"
// or "[]". The pgx codec sends []byte to a JSONB parameter verbatim.
func sampleJSON(items []map[string]any) ([]byte, error) {
	if len(items) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// scanSample decodes JSONB sample bytes back into a slice; nil/empty bytes (a
// NULL column) yield a nil slice so the DTO omits the field.
func scanSample(raw []byte) ([]map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var items []map[string]any
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// SetExecutionStatus updates only an execution's status (e.g. "cancelled").
// ErrNotFound when the execution is unknown.
func (s *Store) SetExecutionStatus(ctx context.Context, id, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE executions SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return fmt.Errorf("postgres: set execution status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("execution not found: " + id)
	}
	return nil
}

// CreateFlow inserts a new draft flow (version 0, no definition) and returns it.
func (s *Store) CreateFlow(ctx context.Context, name, folder string) (store.FlowSummary, error) {
	f := store.FlowSummary{
		ID:        newID("flw_"),
		Name:      name,
		Folder:    folder,
		Status:    "draft",
		Version:   0,
		UpdatedAt: nowISO(),
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO flows (id, name, folder, status, current_version, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		f.ID, f.Name, f.Folder, f.Status, f.Version, f.UpdatedAt); err != nil {
		return store.FlowSummary{}, fmt.Errorf("postgres: create flow: %w", err)
	}
	return f, nil
}

// UpdateFlowDefinition saves the draft graph on an existing flow and touches
// updated_at. ErrNotFound when the flow is unknown.
func (s *Store) UpdateFlowDefinition(ctx context.Context, id string, def flowspec.FlowDef) error {
	raw, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("postgres: marshal flow definition: %w", err)
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE flows SET definition = $2, updated_at = $3 WHERE id = $1`,
		id, raw, nowISO())
	if err != nil {
		return fmt.Errorf("postgres: update flow definition: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("flow not found: " + id)
	}
	return nil
}

// PublishFlow bumps current_version and sets status="published", returning the
// updated summary. ErrNotFound when the flow is unknown.
func (s *Store) PublishFlow(ctx context.Context, id string) (store.FlowSummary, error) {
	f, err := scanFlow(s.pool.QueryRow(ctx,
		`UPDATE flows SET status = 'published', current_version = current_version + 1, updated_at = $2
		 WHERE id = $1
		 RETURNING id, name, folder, status, updated_at, last_run_status, last_run_at, current_version, deleted_at`,
		id, nowISO()))
	if errors.Is(err, pgx.ErrNoRows) {
		return store.FlowSummary{}, store.NewNotFound("flow not found: " + id)
	}
	if err != nil {
		return store.FlowSummary{}, fmt.Errorf("postgres: publish flow: %w", err)
	}
	return f, nil
}

// SetFlowStatus updates only a flow's lifecycle status (pause/resume/stop) and
// touches updated_at. ErrNotFound when the flow is unknown.
func (s *Store) SetFlowStatus(ctx context.Context, id, status string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE flows SET status = $2, updated_at = $3 WHERE id = $1`,
		id, status, nowISO())
	if err != nil {
		return fmt.Errorf("postgres: set flow status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("flow not found: " + id)
	}
	return nil
}

// SoftDeleteFlow stamps deleted_at = now on the flow, leaving status untouched
// (E3-S6). ErrNotFound when the flow is unknown. It touches updated_at so the
// change surfaces in ordering. A re-delete simply re-stamps deleted_at.
func (s *Store) SoftDeleteFlow(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE flows SET deleted_at = now(), updated_at = $2 WHERE id = $1`,
		id, nowISO())
	if err != nil {
		return fmt.Errorf("postgres: soft delete flow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("flow not found: " + id)
	}
	return nil
}

// RestoreFlow clears deleted_at, returning the flow to the default list (E3-S6).
// ErrNotFound when the flow is unknown.
func (s *Store) RestoreFlow(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE flows SET deleted_at = NULL, updated_at = $2 WHERE id = $1`,
		id, nowISO())
	if err != nil {
		return fmt.Errorf("postgres: restore flow: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("flow not found: " + id)
	}
	return nil
}

// ListGrants returns a flow's object-level access grants (E2-S3), ordered by id.
func (s *Store) ListGrants(ctx context.Context, flowID string) ([]store.Grant, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, flow_id, subject_type, subject_id, access
		 FROM flow_grants WHERE flow_id = $1 ORDER BY id ASC`, flowID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list grants: %w", err)
	}
	defer rows.Close()

	out := make([]store.Grant, 0)
	for rows.Next() {
		var g store.Grant
		if err := rows.Scan(&g.ID, &g.FlowID, &g.SubjectType, &g.SubjectID, &g.Access); err != nil {
			return nil, fmt.Errorf("postgres: scan grant: %w", err)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list grants rows: %w", err)
	}
	return out, nil
}

// AddGrant inserts a grant, upserting the access level on the unique
// (flow_id, subject_type, subject_id) key so re-granting a subject updates rather
// than duplicates. It returns the grant with its assigned id.
func (s *Store) AddGrant(ctx context.Context, flowID string, in store.GrantInput) (store.Grant, error) {
	g := store.Grant{FlowID: flowID, SubjectType: in.SubjectType, SubjectID: in.SubjectID, Access: in.Access}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO flow_grants (flow_id, subject_type, subject_id, access)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (flow_id, subject_type, subject_id) DO UPDATE SET access = EXCLUDED.access
		 RETURNING id`,
		flowID, in.SubjectType, in.SubjectID, in.Access).Scan(&g.ID)
	if err != nil {
		return store.Grant{}, fmt.Errorf("postgres: add grant: %w", err)
	}
	return g, nil
}

// DeleteGrant removes one grant by id, scoped to flowID. ErrNotFound when no such
// grant exists on that flow.
func (s *Store) DeleteGrant(ctx context.Context, flowID string, grantID int64) error {
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM flow_grants WHERE id = $1 AND flow_id = $2`, grantID, flowID)
	if err != nil {
		return fmt.Errorf("postgres: delete grant: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound(fmt.Sprintf("grant not found: %d on flow %s", grantID, flowID))
	}
	return nil
}

// CreateVersion snapshots def as an immutable flow_versions row. The (flow_id,
// version_no) pair is unique — a duplicate version_no violates that constraint
// and surfaces as an error.
func (s *Store) CreateVersion(ctx context.Context, flowID string, versionNo int, def flowspec.FlowDef, changeNote, publishedBy string) error {
	raw, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("postgres: marshal version definition: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO flow_versions (id, flow_id, version_no, definition, change_note, published_by, published_at)
		 VALUES (DEFAULT, $1, $2, $3, $4, $5, now())`,
		flowID, versionNo, raw, changeNote, publishedBy); err != nil {
		return fmt.Errorf("postgres: create version: %w", err)
	}
	return nil
}

// ListVersions returns a flow's version metadata, newest (highest version_no)
// first. published_at is rendered as a millisecond ISO-8601 string.
func (s *Store) ListVersions(ctx context.Context, flowID string) ([]store.Version, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT version_no, change_note, COALESCE(published_by, ''), published_at
		 FROM flow_versions WHERE flow_id = $1 ORDER BY version_no DESC`, flowID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list versions: %w", err)
	}
	defer rows.Close()

	out := make([]store.Version, 0)
	for rows.Next() {
		var (
			v  store.Version
			ts time.Time
		)
		if err := rows.Scan(&v.VersionNo, &v.ChangeNote, &v.PublishedBy, &ts); err != nil {
			return nil, fmt.Errorf("postgres: scan version: %w", err)
		}
		v.PublishedAt = ts.UTC().Format("2006-01-02T15:04:05.000Z07:00")
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list versions rows: %w", err)
	}
	return out, nil
}

// GetVersionDefinition loads the pinned definition of one version. ErrNotFound
// when the (flow, versionNo) pair is unknown.
func (s *Store) GetVersionDefinition(ctx context.Context, flowID string, versionNo int) (flowspec.FlowDef, error) {
	var raw []byte
	err := s.pool.QueryRow(ctx,
		`SELECT definition FROM flow_versions WHERE flow_id = $1 AND version_no = $2`,
		flowID, versionNo).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return flowspec.FlowDef{}, store.NewNotFound(fmt.Sprintf("version not found: %s v%d", flowID, versionNo))
	}
	if err != nil {
		return flowspec.FlowDef{}, fmt.Errorf("postgres: get version definition: %w", err)
	}
	var def flowspec.FlowDef
	if err := json.Unmarshal(raw, &def); err != nil {
		return flowspec.FlowDef{}, fmt.Errorf("postgres: unmarshal version definition: %w", err)
	}
	return def, nil
}

// CreateConnection inserts a new connection (status "untested") and returns it.
func (s *Store) CreateConnection(ctx context.Context, in store.ConnectionInput) (store.Connection, error) {
	c := store.Connection{
		ID:           newID("conn_"),
		Name:         in.Name,
		Type:         in.Type,
		Host:         in.Host,
		Status:       "untested",
		AllowedRoles: in.AllowedRoles,
	}
	if c.AllowedRoles == nil {
		c.AllowedRoles = []string{}
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO connections (id, name, type, host, status, allowed_roles)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		c.ID, c.Name, c.Type, c.Host, c.Status, c.AllowedRoles); err != nil {
		return store.Connection{}, fmt.Errorf("postgres: create connection: %w", err)
	}
	return c, nil
}

// UpdateConnection replaces a connection's mutable fields and returns the row.
// ErrNotFound when the id is unknown.
func (s *Store) UpdateConnection(ctx context.Context, id string, in store.ConnectionInput) (store.Connection, error) {
	roles := in.AllowedRoles
	if roles == nil {
		roles = []string{}
	}
	var c store.Connection
	err := s.pool.QueryRow(ctx,
		`UPDATE connections SET name = $2, type = $3, host = $4, allowed_roles = $5
		 WHERE id = $1
		 RETURNING id, name, type, host, status, allowed_roles`,
		id, in.Name, in.Type, in.Host, roles).
		Scan(&c.ID, &c.Name, &c.Type, &c.Host, &c.Status, &c.AllowedRoles)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.Connection{}, store.NewNotFound("connection not found: " + id)
	}
	if err != nil {
		return store.Connection{}, fmt.Errorf("postgres: update connection: %w", err)
	}
	if c.AllowedRoles == nil {
		c.AllowedRoles = []string{}
	}
	return c, nil
}

// DeleteConnection removes a connection. ErrNotFound when the id is unknown.
func (s *Store) DeleteConnection(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM connections WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete connection: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return store.NewNotFound("connection not found: " + id)
	}
	return nil
}
