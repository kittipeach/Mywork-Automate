package config

import (
	"errors"
	"testing"
)

func TestLoadFromEnv_Defaults(t *testing.T) {
	// No env set (t.Setenv unsets siblings via clearing) — expect dev defaults.
	for _, k := range []string{"APP_ENV", "HTTP_ADDR", "DATABASE_URL", "TEMPORAL_HOSTPORT", "FILE_STORE", "AUTH_LOCAL_ENABLED"} {
		t.Setenv(k, "")
	}
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Env != EnvDev {
		t.Errorf("Env = %q, want %q", cfg.Env, EnvDev)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.FileStore != FileStoreLocal {
		t.Errorf("FileStore = %q, want local", cfg.FileStore)
	}
	if cfg.AuthLocalEnabled {
		t.Error("AuthLocalEnabled = true, want false by default")
	}
	if cfg.IsProtectedEnv() {
		t.Error("dev must not be a protected env")
	}
}

func TestLoadFromEnv_OverridesAndTrimsBlank(t *testing.T) {
	t.Setenv("APP_ENV", EnvDev)
	t.Setenv("HTTP_ADDR", "   ") // blank -> falls back to default
	t.Setenv("TEMPORAL_HOSTPORT", "temporal:7233")
	t.Setenv("FILE_STORE", "azureblob")
	t.Setenv("AUTH_LOCAL_ENABLED", "true")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("blank HTTP_ADDR should fall back to default, got %q", cfg.HTTPAddr)
	}
	if cfg.TemporalHostPort != "temporal:7233" {
		t.Errorf("TemporalHostPort = %q", cfg.TemporalHostPort)
	}
	if cfg.FileStore != FileStoreAzureBlob {
		t.Errorf("FileStore = %q, want azureblob", cfg.FileStore)
	}
	if !cfg.AuthLocalEnabled {
		t.Error("AuthLocalEnabled should be true")
	}
}

// Security guard: AUTH_LOCAL_ENABLED must be rejected in protected envs.
func TestLoadFromEnv_LocalAuthGuard(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		local   string
		wantErr error
	}{
		{"prod+local rejected", EnvProd, "true", ErrLocalAuthInProtectedEnv},
		{"sit+local rejected", EnvSIT, "true", ErrLocalAuthInProtectedEnv},
		{"uat+local rejected", EnvUAT, "true", ErrLocalAuthInProtectedEnv},
		{"prod+no-local ok", EnvProd, "false", nil},
		{"dev+local ok", EnvDev, "true", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", tt.env)
			t.Setenv("AUTH_LOCAL_ENABLED", tt.local)
			t.Setenv("HTTP_ADDR", ":8080")
			t.Setenv("FILE_STORE", "local")
			_, err := LoadFromEnv()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromEnv_InvalidValues(t *testing.T) {
	tests := []struct {
		name string
		set  map[string]string
	}{
		{"bad bool", map[string]string{"AUTH_LOCAL_ENABLED": "notabool"}},
		{"unknown env", map[string]string{"APP_ENV": "staging"}},
		{"unknown filestore", map[string]string{"FILE_STORE": "s3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", EnvDev)
			t.Setenv("FILE_STORE", "local")
			t.Setenv("AUTH_LOCAL_ENABLED", "false")
			for k, v := range tt.set {
				t.Setenv(k, v)
			}
			if _, err := LoadFromEnv(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestValidate_EmptyHTTPAddr(t *testing.T) {
	c := Config{Env: EnvDev, HTTPAddr: "", FileStore: FileStoreLocal}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty HTTPAddr")
	}
}
