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
	Nodes []NodeDef `json:"nodes"`
	Edges []EdgeDef `json:"edges"`
}

// NodeDef is one graph node; Config is the node's opaque, type-specific config.
type NodeDef struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Config json.RawMessage `json:"config"`
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
