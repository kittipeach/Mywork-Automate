package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/pkg/authz"
)

// ctxRoleKey is the gin.Context key under which resolveRole stores the caller's
// effective role (an authz.Role).
const ctxRoleKey = "authz.role"

// authDeps bundles the auth collaborators the router needs: the login/verify
// service and a logger for audit. Both may be nil when local auth is disabled —
// the middleware then falls back to the X-Role header / defaultRole so existing
// dev flows and tests keep working.
type authDeps struct {
	service *auth.Service
	log     func(msg string, kv ...any)
}

// resolveRole is middleware that determines the caller's effective role and
// stores it in the gin context under ctxRoleKey. Resolution order:
//
//  1. A valid `Authorization: Bearer <jwt>` — verified via the auth service; the
//     highest-privilege role from the token's claims is used.
//  2. Otherwise the `X-Role` header, if it names a valid role.
//  3. Otherwise defaultRole (admin) so dev/unauthenticated flows and the
//     existing read tests keep working.
//
// An invalid/expired/forged bearer token does not 401 here; it simply falls
// through to the header/default path (per the E2-S3 "bad-JWT → falls back"
// requirement). Per-route RequirePermission is what actually denies access.
func resolveRole(deps authDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := roleFromContext(deps, c)
		c.Set(ctxRoleKey, role)
		c.Next()
	}
}

// roleFromContext applies the resolution order described on resolveRole and
// returns the effective authz.Role.
func roleFromContext(deps authDeps, c *gin.Context) authz.Role {
	if deps.service != nil {
		if tok := bearerToken(c); tok != "" {
			if claims, err := deps.service.Verify(tok); err == nil {
				candidates := make([]authz.Role, 0, len(claims.Roles))
				for _, r := range claims.Roles {
					candidates = append(candidates, authz.Role(r))
				}
				if best, ok := authz.Highest(candidates); ok {
					return best
				}
			}
		}
	}

	if hdr := c.GetHeader(roleHeader); hdr != "" {
		if r := authz.Role(hdr); authz.IsValidRole(r) {
			return r
		}
		// An unknown X-Role (e.g. "auditor") is honoured as-is: it is not a valid
		// role, so RequirePermission denies everything (deny-by-default), and the
		// connection RBAC filter treats it as seeing nothing. This preserves the
		// existing "unknown role sees none" behaviour.
		return authz.Role(hdr)
	}

	return authz.Role(defaultRole)
}

// bearerToken extracts the token from an `Authorization: Bearer <token>` header,
// or "" when absent/malformed.
func bearerToken(c *gin.Context) string {
	h := c.GetHeader("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// callerRole returns the role resolved by resolveRole. If the middleware did not
// run (should not happen on protected routes) it falls back to defaultRole.
func callerRole(c *gin.Context) authz.Role {
	if v, ok := c.Get(ctxRoleKey); ok {
		if r, ok := v.(authz.Role); ok {
			return r
		}
	}
	return authz.Role(defaultRole)
}

// RequirePermission returns middleware that aborts with 403 (forbidden error
// envelope) unless the caller's resolved role is granted p. Deny-by-default:
// unknown/invalid roles are denied.
func RequirePermission(p authz.Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		role := callerRole(c)
		if !authz.Can(role, p) {
			errorEnvelope(c, http.StatusForbidden, "forbidden",
				"role does not have permission: "+string(p))
			c.Abort()
			return
		}
		c.Next()
	}
}
