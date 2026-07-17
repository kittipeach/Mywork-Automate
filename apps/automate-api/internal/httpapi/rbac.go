package httpapi

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/pkg/authz"
)

// ctxRoleKey is the gin.Context key under which resolveRole stores the caller's
// effective role (an authz.Role).
const ctxRoleKey = "authz.role"

// authDeps bundles the auth collaborators the router needs: the login/verify
// service, a logger for structured auth events and the append-only audit trail.
// service/log may be nil when local auth is disabled — the middleware then falls
// back to the X-Role header / defaultRole so existing dev flows and tests keep
// working. audit is never nil (Noop when auditing is disabled) so localLogin can
// Record unconditionally.
type authDeps struct {
	service *auth.Service
	log     func(msg string, kv ...any)
	audit   audit.Service
	// protected marks a sit/uat/prod environment. When true, resolveRole fails
	// closed (401) for a request that resolves no identity instead of granting
	// the dev default role — a banking-grade deny-by-default posture. It is false
	// for dev (and in the zero authDeps used by tests), preserving local flows.
	protected bool
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
//
// The one hard stop is a protected environment (sit/uat/prod) in which no
// identity resolves at all: rather than fall back to the dev default role,
// resolveRole aborts 401. This closes the fail-open where an unauthenticated
// caller (or one whose token failed to verify and who sent no gateway-injected
// X-Role) would otherwise be handed the default admin role.
func resolveRole(deps authDeps) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, ok := roleFromContext(deps, c)
		if !ok {
			errorEnvelope(c, http.StatusUnauthorized, "unauthorized", "authentication required")
			c.Abort()
			return
		}
		c.Set(ctxRoleKey, role)
		c.Next()
	}
}

// roleFromContext applies the resolution order described on resolveRole. The
// second return is false only when no identity could be resolved in a protected
// environment — the caller (resolveRole) then denies with 401. In dev the
// default role is always resolved (ok=true) so local flows keep working.
func roleFromContext(deps authDeps, c *gin.Context) (authz.Role, bool) {
	if deps.service != nil {
		if tok := bearerToken(c); tok != "" {
			if claims, err := deps.service.Verify(tok); err == nil {
				candidates := make([]authz.Role, 0, len(claims.Roles))
				for _, r := range claims.Roles {
					candidates = append(candidates, authz.Role(r))
				}
				if best, ok := authz.Highest(candidates); ok {
					return best, true
				}
			}
		}
	}

	if hdr := c.GetHeader(roleHeader); hdr != "" {
		if r := authz.Role(hdr); authz.IsValidRole(r) {
			return r, true
		}
		// An unknown X-Role (e.g. "auditor") is honoured as-is: it is not a valid
		// role, so RequirePermission denies everything (deny-by-default), and the
		// connection RBAC filter treats it as seeing nothing. This preserves the
		// existing "unknown role sees none" behaviour. It still counts as a
		// resolved identity (the caller supplied a role), so it is denied with
		// 403 by the per-route gate rather than 401 here.
		return authz.Role(hdr), true
	}

	// No token and no X-Role. In a protected env this is an unauthenticated
	// request: fail closed. In dev, fall back to the default role.
	if deps.protected {
		return "", false
	}
	return authz.Role(defaultRole), true
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

// RequireAdmin returns middleware that aborts with 403 unless the caller's
// resolved role is exactly authz.Admin. It gates admin-only surfaces such as the
// audit-log read model (docs/spec/07 §6), where a permission gate would leak the
// trail to any role sharing that permission. Deny-by-default: every non-admin
// (and every unknown) role is denied.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if callerRole(c) != authz.Admin {
			errorEnvelope(c, http.StatusForbidden, "forbidden", "admin role required")
			c.Abort()
			return
		}
		c.Next()
	}
}
