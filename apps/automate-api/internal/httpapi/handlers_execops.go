package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/preview"
)

// terminalStatuses are the execution states that cannot be cancelled or that
// already represent a finished run.
var terminalStatuses = map[string]bool{
	"success":   true,
	"failed":    true,
	"cancelled": true,
}

// cancelExecution → POST /executions/{id}/cancel. It loads the execution (404 if
// missing), rejects a terminal execution with 409, otherwise asks the Runner to
// cancel the in-flight workflow and flips the stored status to "cancelled".
// Requires FlowRun. Returns 200 {status:"cancelled"} and audits execution.cancel.
// 503 when no Runner is configured (Temporal unavailable).
func (h *handlers) cancelExecution(c *gin.Context) {
	if h.runner == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "runner_unavailable",
			"flow runner is not available (temporal unreachable)")
		return
	}

	id := c.Param("id")
	ctx := c.Request.Context()

	exec, err := h.store.GetExecution(ctx, id)
	if err != nil {
		h.writeStoreError(c, err)
		return
	}
	if terminalStatuses[exec.Status] {
		errorEnvelope(c, http.StatusConflict, "conflict",
			"execution is already in a terminal state: "+exec.Status)
		return
	}

	if err := h.runner.Cancel(ctx, id); err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	if err := h.store.SetExecutionStatus(ctx, id, "cancelled"); err != nil {
		h.writeStoreError(c, err)
		return
	}

	h.record(c, audit.Entry{
		Action:       audit.ActionExecutionCancel,
		ResourceType: "execution",
		ResourceID:   id,
	})

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// retryExecution → POST /executions/{id}/retry. It loads the source execution
// (404 if missing) to find its flow, loads that flow's current definition and
// summary, then starts a brand-new execution using the shared run-and-record
// path (create "running" → Runner.Run → onDone finalises). Requires FlowRun.
// Returns 202 {executionId} for the new run and audits execution.retry. 503 when
// no Runner is configured.
func (h *handlers) retryExecution(c *gin.Context) {
	if h.runner == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "runner_unavailable",
			"flow runner is not available (temporal unreachable)")
		return
	}

	id := c.Param("id")
	ctx := c.Request.Context()

	src, err := h.store.GetExecution(ctx, id)
	if err != nil {
		h.writeStoreError(c, err)
		return
	}

	def, flow, ok := h.loadRunnable(c, src.FlowID)
	if !ok {
		return
	}

	// Retry re-runs with no params (the original params are not persisted on the
	// execution row); the run reuses the flow's current definition.
	execID, ok := h.startRun(c, def, flow, nil)
	if !ok {
		return
	}

	h.record(c, audit.Entry{
		Action:       audit.ActionExecutionRetry,
		ResourceType: "execution",
		ResourceID:   id,
		Detail:       map[string]any{"executionId": execID, "flowId": flow.ID},
	})

	c.JSON(http.StatusAccepted, gin.H{"executionId": execID})
}

// queryPreview → POST /connections/{id}/query-preview {sql, maxRows?}. It
// validates the SQL (SELECT-only via sqlguard), runs it against the injected
// querier, caps the result, maps rows to objects and masks them at the preview
// point for the caller's role. Requires FlowView. Returns 200
// {items, meta:{columns, rowCount, truncated}}. A sqlguard rejection is a 400
// with code "invalid_sql" (preserving the sentinel message); no querier
// configured is a 503.
func (h *handlers) queryPreview(c *gin.Context) {
	if h.querier == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "preview_unavailable",
			"query preview is not available (no database pool configured)")
		return
	}

	var body struct {
		SQL     string `json:"sql"`
		MaxRows int    `json:"maxRows"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	role := string(callerRole(c))
	res, err := preview.Query(c.Request.Context(), h.querier, h.mask, body.SQL, body.MaxRows, []string{role})
	if err != nil {
		if errors.Is(err, preview.ErrInvalidSQL) {
			errorEnvelope(c, http.StatusBadRequest, "invalid_sql", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"items": res.Items, "meta": res.Meta})
}

// connectionSchema → GET /connections/{id}/schema. It introspects the public
// base tables and columns via the injected querier and returns
// {tables:[{name, columns:[{name,type}]}]}. Requires FlowView. 503 when no
// querier is configured.
func (h *handlers) connectionSchema(c *gin.Context) {
	if h.querier == nil {
		errorEnvelope(c, http.StatusServiceUnavailable, "preview_unavailable",
			"schema introspection is not available (no database pool configured)")
		return
	}

	sch, err := preview.FetchSchema(c.Request.Context(), h.querier)
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, sch)
}
