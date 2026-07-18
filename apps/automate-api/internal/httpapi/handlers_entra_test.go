package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mywork/automate/apps/automate-api/internal/auth/entra"
	"github.com/mywork/automate/internal/config"
)

// TestResolveRole_EntraToken proves a real Entra ID token (RS256, JWKS-verified)
// authenticates through resolveRole in a protected environment — the prod SSO
// path — mapping its app-role claim to the caller's authz role.
func TestResolveRole_EntraToken(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const iss, aud, kid = "https://login.microsoftonline.com/t/v2.0", "api://automate", "k1"
	ver := entra.New(
		entra.Config{JWKSURL: "https://jwks", Issuer: iss, Audience: aud},
		entra.WithKeyFetcher(func(context.Context) (map[string]*rsa.PublicKey, error) {
			return map[string]*rsa.PublicKey{kid: &priv.PublicKey}, nil
		}),
	)
	// Protected env so there is no dev admin default — only a valid identity passes.
	r := NewRouter(config.Config{Env: config.EnvUAT, FileStore: config.FileStoreLocal},
		seedFake(), nil, nil, nil, AuthConfig{Entra: ver}, nil, nil)

	sign := func(roles []any, exp time.Time) string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": iss, "aud": aud, "sub": "u1", "roles": roles, "exp": exp.Unix(),
		})
		tok.Header["kid"] = kid
		s, _ := tok.SignedString(priv)
		return s
	}

	me := func(headers map[string]string, cookie *http.Cookie) (int, string) {
		req := httptest.NewRequest(http.MethodGet, APIBasePath+"/me", nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var body map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &body)
		role, _ := body["role"].(string)
		return w.Code, role
	}

	// valid Entra token (designer) via Authorization header
	if code, role := me(map[string]string{"Authorization": "Bearer " + sign([]any{"designer"}, time.Now().Add(time.Hour))}, nil); code != 200 || role != "designer" {
		t.Fatalf("bearer Entra token = %d role %q, want 200 designer", code, role)
	}
	// valid Entra token via the SSO cookie
	if code, role := me(nil, &http.Cookie{Name: authTokenCookie, Value: sign([]any{"admin"}, time.Now().Add(time.Hour))}); code != 200 || role != "admin" {
		t.Fatalf("cookie Entra token = %d role %q, want 200 admin", code, role)
	}
	// expired Entra token in a protected env → fails closed (401)
	if code, _ := me(map[string]string{"Authorization": "Bearer " + sign([]any{"admin"}, time.Now().Add(-time.Hour))}, nil); code != 401 {
		t.Fatalf("expired Entra token = %d, want 401", code)
	}
	// no token in a protected env → 401
	if code, _ := me(nil, nil); code != 401 {
		t.Fatalf("no token in protected env = %d, want 401", code)
	}
}
