package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/nodes"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/store"
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
// store and the flow Runner (nil when Temporal is unavailable — /run then 503s).
type handlers struct {
	store  store.Store
	runner runner.Runner
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

// listConnections → GET /connections, RBAC-filtered by the caller's X-Role.
func (h *handlers) listConnections(c *gin.Context) {
	role := c.GetHeader(roleHeader)
	if role == "" {
		role = defaultRole
	}
	conns, err := h.store.ListConnections(c.Request.Context(), []string{role})
	if err != nil {
		errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"connections": conns})
}
