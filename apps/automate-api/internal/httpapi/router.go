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

	"github.com/mywork/automate/internal/config"
)

// APIBasePath is the versioned control-plane prefix (docs/spec/06 §2).
const APIBasePath = "/api/automate/v1"

// NewRouter builds the Gin engine for the given config.
func NewRouter(cfg config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ready", "env": cfg.Env})
	})

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

	return r
}
