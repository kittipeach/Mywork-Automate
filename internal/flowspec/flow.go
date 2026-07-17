// Package flow holds the flow-definition and workflow I/O types shared between
// the control-plane API (which stores definitions and starts runs) and the
// worker's interpreter (which executes them). Temporal passes these as JSON, so
// the JSON tags here MUST match the interpreter's types byte-for-byte; the API
// starts the workflow by its registered name (WorkflowName) rather than by the
// Go function, so it never imports the worker's internal packages.
package flowspec

import "encoding/json"

const (
	// TaskQueue is the Temporal task queue the worker serves.
	TaskQueue = "automate-task-queue"
	// Namespace is the dedicated Temporal namespace for automate.
	Namespace = "automate"
	// WorkflowName is the registered name of the interpreter's FlowWorkflow.
	WorkflowName = "FlowWorkflow"
)

// FlowDef is the flow-definition graph (docs/spec/06 §1).
type FlowDef struct {
	Nodes    []NodeDef `json:"nodes"`
	Edges    []EdgeDef `json:"edges"`
	Settings *Settings `json:"settings,omitempty"`
}

// Settings holds flow-level configuration (docs/spec/06: settings.notification).
type Settings struct {
	Notification *NotificationSettings `json:"notification,omitempty"`
}

// NotificationSettings configures run-failure alerts (E5-S6).
type NotificationSettings struct {
	FailureEmails []string `json:"failureEmails,omitempty"`
}

// NodeDef is one graph node; Config is the node's opaque, type-specific config.
// OnError controls how the interpreter reacts when the node's activity fails
// ("fail" = fail the run (default), "continue" = record the error and carry on,
// "errorBranch" = follow the node's "error"-labelled out-edge). Retry is the
// per-node retry policy applied to the node's activity (FR-LOGIC-008).
type NodeDef struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Config  json.RawMessage `json:"config"`
	OnError string          `json:"onError,omitempty"` // "fail"|"continue"|"errorBranch"
	Retry   *RetryPolicy    `json:"retry,omitempty"`
}

// RetryPolicy is a node's activity retry configuration.
type RetryPolicy struct {
	MaxAttempts            int `json:"maxAttempts,omitempty"`            // total attempts (1 = no retry)
	InitialIntervalSeconds int `json:"initialIntervalSeconds,omitempty"` // backoff seed
}

// EdgeDef is a directed edge; Label is "" or "true"/"false" for logic.if.
type EdgeDef struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// FlowInput starts a run: the definition, the viewer roles (for masking) and
// optional trigger params.
type FlowInput struct {
	Flow        FlowDef        `json:"flow"`
	ViewerRoles []string       `json:"viewerRoles"`
	Params      map[string]any `json:"params"`
}

// NodeOutput is one node's produced items plus metadata.
type NodeOutput struct {
	Items []map[string]any `json:"items"`
	Meta  map[string]any   `json:"meta"`
}

// FlowResult is the workflow result: node ids in execution order + each node's output.
type FlowResult struct {
	Path    []string              `json:"path"`
	Outputs map[string]NodeOutput `json:"outputs"`
}
