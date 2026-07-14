package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/flowspec"
)

// newExecID returns a fresh execution id (exe_<random hex>). crypto/rand keeps
// ids unguessable; 8 bytes is ample at control-plane volumes.
func newExecID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "exe_" + hex.EncodeToString(b[:])
}

// runFlow → POST /flows/{id}/run. It loads the flow definition (404 if none),
// records a "running" execution, then hands the run to the Runner. The onDone
// callback maps the FlowResult into ExecutionStep rows and calls FinishExecution
// (status "success", or "failed" carrying the error). Returns 202 with the new
// execution id. If no Runner is configured (Temporal unavailable) it 503s.
func (h *handlers) runFlow(c *gin.Context) {
	if h.runner == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "runner_unavailable",
			"flow runner is not available (temporal unreachable)")
		return
	}

	var body struct {
		Params map[string]any `json:"params"`
	}
	// A missing/empty body is fine (params optional); only reject malformed JSON.
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
	}

	id := c.Param("id")
	ctx := c.Request.Context()

	def, err := h.store.GetFlowDefinition(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	flow, err := h.store.GetFlow(ctx, id)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	role := c.GetHeader(roleHeader)
	if role == "" {
		role = defaultRole
	}

	startedAt := time.Now()
	execID := newExecID()
	exec := store.Execution{
		ID:        execID,
		FlowID:    flow.ID,
		FlowName:  flow.Name,
		Status:    "running",
		Trigger:   "manual",
		StartedAt: startedAt.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Version:   flow.Version,
	}
	if err := h.store.CreateExecution(ctx, exec); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	in := flowspec.FlowInput{
		Flow:        def,
		ViewerRoles: []string{role},
		Params:      body.Params,
	}

	// onDone records the terminal outcome. It runs in the caller's goroutine for
	// the synchronous fake (tests) and on a background goroutine for the real
	// Temporal runner, so it uses a detached context and never touches gin.
	onDone := func(res flowspec.FlowResult, runErr error) {
		durationMs := time.Since(startedAt).Milliseconds()
		steps := buildSteps(def, res, runErr)
		status := "success"
		if runErr != nil {
			status = "failed"
		}
		_ = h.store.FinishExecution(context.Background(), execID, status, durationMs, steps)
	}

	if err := h.runner.Run(ctx, execID, in, onDone); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"executionId": execID})
}

// buildSteps maps a FlowResult into execution steps. Each executed node in
// result.Path becomes a "success" step; outputCount comes from the node's
// Meta["rowCount"] if present, else len(Items). On a run error the final step
// (the last node that ran, or a synthetic one when nothing ran) is marked
// "failed" and carries the error message.
func buildSteps(def flowspec.FlowDef, res flowspec.FlowResult, runErr error) []store.ExecutionStep {
	steps := make([]store.ExecutionStep, 0, len(res.Path))
	for _, nodeID := range res.Path {
		out := res.Outputs[nodeID]
		steps = append(steps, store.ExecutionStep{
			NodeID:      nodeID,
			NodeName:    nodeName(def, nodeID),
			NodeType:    nodeType(def, nodeID),
			Status:      "success",
			OutputCount: outputCount(out),
		})
	}

	if runErr != nil {
		msg := runErr.Error()
		if len(steps) > 0 {
			last := &steps[len(steps)-1]
			last.Status = "failed"
			last.Error = &msg
		} else {
			// Nothing ran (e.g. trigger resolution failed): synthesise one failed
			// step so the execution detail still explains why it failed.
			steps = append(steps, store.ExecutionStep{
				NodeID:   "flow",
				NodeName: "flow",
				NodeType: "flow",
				Status:   "failed",
				Error:    &msg,
			})
		}
	}
	return steps
}

// outputCount prefers the node's reported rowCount metadata, falling back to the
// number of items it emitted.
func outputCount(out flowspec.NodeOutput) int64 {
	if out.Meta != nil {
		if rc, ok := out.Meta["rowCount"]; ok {
			if n, ok := toInt64(rc); ok {
				return n
			}
		}
	}
	return int64(len(out.Items))
}

// toInt64 coerces a JSON-decoded number (which may arrive as float64, json
// numbers, or ints depending on the source) into an int64.
func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func nodeName(def flowspec.FlowDef, id string) string {
	for _, n := range def.Nodes {
		if n.ID == id {
			if n.Name != "" {
				return n.Name
			}
			return id
		}
	}
	return id
}

func nodeType(def flowspec.FlowDef, id string) string {
	for _, n := range def.Nodes {
		if n.ID == id {
			return n.Type
		}
	}
	return ""
}

// createFlow → POST /flows {name, folder} → 201 the new draft flow.
func (h *handlers) createFlow(c *gin.Context) {
	var body struct {
		Name   string `json:"name"`
		Folder string `json:"folder"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Name == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	flow, err := h.store.CreateFlow(c.Request.Context(), body.Name, body.Folder)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusCreated, flow)
}

// updateFlowDraft → PUT /flows/{id}/draft {definition} → 200.
func (h *handlers) updateFlowDraft(c *gin.Context) {
	var body struct {
		Definition flowspec.FlowDef `json:"definition"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	err := h.store.UpdateFlowDefinition(c.Request.Context(), c.Param("id"), body.Definition)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// publishFlow → POST /flows/{id}/publish → 200 the published flow.
func (h *handlers) publishFlow(c *gin.Context) {
	flow, err := h.store.PublishFlow(c.Request.Context(), c.Param("id"))
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, flow)
}

// createConnection → POST /connections → 201 the new connection.
func (h *handlers) createConnection(c *gin.Context) {
	var in store.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if in.Name == "" || in.Type == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "name and type are required")
		return
	}
	conn, err := h.store.CreateConnection(c.Request.Context(), in)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusCreated, conn)
}

// updateConnection → PUT /connections/{id} → 200 the updated connection.
func (h *handlers) updateConnection(c *gin.Context) {
	var in store.ConnectionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	conn, err := h.store.UpdateConnection(c.Request.Context(), c.Param("id"), in)
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, conn)
}

// deleteConnection → DELETE /connections/{id} → 204.
func (h *handlers) deleteConnection(c *gin.Context) {
	err := h.store.DeleteConnection(c.Request.Context(), c.Param("id"))
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// testConnection → POST /connections/{id}/test. A stub for E-phase: returns a
// canned ok result (a live probe lands with the secrets resolver later).
func (h *handlers) testConnection(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
