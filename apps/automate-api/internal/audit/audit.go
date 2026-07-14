// Package audit is the append-only audit trail for automate-api (docs/spec/07
// §6). It records every security-relevant action — who did what, to which
// resource, from which ip/user-agent, and when — for compliance and SIEM export.
//
// The package defines the storage-agnostic Service contract plus a Noop
// implementation used when auditing is disabled or no database is configured.
// The concrete pgx-backed Service lives in the postgres sub-package so it can be
// integration-tagged and excluded from the unit-coverage gate.
package audit

import "context"

// Entry is a single audit record. JSON tags match the /audit-logs response
// shape (camelCase). Optional string fields are omitted when empty; Role and
// Action are always present. CreatedAt is an RFC3339 timestamp with millisecond
// precision (assigned by the store on Record).
type Entry struct {
	ID           int64          `json:"id"`
	UserID       string         `json:"userId,omitempty"`
	Role         string         `json:"role"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType,omitempty"`
	ResourceID   string         `json:"resourceId,omitempty"`
	Detail       map[string]any `json:"detail,omitempty"`
	IP           string         `json:"ip,omitempty"`
	UserAgent    string         `json:"userAgent,omitempty"`
	CreatedAt    string         `json:"createdAt"`
}

// Filter narrows a List query. Action is an exact match; From/To bound
// created_at (inclusive) as RFC3339 strings; Limit caps the row count. Empty
// fields are ignored. Limit is clamped by ClampLimit.
type Filter struct {
	Action string
	From   string
	To     string
	Limit  int
}

// Service records and lists audit entries. Record is append-only: it never
// mutates or removes existing entries. Implementations must be safe for
// concurrent use.
type Service interface {
	// Record appends one audit entry. The store assigns ID and CreatedAt.
	Record(ctx context.Context, e Entry) error
	// List returns entries matching f, most-recent first. It never returns a
	// nil slice — an empty result is an empty, non-nil slice.
	List(ctx context.Context, f Filter) ([]Entry, error)
}

// Limit bounds for List queries. DefaultLimit applies when Filter.Limit <= 0;
// MaxLimit is the hard cap regardless of the request.
const (
	DefaultLimit = 200
	MaxLimit     = 1000
)

// ClampLimit normalises a requested limit: non-positive requests fall back to
// DefaultLimit, and anything above MaxLimit is capped at MaxLimit. It is the
// single source of truth for the page-size policy shared by every Service.
func ClampLimit(requested int) int {
	if requested <= 0 {
		return DefaultLimit
	}
	if requested > MaxLimit {
		return MaxLimit
	}
	return requested
}

// Action constants enumerate the audited events from docs/spec/07 §6. Handlers
// pass these to Record so action strings stay consistent across the codebase and
// match the index/read paths.
const (
	ActionAuthLogin  = "auth.login"
	ActionAuthLogout = "auth.logout"
	ActionAuthFailed = "auth.failed"

	ActionFlowCreate   = "flow.create"
	ActionFlowUpdate   = "flow.update"
	ActionFlowPublish  = "flow.publish"
	ActionFlowPause    = "flow.pause"
	ActionFlowResume   = "flow.resume"
	ActionFlowStop     = "flow.stop"
	ActionFlowDelete   = "flow.delete"
	ActionFlowRollback = "flow.rollback"

	ActionConnectionCreate = "connection.create"
	ActionConnectionUpdate = "connection.update"
	ActionConnectionDelete = "connection.delete"
	ActionConnectionTest   = "connection.test"

	ActionExecutionManualRun = "execution.manual_run"
	ActionExecutionCancel    = "execution.cancel"
	ActionExecutionRetry     = "execution.retry"

	ActionFileDownload = "file.download"

	ActionMaskingRuleChange = "masking.rule_change"
	ActionMaskingBypassView = "masking.bypass_view"

	ActionRBACGrantChange = "rbac.grant_change"

	ActionAICall = "ai.call"
)

// noop is a Service that discards records and returns no entries. It is used
// when auditing is disabled or no database is wired in, so callers can always
// Record unconditionally without nil checks.
type noop struct{}

// NewNoop returns a Service whose Record is a no-op and whose List returns an
// empty (non-nil) slice.
func NewNoop() Service { return noop{} }

func (noop) Record(context.Context, Entry) error { return nil }

func (noop) List(context.Context, Filter) ([]Entry, error) { return []Entry{}, nil }
