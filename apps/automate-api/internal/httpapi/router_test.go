package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/mywork/automate/internal/config"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

func doGET(t *testing.T, r http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	var body map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("invalid JSON from %s: %v (%s)", path, err, w.Body.String())
		}
	}
	return w, body
}

func TestRouter_Healthz(t *testing.T) {
	r := NewRouter(config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal})
	w, body := doGET(t, r, "/healthz")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["status"] != "ok" {
		t.Errorf("status body = %v, want ok", body["status"])
	}
}

func TestRouter_Readyz(t *testing.T) {
	r := NewRouter(config.Config{Env: config.EnvSIT, FileStore: config.FileStoreLocal})
	w, body := doGET(t, r, "/readyz")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if body["env"] != config.EnvSIT {
		t.Errorf("env = %v, want sit", body["env"])
	}
}

func TestRouter_AuthConfig(t *testing.T) {
	tests := []struct {
		name          string
		cfg           config.Config
		wantProviders []any
	}{
		{
			name:          "dev with local enabled exposes local",
			cfg:           config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal, AuthLocalEnabled: true},
			wantProviders: []any{"entra", "local"},
		},
		{
			name:          "prod never exposes local even if flag set",
			cfg:           config.Config{Env: config.EnvProd, FileStore: config.FileStoreLocal, AuthLocalEnabled: true},
			wantProviders: []any{"entra"},
		},
		{
			name:          "dev without local flag is entra only",
			cfg:           config.Config{Env: config.EnvDev, FileStore: config.FileStoreLocal, AuthLocalEnabled: false},
			wantProviders: []any{"entra"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(tt.cfg)
			w, body := doGET(t, r, APIBasePath+"/auth/config")
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			got, _ := body["providers"].([]any)
			if len(got) != len(tt.wantProviders) {
				t.Fatalf("providers = %v, want %v", got, tt.wantProviders)
			}
			for i := range got {
				if got[i] != tt.wantProviders[i] {
					t.Errorf("providers[%d] = %v, want %v", i, got[i], tt.wantProviders[i])
				}
			}
		})
	}
}
