package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// protectedRoute describes one guarded endpoint and the roles that may reach it.
// allowed lists the roles that should get a non-403 response; every other role
// (of admin/designer/operator/viewer) must get 403.
type protectedRoute struct {
	name    string
	method  string
	path    string
	body    string
	allowed map[string]bool
}

// rbacRouter builds a router with a runner and a fake store seeded with a flow
// definition so write/run routes can proceed past the RBAC gate.
func rbacRouter() http.Handler {
	fake := seedFake()
	fake.def = sampleDef()
	run := &fakeRunner{result: flowspec.FlowResult{Path: []string{"t1", "q1"}}}
	return NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, fake, run, nil, nil, AuthConfig{}, nil)
}

// doRoleReq issues a request with an X-Role header (empty role → no header).
func doRoleReq(t *testing.T, r http.Handler, method, path, role, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody = strings.NewReader(body)
	req := httptest.NewRequest(method, path, reqBody)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set(roleHeader, role)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestRBAC_Matrix_PerRoute asserts deny-by-default per protected route: each of
// the four roles is either allowed (non-403) or forbidden (403) exactly per the
// docs/spec/07 §2.1 matrix.
func TestRBAC_Matrix_PerRoute(t *testing.T) {
	base := APIBasePath
	routes := []protectedRoute{
		// Reads: FlowView / RunView — all four roles allowed.
		{"list flows", http.MethodGet, base + "/flows", "", allRoles()},
		{"get flow", http.MethodGet, base + "/flows/flw_payroll", "", allRoles()},
		{"list nodes", http.MethodGet, base + "/nodes", "", allRoles()},
		{"list executions", http.MethodGet, base + "/executions", "", allRoles()},
		{"get execution", http.MethodGet, base + "/executions/exe_1001", "", allRoles()},
		{"list connections", http.MethodGet, base + "/connections", "", allRoles()},
		// FlowCreate — admin, designer.
		{"create flow", http.MethodPost, base + "/flows", `{"name":"X"}`, roleSet("admin", "designer")},
		{"update draft", http.MethodPut, base + "/flows/flw_payroll/draft", `{"definition":{"nodes":[],"edges":[]}}`, roleSet("admin", "designer")},
		// FlowPublish — admin, designer.
		{"publish flow", http.MethodPost, base + "/flows/flw_payroll/publish", "", roleSet("admin", "designer")},
		// FlowRun — admin, designer, operator.
		{"run flow", http.MethodPost, base + "/flows/flw_payroll/run", "", roleSet("admin", "designer", "operator")},
		// ConnectionManage — admin only.
		{"create connection", http.MethodPost, base + "/connections", `{"name":"C","type":"postgres"}`, roleSet("admin")},
		{"update connection", http.MethodPut, base + "/connections/conn_hr", `{"name":"C","type":"postgres"}`, roleSet("admin")},
		{"delete connection", http.MethodDelete, base + "/connections/conn_hr", "", roleSet("admin")},
		{"test connection", http.MethodPost, base + "/connections/conn_hr/test", "", roleSet("admin")},
	}

	for _, rt := range routes {
		for _, role := range []string{"admin", "designer", "operator", "viewer"} {
			t.Run(rt.name+"/"+role, func(t *testing.T) {
				r := rbacRouter()
				w := doRoleReq(t, r, rt.method, rt.path, role, rt.body)
				if rt.allowed[role] {
					if w.Code == http.StatusForbidden {
						t.Fatalf("%s as %s = 403, want allowed", rt.name, role)
					}
				} else {
					if w.Code != http.StatusForbidden {
						t.Fatalf("%s as %s = %d, want 403 (deny-by-default)", rt.name, role, w.Code)
					}
				}
			})
		}
	}
}

// TestRBAC_UnknownRole_ForbiddenEverywhere asserts a role outside the matrix is
// denied on every protected route.
func TestRBAC_UnknownRole_ForbiddenEverywhere(t *testing.T) {
	paths := []struct {
		method, path, body string
	}{
		{http.MethodGet, APIBasePath + "/flows", ""},
		{http.MethodGet, APIBasePath + "/executions", ""},
		{http.MethodPost, APIBasePath + "/flows", `{"name":"X"}`},
		{http.MethodPost, APIBasePath + "/flows/flw_payroll/run", ""},
		{http.MethodPost, APIBasePath + "/connections", `{"name":"C","type":"postgres"}`},
	}
	for _, p := range paths {
		r := rbacRouter()
		w := doRoleReq(t, r, p.method, p.path, "auditor", p.body)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s %s as auditor = %d, want 403", p.method, p.path, w.Code)
		}
	}
}

// TestRBAC_NoAuth_DefaultsToAdmin confirms the dev fallback: no JWT and no
// X-Role resolves to admin, so a write route succeeds.
func TestRBAC_NoAuth_DefaultsToAdmin(t *testing.T) {
	r := rbacRouter()
	w := doRoleReq(t, r, http.MethodPost, APIBasePath+"/flows", "", `{"name":"New"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("no-auth create flow = %d, want 201 (defaults to admin)", w.Code)
	}
}

// --- JWT-authenticated path + bad-JWT fallback ---

// tokenForRoles mints a token with the given roles using the service's issuer
// and returns it alongside a router wired to that service.
func tokenForRoles(t *testing.T, roles []string) (string, *auth.Service, http.Handler) {
	t.Helper()
	tokens, _ := auth.NewTokenIssuer("router-secret")
	tok, err := tokens.Issue("usr_x", roles)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	hash, _ := auth.HashPassword(auth.DevAdminPassword)
	users := auth.NewMemUserStore(auth.User{ID: "usr_x", Email: "u@x", PasswordHash: hash, Roles: roles})
	svc := auth.NewService(users, tokens, auth.NewLockoutTracker(nil))
	fake := seedFake()
	fake.def = sampleDef()
	cfg := config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal, AuthLocalEnabled: true}
	r := NewRouter(cfg, fake, runner.Runner(&fakeRunner{}), nil, nil, AuthConfig{Service: svc}, nil)
	return tok, svc, r
}

func TestRBAC_JWT_AdminHappyPath(t *testing.T) {
	tok, _, r := tokenForRoles(t, []string{"admin"})
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/connections", strings.NewReader(`{"name":"C","type":"postgres"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("admin JWT create connection = %d, want 201", w.Code)
	}
}

func TestRBAC_JWT_ViewerForbiddenOnWrite(t *testing.T) {
	tok, _, r := tokenForRoles(t, []string{"viewer"})
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/flows", strings.NewReader(`{"name":"C"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer JWT create flow = %d, want 403", w.Code)
	}
}

func TestRBAC_JWT_HighestRoleWins(t *testing.T) {
	// A token carrying both viewer and admin resolves to admin.
	tok, _, r := tokenForRoles(t, []string{"viewer", "admin"})
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/connections", strings.NewReader(`{"name":"C","type":"postgres"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("multi-role JWT create connection = %d, want 201 (highest role wins)", w.Code)
	}
}

func TestRBAC_BadJWT_FallsBackToHeader(t *testing.T) {
	_, _, r := tokenForRoles(t, []string{"admin"})
	// Garbage bearer token → verification fails → falls back to X-Role: viewer.
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/flows", strings.NewReader(`{"name":"C"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer not.a.real.jwt")
	req.Header.Set(roleHeader, "viewer")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("bad JWT + viewer header on create flow = %d, want 403 (fell back to viewer)", w.Code)
	}
}

func TestRBAC_BadJWT_NoHeader_FallsBackToDefaultAdmin(t *testing.T) {
	_, _, r := tokenForRoles(t, []string{"viewer"})
	// Garbage bearer, no X-Role → default admin → allowed.
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/flows", strings.NewReader(`{"name":"C"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer garbage")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("bad JWT, no header create flow = %d, want 201 (default admin)", w.Code)
	}
}

// TestBearerToken covers extraction from the Authorization header, including the
// non-Bearer and short-header fallthroughs.
func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"bearer lowercase", "lowercase"}, // scheme is case-insensitive
		{"Bearer   spaced  ", "spaced"},   // trimmed
		{"Basic dXNlcjpwdw==", ""},        // wrong scheme
		{"", ""},                          // absent
		{"Bearer", ""},                    // too short, no token
	}
	for _, tt := range tests {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		if tt.header != "" {
			c.Request.Header.Set("Authorization", tt.header)
		}
		if got := bearerToken(c); got != tt.want {
			t.Errorf("bearerToken(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

// TestCallerRole_FallbackWhenUnset covers the defensive path where resolveRole
// did not run (no value in the context) → defaultRole.
func TestCallerRole_FallbackWhenUnset(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := callerRole(c); string(got) != defaultRole {
		t.Errorf("callerRole with empty ctx = %q, want %q", got, defaultRole)
	}
	// A value of the wrong type also falls back.
	c.Set(ctxRoleKey, 123)
	if got := callerRole(c); string(got) != defaultRole {
		t.Errorf("callerRole with wrong-type ctx = %q, want %q", got, defaultRole)
	}
}

// --- helpers ---

func allRoles() map[string]bool {
	return roleSet("admin", "designer", "operator", "viewer")
}

func roleSet(roles ...string) map[string]bool {
	m := make(map[string]bool, len(roles))
	for _, r := range roles {
		m[r] = true
	}
	return m
}
