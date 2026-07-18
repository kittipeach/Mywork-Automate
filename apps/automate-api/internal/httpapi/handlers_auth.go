package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/pkg/authz"
)

// authHandlers serves the local-login and identity endpoints. deps.service is
// non-nil only when local auth is enabled and permitted (see NewRouter).
type authHandlers struct {
	deps authDeps
}

// audit logs an auth event if a logger is configured (docs/spec/07 §6:
// auth.login/failed must be audited). It never panics on a nil logger.
func (h *authHandlers) audit(msg string, kv ...any) {
	if h.deps.log != nil {
		h.deps.log(msg, kv...)
	}
}

// recordAuth appends an auth event to the append-only audit trail. A record
// failure must never break login: it is silently swallowed here (the structured
// log via audit() still fires). deps.audit is never nil (Noop when disabled).
func (h *authHandlers) recordAuth(c *gin.Context, e audit.Entry) {
	if h.deps.audit == nil {
		return
	}
	e.IP = c.ClientIP()
	e.UserAgent = c.Request.UserAgent()
	_ = h.deps.audit.Record(c.Request.Context(), e)
}

// localLogin → POST /auth/local/login {email, password}. On success it returns
// { token, roles } and audits auth.login; on failure it audits auth.failed and
// returns a 401 (invalid credentials) or 423 Locked (account locked). Registered
// only when local auth is enabled.
func (h *authHandlers) localLogin(c *gin.Context) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if body.Email == "" || body.Password == "" {
		errorEnvelope(c, http.StatusBadRequest, "bad_request", "email and password are required")
		return
	}

	res, until, err := h.deps.service.Authenticate(body.Email, body.Password)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrAccountLocked):
			h.audit("auth.failed", "email", body.Email, "reason", "locked",
				"ip", c.ClientIP(), "userAgent", c.Request.UserAgent())
			h.recordAuth(c, audit.Entry{Action: audit.ActionAuthFailed,
				Detail: map[string]any{"email": body.Email, "reason": "locked"}})
			c.Header("Retry-After", until.UTC().Format(http.TimeFormat))
			errorEnvelope(c, http.StatusLocked, "account_locked",
				"account locked due to too many failed attempts")
		case errors.Is(err, auth.ErrInvalidCredentials):
			h.audit("auth.failed", "email", body.Email, "reason", "invalid_credentials",
				"ip", c.ClientIP(), "userAgent", c.Request.UserAgent())
			h.recordAuth(c, audit.Entry{Action: audit.ActionAuthFailed,
				Detail: map[string]any{"email": body.Email, "reason": "invalid_credentials"}})
			errorEnvelope(c, http.StatusUnauthorized, "invalid_credentials",
				"invalid email or password")
		default:
			errorEnvelope(c, http.StatusInternalServerError, "internal", err.Error())
		}
		return
	}

	h.audit("auth.login", "email", body.Email, "userId", res.UserID,
		"ip", c.ClientIP(), "userAgent", c.Request.UserAgent())
	h.recordAuth(c, audit.Entry{Action: audit.ActionAuthLogin, UserID: res.UserID,
		Role: highestRoleStr(res.Roles), Detail: map[string]any{"email": body.Email}})
	// Set the SSO auth cookie so a browser is authenticated on subsequent requests
	// without a JS-managed bearer header. The token is also returned in the body
	// for API/CLI clients that prefer the Authorization header.
	h.setAuthCookie(c, res.Token)
	c.JSON(http.StatusOK, gin.H{
		"token":  res.Token,
		"userId": res.UserID,
		"roles":  res.Roles,
	})
}

// setAuthCookie writes the JWT to an HttpOnly cookie. Secure is set in protected
// environments (HTTPS); SameSite=Lax lets top-level SSO redirects carry it while
// blocking cross-site sends. HttpOnly keeps it out of reach of page JavaScript.
func (h *authHandlers) setAuthCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authTokenCookie, token, int(authCookieTTL.Seconds()), "/", "", h.deps.protected, true)
}

// logout → POST /auth/logout. Clears the SSO auth cookie. Safe to call
// unauthenticated (it only expires the cookie); no role is required.
func (h *authHandlers) logout(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(authTokenCookie, "", -1, "/", "", h.deps.protected, true)
	c.JSON(http.StatusOK, gin.H{"status": "logged_out"})
}

// me → GET /me. Returns the caller's resolved role and effective permissions
// (from authz). Public-ish: it reflects whatever resolveRole determined, which
// is defaultRole for unauthenticated dev callers.
func (h *authHandlers) me(c *gin.Context) {
	role := callerRole(c)
	perms := authz.Permissions(role)
	permStrs := make([]string, len(perms))
	for i, p := range perms {
		permStrs[i] = string(p)
	}
	c.JSON(http.StatusOK, gin.H{
		"role":        string(role),
		"permissions": permStrs,
	})
}

// highestRoleStr returns the most-privileged role from a claim set as a string,
// or "" when none is a valid role. It records who logged in at their effective
// privilege on the auth.login audit entry.
func highestRoleStr(roles []string) string {
	candidates := make([]authz.Role, 0, len(roles))
	for _, r := range roles {
		candidates = append(candidates, authz.Role(r))
	}
	if best, ok := authz.Highest(candidates); ok {
		return string(best)
	}
	return ""
}
