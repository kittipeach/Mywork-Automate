// Package store defines the control-plane data-access contract used by the
// httpapi handlers plus the DTO structs whose JSON tags are the wire contract
// with the Next.js UI (apps/automate-web/src/lib/mock/store.ts).
//
// The concrete pgx implementation lives in the sibling package
// store/postgres so it can be integration-tested against a live database and
// excluded from the unit-coverage gate (like automate-worker/pgxquerier).
package store

import (
	"context"

	"github.com/mywork/automate/internal/flowspec"
)

// LastRun mirrors the TS `lastRun?: { status, at }` optional object. It is a
// pointer field on FlowSummary so it serialises to `null`/absent when a flow
// has never run (matching the mock, which omits `lastRun` for draft flows).
type LastRun struct {
	Status string `json:"status"`
	At     string `json:"at"`
}

// FlowSummary matches the TS `FlowSummary` shape byte-for-byte.
type FlowSummary struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Folder    string   `json:"folder"`
	Status    string   `json:"status"`
	UpdatedAt string   `json:"updatedAt"`
	LastRun   *LastRun `json:"lastRun,omitempty"`
	Version   int      `json:"version"`
}

// ExecutionStep matches the TS execution `steps[]` element. Error is a pointer
// so it is omitted when empty, matching the mock's optional `error?`.
type ExecutionStep struct {
	NodeID      string  `json:"nodeId"`
	NodeName    string  `json:"nodeName"`
	NodeType    string  `json:"nodeType"`
	Status      string  `json:"status"`
	DurationMs  int64   `json:"durationMs"`
	InputCount  int64   `json:"inputCount"`
	OutputCount int64   `json:"outputCount"`
	Error       *string `json:"error,omitempty"`
}

// Execution matches the TS `Execution` shape. Note the JSON key is `trigger`
// (the DB column is trigger_type). Steps is always a non-nil slice so it
// serialises as `[]` rather than `null` for step-less executions.
type Execution struct {
	ID         string          `json:"id"`
	FlowID     string          `json:"flowId"`
	FlowName   string          `json:"flowName"`
	Status     string          `json:"status"`
	Trigger    string          `json:"trigger"`
	StartedAt  string          `json:"startedAt"`
	DurationMs int64           `json:"durationMs"`
	Version    int             `json:"version"`
	Steps      []ExecutionStep `json:"steps"`
}

// Connection matches the TS `Connection` shape. AllowedRoles is always non-nil
// so it serialises as `[]` rather than `null`.
type Connection struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Host         string   `json:"host"`
	Status       string   `json:"status"`
	AllowedRoles []string `json:"allowedRoles"`
}

// FlowFilter carries the /flows query parameters. Empty fields mean "no filter".
// Q matches a case-insensitive substring of the flow name; Status and Folder
// are exact matches (semantics copied from the Next mock route).
type FlowFilter struct {
	Q      string
	Status string
	Folder string
}

// ExecutionFilter carries the /executions query parameters. Empty fields mean
// "no filter"; both Status and FlowID are exact matches.
type ExecutionFilter struct {
	Status string
	FlowID string
}

// ErrNotFound is returned by GetFlow/GetExecution when no row matches the id.
// Handlers translate it into a 404 with the standard error envelope.
type notFoundError struct{ msg string }

func (e notFoundError) Error() string { return e.msg }

// NewNotFound builds an ErrNotFound-style error carrying a human message.
func NewNotFound(msg string) error { return notFoundError{msg: msg} }

// IsNotFound reports whether err was produced by NewNotFound.
func IsNotFound(err error) bool {
	_, ok := err.(notFoundError)
	return ok
}

// ConnectionInput carries the mutable fields when creating or updating a
// connection. AllowedRoles may be nil (treated as empty/public).
type ConnectionInput struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Host         string   `json:"host"`
	AllowedRoles []string `json:"allowedRoles"`
}

// Version is one immutable published-version snapshot's metadata (docs/spec/08
// E4-S2). The definition itself is fetched separately via GetVersionDefinition;
// this is the list-view row. JSON tags are camelCase (the /flows/{id}/versions
// wire contract).
type Version struct {
	VersionNo   int    `json:"versionNo"`
	ChangeNote  string `json:"changeNote"`
	PublishedBy string `json:"publishedBy"`
	PublishedAt string `json:"publishedAt"`
}

// Store is the contract the httpapi handlers depend on: the read endpoints plus
// the write endpoints that close the execution loop (run/record) and edit the
// flow/connection catalog. It is small and interface-based so handlers can be
// unit-tested with an in-memory fake and the pgx implementation swapped in at
// runtime.
type Store interface {
	ListFlows(ctx context.Context, f FlowFilter) ([]FlowSummary, error)
	GetFlow(ctx context.Context, id string) (FlowSummary, error)
	ListExecutions(ctx context.Context, f ExecutionFilter) ([]Execution, error)
	GetExecution(ctx context.Context, id string) (Execution, error)
	ListConnections(ctx context.Context, roles []string) ([]Connection, error)

	// GetFlowDefinition returns the flow's runnable graph, or ErrNotFound when
	// the flow is unknown or has no definition (NULL/absent).
	GetFlowDefinition(ctx context.Context, id string) (flowspec.FlowDef, error)

	// CreateExecution inserts a new execution row (status is usually "running").
	CreateExecution(ctx context.Context, e Execution) error
	// FinishExecution updates an execution's terminal status+duration and inserts
	// its ordered steps in one transaction.
	FinishExecution(ctx context.Context, id, status string, durationMs int64, steps []ExecutionStep) error

	// CreateFlow inserts a new draft flow (version 0, no definition) and returns
	// its summary.
	CreateFlow(ctx context.Context, name, folder string) (FlowSummary, error)
	// UpdateFlowDefinition saves the draft graph on an existing flow.
	UpdateFlowDefinition(ctx context.Context, id string, def flowspec.FlowDef) error
	// PublishFlow bumps current_version and sets status="published".
	PublishFlow(ctx context.Context, id string) (FlowSummary, error)

	// SetFlowStatus updates only a flow's lifecycle status (pause/resume/stop).
	// ErrNotFound when the flow is unknown.
	SetFlowStatus(ctx context.Context, id, status string) error

	// CreateVersion snapshots the given definition as an immutable version row
	// (docs/spec/08 E4-S2). versionNo must be unique per flow; changeNote is the
	// mandatory publish note and publishedBy the caller identity.
	CreateVersion(ctx context.Context, flowID string, versionNo int, def flowspec.FlowDef, changeNote, publishedBy string) error
	// ListVersions returns a flow's version metadata, newest first.
	ListVersions(ctx context.Context, flowID string) ([]Version, error)
	// GetVersionDefinition loads the pinned definition of one version. ErrNotFound
	// when the (flow, versionNo) pair is unknown.
	GetVersionDefinition(ctx context.Context, flowID string, versionNo int) (flowspec.FlowDef, error)

	// CreateConnection inserts a new connection and returns it.
	CreateConnection(ctx context.Context, in ConnectionInput) (Connection, error)
	// UpdateConnection replaces a connection's mutable fields.
	UpdateConnection(ctx context.Context, id string, in ConnectionInput) (Connection, error)
	// DeleteConnection removes a connection (ErrNotFound when unknown).
	DeleteConnection(ctx context.Context, id string) error
}
