// Package postgres is the pgx-backed audit.Service. It is exercised by an
// integration test (build tag `integration`) against a live database, so — like
// store/postgres and the cmd/ entrypoints — it is excluded from the unit-coverage
// gate. The composition root builds a Service with New(pool) and passes it into
// handlers, which Record on each audited action; GET /audit-logs calls List.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
)

// rfc3339Millis is the timestamp format used across the read model — an ISO-8601
// instant with millisecond precision, so audit CreatedAt round-trips consistently
// with the other API responses.
const rfc3339Millis = "2006-01-02T15:04:05.000Z07:00"

// Service is the pgx-backed audit.Service. All queries are parameterised ($n) —
// no interpolation. It is append-only: it only ever INSERTs and SELECTs.
type Service struct {
	pool *pgxpool.Pool
}

// New builds a Service over pool.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// compile-time check that *Service satisfies the interface.
var _ audit.Service = (*Service)(nil)

// Record appends one audit entry. created_at is assigned by the database default
// and returned so the caller-visible CreatedAt is the authoritative server time,
// formatted as RFC3339 millis. A nil/empty Detail is stored as SQL NULL.
func (s *Service) Record(ctx context.Context, e audit.Entry) error {
	var detail []byte
	if len(e.Detail) > 0 {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("audit postgres: marshal detail: %w", err)
		}
		detail = b
	}

	var (
		id        int64
		createdAt time.Time
	)
	err := s.pool.QueryRow(ctx,
		`INSERT INTO audit_logs (user_id, role, action, resource_type, resource_id, detail, ip, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id, created_at`,
		nullIfEmpty(e.UserID), e.Role, e.Action, e.ResourceType, e.ResourceID,
		detail, e.IP, e.UserAgent).Scan(&id, &createdAt)
	if err != nil {
		return fmt.Errorf("audit postgres: record: %w", err)
	}
	return nil
}

// List returns entries matching f, most-recent first, capped by the clamped
// limit. Action is an exact match; From/To bound created_at inclusively. The
// result is always non-nil.
func (s *Service) List(ctx context.Context, f audit.Filter) ([]audit.Entry, error) {
	var (
		conds []string
		args  []any
	)
	if f.Action != "" {
		args = append(args, f.Action)
		conds = append(conds, fmt.Sprintf("action = $%d", len(args)))
	}
	if f.From != "" {
		t, err := parseTime(f.From)
		if err != nil {
			return nil, fmt.Errorf("audit postgres: parse from: %w", err)
		}
		args = append(args, t)
		conds = append(conds, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if f.To != "" {
		t, err := parseTime(f.To)
		if err != nil {
			return nil, fmt.Errorf("audit postgres: parse to: %w", err)
		}
		args = append(args, t)
		conds = append(conds, fmt.Sprintf("created_at <= $%d", len(args)))
	}

	q := `SELECT id, user_id, role, action, resource_type, resource_id, detail, ip, user_agent, created_at
	      FROM audit_logs`
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, audit.ClampLimit(f.Limit))
	q += fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d", len(args))

	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("audit postgres: list: %w", err)
	}
	defer rows.Close()

	out := make([]audit.Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("audit postgres: scan entry: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit postgres: list rows: %w", err)
	}
	return out, nil
}

func scanEntry(row pgx.Row) (audit.Entry, error) {
	var (
		e         audit.Entry
		userID    *string
		detail    []byte
		createdAt time.Time
	)
	if err := row.Scan(&e.ID, &userID, &e.Role, &e.Action, &e.ResourceType,
		&e.ResourceID, &detail, &e.IP, &e.UserAgent, &createdAt); err != nil {
		return audit.Entry{}, err
	}
	if userID != nil {
		e.UserID = *userID
	}
	if len(detail) > 0 {
		if err := json.Unmarshal(detail, &e.Detail); err != nil {
			return audit.Entry{}, fmt.Errorf("unmarshal detail: %w", err)
		}
	}
	e.CreatedAt = createdAt.UTC().Format(rfc3339Millis)
	return e, nil
}

// parseTime accepts either a full RFC3339 instant or the millis variant used by
// the read model, so From/To filters can be passed in the same shape the API
// returns CreatedAt.
func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	return time.Parse(rfc3339Millis, s)
}

// nullIfEmpty maps the empty string to a nil *string so an unknown actor is
// stored as SQL NULL (user_id is nullable) rather than "".
func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
