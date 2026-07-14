package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/internal/config"
)

// loginService builds a local-auth service seeded with the dev admin.
func loginService(t *testing.T) *auth.Service {
	t.Helper()
	tokens, err := auth.NewTokenIssuer("login-secret")
	if err != nil {
		t.Fatalf("NewTokenIssuer: %v", err)
	}
	adminHash, _ := auth.HashPassword(auth.DevAdminPassword)
	return auth.NewService(
		auth.NewMemUserStore(
			auth.User{ID: "usr_admin", Email: auth.DevAdminEmail, PasswordHash: adminHash, Roles: []string{"admin"}},
		),
		tokens,
		auth.NewLockoutTracker(nil),
	)
}

// loginRouter builds a router with local auth enabled, seeded with the dev admin.
func loginRouter(t *testing.T) (http.Handler, *auth.Service) {
	t.Helper()
	svc := loginService(t)
	cfg := config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal, AuthLocalEnabled: true}
	r := NewRouter(cfg, seedFake(), runner.Runner(&fakeRunner{}), nil, nil, AuthConfig{Service: svc})
	return r, svc
}

func postJSON(t *testing.T, r http.Handler, path, body string, headers map[string]string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var parsed map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	}
	return w, parsed
}

// TestLogin_WithSlogLogger exercises NewRouter's slog-logger wiring (the
// deps.log closure) end-to-end.
func TestLogin_WithSlogLogger(t *testing.T) {
	svc := loginService(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal, AuthLocalEnabled: true}
	r := NewRouter(cfg, seedFake(), runner.Runner(&fakeRunner{}), nil, nil, AuthConfig{Service: svc, Logger: logger})

	w, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)
	if w.Code != http.StatusOK || body["token"] == nil {
		t.Fatalf("login with slog logger = %d (%s)", w.Code, w.Body.String())
	}
}

func TestLogin_Success(t *testing.T) {
	r, _ := loginRouter(t)
	w, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	if body["token"] == nil || body["token"] == "" {
		t.Fatal("missing token")
	}
	roles, _ := body["roles"].([]any)
	if len(roles) != 1 || roles[0] != "admin" {
		t.Errorf("roles = %v, want [admin]", roles)
	}
}

func TestLogin_WrongPassword_401(t *testing.T) {
	r, _ := loginRouter(t)
	w, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"wrong-password-99"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_credentials")
}

func TestLogin_UnknownUser_401(t *testing.T) {
	r, _ := loginRouter(t)
	w, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"ghost@x","password":"whatever-1234"}`, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	assertErrorEnvelope(t, body, "invalid_credentials")
}

func TestLogin_MissingFields_400(t *testing.T) {
	r, _ := loginRouter(t)
	for _, body := range []string{`{}`, `{"email":"a@b"}`, `{"password":"x"}`} {
		w, parsed := postJSON(t, r, APIBasePath+"/auth/local/login", body, nil)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %q status = %d, want 400", body, w.Code)
		}
		assertErrorEnvelope(t, parsed, "bad_request")
	}
}

func TestLogin_MalformedJSON_400(t *testing.T) {
	r, _ := loginRouter(t)
	w, parsed := postJSON(t, r, APIBasePath+"/auth/local/login", `{not json`, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	assertErrorEnvelope(t, parsed, "bad_request")
}

func TestLogin_Lockout_423(t *testing.T) {
	r, _ := loginRouter(t)
	// 5 bad attempts trips the lock; the 5th response is already 423.
	var last *httptest.ResponseRecorder
	for i := 0; i < auth.MaxFailedAttempts; i++ {
		last, _ = postJSON(t, r, APIBasePath+"/auth/local/login",
			`{"email":"`+auth.DevAdminEmail+`","password":"bad-000000000"}`, nil)
	}
	if last.Code != http.StatusLocked {
		t.Fatalf("5th attempt status = %d, want 423", last.Code)
	}
	if last.Header().Get("Retry-After") == "" {
		t.Error("missing Retry-After header on lockout")
	}
	// Even the correct password is now refused with 423.
	w, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)
	if w.Code != http.StatusLocked {
		t.Fatalf("correct pw during lock status = %d, want 423", w.Code)
	}
	assertErrorEnvelope(t, body, "account_locked")
}

func TestLogin_NotRegisteredWhenLocalAuthDisabled(t *testing.T) {
	// No service → POST /auth/local/login is not registered → 404.
	r := NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal}, seedFake(), nil, nil, nil, AuthConfig{})
	w, _ := postJSON(t, r, APIBasePath+"/auth/local/login", `{"email":"a@b","password":"x"}`, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (route absent)", w.Code)
	}
}

func TestLogin_ThenUseTokenOnProtectedRoute(t *testing.T) {
	r, _ := loginRouter(t)
	_, body := postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)
	tok, _ := body["token"].(string)
	if tok == "" {
		t.Fatal("no token from login")
	}
	// Use the token to reach an admin-only route.
	req := httptest.NewRequest(http.MethodPost, APIBasePath+"/connections",
		strings.NewReader(`{"name":"C","type":"postgres"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("token-authed create connection = %d, want 201", w.Code)
	}
}

// --- /me ---

func TestMe_PerRole(t *testing.T) {
	tests := []struct {
		role      string // "" = no header (default admin)
		wantRole  string
		wantPerms []string
	}{
		{"", "admin", []string{"flow.view", "run.view", "flow.create", "flow.publish", "flow.run", "connection.manage", "template.manage", "ai.use", "masking.bypass"}},
		{"designer", "designer", []string{"flow.view", "run.view", "flow.create", "flow.publish", "flow.run", "template.manage", "ai.use", "masking.bypass"}},
		{"operator", "operator", []string{"flow.view", "run.view", "flow.run", "ai.use", "masking.bypass"}},
		{"viewer", "viewer", []string{"flow.view", "run.view"}},
	}
	for _, tt := range tests {
		t.Run(tt.wantRole, func(t *testing.T) {
			r := newTestRouter(seedFake())
			headers := map[string]string{}
			if tt.role != "" {
				headers[roleHeader] = tt.role
			}
			w, body := doReq(t, r, http.MethodGet, APIBasePath+"/me", headers)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if body["role"] != tt.wantRole {
				t.Errorf("role = %v, want %v", body["role"], tt.wantRole)
			}
			perms, _ := body["permissions"].([]any)
			if len(perms) != len(tt.wantPerms) {
				t.Fatalf("permissions = %v, want %v", perms, tt.wantPerms)
			}
			for i, p := range tt.wantPerms {
				if perms[i] != p {
					t.Errorf("permissions[%d] = %v, want %v", i, perms[i], p)
				}
			}
		})
	}
}

// --- audit logging ---

// captureRouter wires an authHandlers with a capturing logger so the audit path
// (deps.log) is exercised for both success and failure.
func captureRouter(t *testing.T, svc *auth.Service) (http.Handler, *[]string) {
	t.Helper()
	var logs []string
	deps := authDeps{service: svc, log: func(msg string, _ ...any) { logs = append(logs, msg) }}
	ah := &authHandlers{deps: deps}

	r := gin.New()
	v1 := r.Group(APIBasePath)
	v1.POST("/auth/local/login", ah.localLogin)
	v1.Use(resolveRole(deps))
	v1.GET("/me", ah.me)
	return r, &logs
}

func TestLogin_AuditsSuccessAndFailure(t *testing.T) {
	svc := loginService(t)
	r, logs := captureRouter(t, svc)

	// Failure first.
	postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"bad-000000000"}`, nil)
	// Then success.
	postJSON(t, r, APIBasePath+"/auth/local/login",
		`{"email":"`+auth.DevAdminEmail+`","password":"`+auth.DevAdminPassword+`"}`, nil)

	if len(*logs) != 2 {
		t.Fatalf("audit events = %v, want [auth.failed auth.login]", *logs)
	}
	if (*logs)[0] != "auth.failed" || (*logs)[1] != "auth.login" {
		t.Errorf("audit order = %v, want [auth.failed auth.login]", *logs)
	}
}

func TestLogin_AuditsLockout(t *testing.T) {
	svc := loginService(t)
	r, logs := captureRouter(t, svc)
	for i := 0; i < auth.MaxFailedAttempts; i++ {
		postJSON(t, r, APIBasePath+"/auth/local/login",
			`{"email":"`+auth.DevAdminEmail+`","password":"bad-000000000"}`, nil)
	}
	// Every failed attempt is audited; the 5th (and any after) reports "locked".
	if len(*logs) != auth.MaxFailedAttempts {
		t.Fatalf("audit events = %d, want %d", len(*logs), auth.MaxFailedAttempts)
	}
	for _, m := range *logs {
		if m != "auth.failed" {
			t.Errorf("audit event = %q, want auth.failed", m)
		}
	}
}

func TestMe_UnknownRole_EmptyPermissions(t *testing.T) {
	r := newTestRouter(seedFake())
	w, body := doReq(t, r, http.MethodGet, APIBasePath+"/me", map[string]string{roleHeader: "auditor"})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	perms, _ := body["permissions"].([]any)
	if len(perms) != 0 {
		t.Errorf("permissions = %v, want empty", perms)
	}
}
