// Package httpapi builds the automate-api HTTP surface (Gin).
//
// For E1-S1 this exposes health/readiness probes and a versioned API group
// that later stories (E2 auth, E3 flows, ...) hang handlers off. RBAC and the
// masking interceptor are wired as middleware slots here so downstream stories
// only register routes.
package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
)

// APIBasePath is the versioned control-plane prefix (docs/spec/06 §2).
const APIBasePath = "/api/automate/v1"

// NewRouter builds the Gin engine for the given config, data store and flow
// Runner. The store backs the read+write endpoints (/flows, /executions,
// /connections); /nodes is served from the static Go registry. The runner backs
// POST /flows/{id}/run and may be nil (Temporal unavailable) — /run then 503s.
func NewRouter(cfg config.Config, st store.Store, run runner.Runner) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "env": cfg.Env})
	})

	h := &handlers{store: st, runner: run}

	v1 := r.Group(APIBasePath)
	// GET /auth/config — frontend uses this to decide whether to render the
	// local-login form (docs/spec/06 §Admin & Auth). Only "local" when the
	// runtime guard permits it.
	v1.GET("/auth/config", func(c *gin.Context) {
		providers := []string{"entra"}
		if cfg.AuthLocalEnabled && !cfg.IsProtectedEnv() {
			providers = append(providers, "local")
		}
		c.JSON(http.StatusOK, gin.H{"providers": providers})
	})

	v1.GET("/nodes", h.listNodes)
	v1.GET("/flows", h.listFlows)
	v1.GET("/flows/:id", h.getFlow)
	v1.GET("/executions", h.listExecutions)
	v1.GET("/executions/:id", h.getExecution)
	v1.GET("/connections", h.listConnections)

	// Write endpoints (close the execution loop + edit the catalog).
	v1.POST("/flows", h.createFlow)
	v1.POST("/flows/:id/run", h.runFlow)
	v1.PUT("/flows/:id/draft", h.updateFlowDraft)
	v1.POST("/flows/:id/publish", h.publishFlow)
	v1.POST("/connections", h.createConnection)
	v1.PUT("/connections/:id", h.updateConnection)
	v1.DELETE("/connections/:id", h.deleteConnection)
	v1.POST("/connections/:id/test", h.testConnection)

	return r
}
