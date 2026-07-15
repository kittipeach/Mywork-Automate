package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/nodes"
	"github.com/mywork/automate/apps/automate-api/internal/notify"
	"github.com/mywork/automate/apps/automate-api/internal/preview"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/pkg/masking"
)

// defaultRole is assumed when the caller sends no X-Role header. admin sees all
// connections, matching the mock which is effectively unfiltered.
const defaultRole = "admin"

// roleHeader is the request header carrying the caller's role for RBAC. Auth
// (E2) will populate this from the JWT; for now the UI/dev client sets it.
const roleHeader = "X-Role"

// errorEnvelope writes the standard { "error": { code, message } } body
// (docs/spec/06 §2) with the given HTTP status.
func errorEnvelope(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": gin.H{"code": code, "message": message}})
}

// handlers bundles the dependencies for the read and write endpoints: the data
// store, the flow Runner (nil when Temporal is unavailable — /run then 503s),
// the append-only audit trail (never nil — Noop when auditing is disabled) and
// the schedule Scheduler (never nil — Noop when Temporal is unavailable). log is
// used only for the "audit/scheduler failed but the request still succeeds"
// WARN path; nil is tolerated (no-op).
type handlers struct {
	store  store.Store
	runner runner.Runner
	audit  audit.Service
	sched  scheduler.Scheduler
	log    *slog.Logger

	// notifier alerts a flow's configured recipients when a run finishes
	// "failed" (E5-S6). Never nil once NewRouter has run — notify.Noop when SMTP
	// is not configured. A notify failure is non-fatal (logged WARN in onDone).
	notifier notify.Notifier

	// querier backs the query-preview and schema endpoints (E6-S4/E6-S2). It is
	// the seam over the database: in production it is the API's pgx pool (see the
	// note in the preview package on per-connection pools for production); in
	// tests it is a fake. nil when no pool is configured — the endpoints then 503.
	querier preview.Querier
	// mask is the masking engine applied to preview results at the preview point.
	// Never nil once NewRouter has built it from masking.DefaultRules().
	mask *masking.Engine
}

// record appends one audit entry after a mutating action has already succeeded.
// An audit failure must never break the request (docs/spec/07 §6): it is logged
// at WARN and the caller proceeds. c is used for the request-scoped context, the
// client IP and the user-agent; role/action/resource are supplied by the caller.
func (h *handlers) record(c *gin.Context, e audit.Entry) {
	e.Role = string(callerRole(c))
	e.IP = c.ClientIP()
	e.UserAgent = c.Request.UserAgent()
	if err := h.audit.Record(c.Request.Context(), e); err != nil && h.log != nil {
		h.log.Warn("audit record failed", "action", e.Action, "err", err)
	}
}

// listAuditLogs → GET /audit-logs?action=&from=&to=&limit= → { auditLogs: [...] }.
// Admin-only (gated by RequireAdmin on the route). Limit is clamped by the audit
// package's shared page-size policy.
func (h *handlers) listAuditLogs(c *gin.Context) {
	entries, err := h.audit.List(c.Request.Context(), audit.Filter{
		Action: c.Query("action"),
		From:   c.Query("from"),
		To:     c.Query("to"),
		Limit:  parseLimit(c.Query("limit")),
	})
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"auditLogs": entries})
}

// parseLimit turns the ?limit= query string into an int; a missing or malformed
// value yields 0, which ClampLimit maps to the default page size.
func parseLimit(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// listNodes → GET /nodes: the static Go node registry as { nodes: [...] }.
func (h *handlers) listNodes(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"nodes": nodes.All()})
}

// listFlows → GET /flows?q=&status=&folder=
func (h *handlers) listFlows(c *gin.Context) {
	flows, err := h.store.ListFlows(c.Request.Context(), store.FlowFilter{
		Q:      c.Query("q"),
		Status: c.Query("status"),
		Folder: c.Query("folder"),
	})
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"flows": flows})
}

// getFlow → GET /flows/{id} (404 with error envelope when missing).
func (h *handlers) getFlow(c *gin.Context) {
	flow, err := h.store.GetFlow(c.Request.Context(), c.Param("id"))
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

// listExecutions → GET /executions?status=&flowId=
func (h *handlers) listExecutions(c *gin.Context) {
	execs, err := h.store.ListExecutions(c.Request.Context(), store.ExecutionFilter{
		Status: c.Query("status"),
		FlowID: c.Query("flowId"),
	})
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"executions": execs})
}

// getExecution → GET /executions/{id} (includes steps; 404 when missing).
func (h *handlers) getExecution(c *gin.Context) {
	exec, err := h.store.GetExecution(c.Request.Context(), c.Param("id"))
	if err != nil {
		if store.IsNotFound(err) {
			errorEnvelope(c, http.StatusNotFound, "not_found", err.Error())
			return
		}
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, exec)
}

// listConnections → GET /connections, RBAC-filtered by the caller's resolved
// role (JWT → X-Role → defaultRole; see resolveRole).
func (h *handlers) listConnections(c *gin.Context) {
	role := string(callerRole(c))
	conns, err := h.store.ListConnections(c.Request.Context(), []string{role})
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"connections": conns})
}
