// Package httpapi builds the automate-api HTTP surface (Gin).
//
// For E1-S1 this exposes health/readiness probes and a versioned API group
// that later stories (E2 auth, E3 flows, ...) hang handlers off. E2-S2/S3 add
// local login, the RBAC middleware (deny-by-default, applied per route) and the
// role-resolution middleware; the masking interceptor is wired later.
package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/preview"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/pkg/authz"
	"github.com/mywork/automate/pkg/masking"
)

// APIBasePath is the versioned control-plane prefix (docs/spec/06 §2).
const APIBasePath = "/api/automate/v1"

// AuthConfig carries the auth wiring into NewRouter. Service is the local-login
// verifier/issuer; it is nil when local auth is disabled or not permitted, in
// which case the RBAC middleware falls back to the X-Role header / defaultRole
// and POST /auth/local/login is not registered. Logger is used for audit
// events; nil is tolerated (no-op).
type AuthConfig struct {
	Service *auth.Service
	Logger  *slog.Logger
}

// NewRouter builds the Gin engine for the given config, data store, flow Runner,
// audit trail, schedule Scheduler and auth wiring. The store backs the
// read+write endpoints (/flows, /executions, /connections); /nodes is served
// from the static Go registry. The runner backs POST /flows/{id}/run and may be
// nil (Temporal unavailable) — /run then 503s. auditSvc records every mutating
// action and backs GET /audit-logs; sched keeps published flows' schedules in
// sync — both must be non-nil (the composition root passes audit.NewNoop() /
// scheduler.Noop{} when the backend is unavailable). authCfg wires local login +
// RBAC role resolution; pass a zero AuthConfig to run without local auth (X-Role
// / defaultRole fallback). querier backs the query-preview and schema endpoints
// (E6-S4/E6-S2) — the composition root passes preview.PoolQuerier over the API's
// pgx pool; nil disables those endpoints (they then 503).
func NewRouter(cfg config.Config, st store.Store, run runner.Runner, auditSvc audit.Service, sched scheduler.Scheduler, authCfg AuthConfig, querier preview.Querier) *gin.Engine {
	if auditSvc == nil {
		auditSvc = audit.NewNoop()
	}
	if sched == nil {
		sched = scheduler.Noop{}
	}

	// The masking engine uses the security-reviewed default rule set; its patterns
	// are constant so compilation can never fail at runtime — panic if it somehow
	// does rather than serve unmasked previews.
	maskEng, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		panic("httpapi: masking engine build failed: " + err.Error())
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(corsMiddleware())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "env": cfg.Env})
	})

	h := &handlers{store: st, runner: run, audit: auditSvc, sched: sched, log: authCfg.Logger, querier: querier, mask: maskEng}

	deps := authDeps{service: authCfg.Service, audit: auditSvc}
	if authCfg.Logger != nil {
		log := authCfg.Logger
		deps.log = func(msg string, kv ...any) { log.Info(msg, kv...) }
	}
	ah := &authHandlers{deps: deps}

	v1 := r.Group(APIBasePath)
	// Public: the frontend uses /auth/config to decide whether to render the
	// local-login form; /auth/local/login is the login itself. Neither requires
	// a resolved role.
	v1.GET("/auth/config", func(c *gin.Context) {
		providers := []string{"entra"}
		if cfg.AuthLocalEnabled && !cfg.IsProtectedEnv() {
			providers = append(providers, "local")
		}
		c.JSON(http.StatusOK, gin.H{"providers": providers})
	})
	// Local login is only reachable when a service is wired (local auth enabled
	// and permitted by the runtime guard).
	if authCfg.Service != nil {
		v1.POST("/auth/local/login", ah.localLogin)
	}

	// Everything below resolves the caller's role (JWT → X-Role → defaultRole)
	// and is then guarded per route by RequirePermission (deny-by-default).
	authed := v1.Group("")
	authed.Use(resolveRole(deps))

	// /me reports the caller's role + effective permissions (no permission gate).
	authed.GET("/me", ah.me)

	// /nodes is the static catalog — visible to anyone who can view flows.
	authed.GET("/nodes", RequirePermission(authz.FlowView), h.listNodes)

	// Read endpoints.
	authed.GET("/flows", RequirePermission(authz.FlowView), h.listFlows)
	authed.GET("/flows/:id", RequirePermission(authz.FlowView), h.getFlow)
	authed.GET("/executions", RequirePermission(authz.RunView), h.listExecutions)
	authed.GET("/executions/:id", RequirePermission(authz.RunView), h.getExecution)
	authed.GET("/connections", RequirePermission(authz.FlowView), h.listConnections)
	// Version history read model — anyone who can view flows (E4-S2).
	authed.GET("/flows/:id/versions", RequirePermission(authz.FlowView), h.listVersions)

	// Audit trail read model — admin-only (docs/spec/07 §6). A permission gate
	// would leak the trail to any role sharing that permission, so it is gated on
	// the exact admin role.
	authed.GET("/audit-logs", RequireAdmin(), h.listAuditLogs)

	// Write endpoints (close the execution loop + edit the catalog).
	authed.POST("/flows", RequirePermission(authz.FlowCreate), h.createFlow)
	authed.PUT("/flows/:id/draft", RequirePermission(authz.FlowCreate), h.updateFlowDraft)
	authed.POST("/flows/:id/publish", RequirePermission(authz.FlowPublish), h.publishFlow)
	// Lifecycle transitions + rollback — publish-privileged (E4-S1/S2).
	authed.POST("/flows/:id/pause", RequirePermission(authz.FlowPublish), h.pauseFlow)
	authed.POST("/flows/:id/resume", RequirePermission(authz.FlowPublish), h.resumeFlow)
	authed.POST("/flows/:id/stop", RequirePermission(authz.FlowPublish), h.stopFlow)
	authed.POST("/flows/:id/rollback", RequirePermission(authz.FlowPublish), h.rollbackFlow)
	authed.POST("/flows/:id/run", RequirePermission(authz.FlowRun), h.runFlow)
	// Execution control (E5-S5): cancel an in-flight run, or re-run a past one.
	// Both are run-privileged (FlowRun).
	authed.POST("/executions/:id/cancel", RequirePermission(authz.FlowRun), h.cancelExecution)
	authed.POST("/executions/:id/retry", RequirePermission(authz.FlowRun), h.retryExecution)
	authed.POST("/connections", RequirePermission(authz.ConnectionManage), h.createConnection)
	authed.PUT("/connections/:id", RequirePermission(authz.ConnectionManage), h.updateConnection)
	authed.DELETE("/connections/:id", RequirePermission(authz.ConnectionManage), h.deleteConnection)
	authed.POST("/connections/:id/test", RequirePermission(authz.ConnectionManage), h.testConnection)
	// Query preview (E6-S4) + schema metadata (E6-S2): read-only introspection of
	// a connection's database. Gated on FlowView (anyone who can design flows).
	authed.POST("/connections/:id/query-preview", RequirePermission(authz.FlowView), h.queryPreview)
	authed.GET("/connections/:id/schema", RequirePermission(authz.FlowView), h.connectionSchema)

	return r
}
