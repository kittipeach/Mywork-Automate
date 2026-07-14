package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// allowedOrigins are the web dev origins permitted by default. Any other Origin
// is still echoed back for GET requests (the API is read-only today), so the
// local UI works regardless of which dev port it runs on.
var allowedOrigins = map[string]bool{
	"http://localhost:3000": true,
	"http://localhost:4455": true,
}

// corsMiddleware sets permissive-but-scoped CORS headers for the web client and
// short-circuits preflight (OPTIONS) with 204. Allowed methods are the read
// verbs plus the write verbs later stories will add; allowed headers include
// the X-Role RBAC header the connections endpoint reads.
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (allowedOrigins[origin] || c.Request.Method == http.MethodGet || c.Request.Method == http.MethodOptions) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type,Authorization,X-Role")
			c.Header("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
