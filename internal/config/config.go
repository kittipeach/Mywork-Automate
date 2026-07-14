// Package config loads and validates runtime configuration from the
// environment for automate-api and automate-worker.
//
// Security note (docs/spec/07-security-testing.md §1.2): local user login must
// never be enabled outside development. LoadFromEnv enforces a runtime guard —
// when Env is a protected environment (sit/uat/prod) and AuthLocalEnabled is
// true, loading fails so the process refuses to start.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// FileStoreKind selects the file storage backend.
type FileStoreKind string

const (
	FileStoreLocal     FileStoreKind = "local"
	FileStoreAzureBlob FileStoreKind = "azureblob"
)

// Environment names.
const (
	EnvDev  = "dev"
	EnvSIT  = "sit"
	EnvUAT  = "uat"
	EnvProd = "prod"
)

// ErrLocalAuthInProtectedEnv is returned when AUTH_LOCAL_ENABLED=true is set in
// a protected environment (sit/uat/prod). This is the runtime half of the
// defence-in-depth control; the CI policy check is the other half.
var ErrLocalAuthInProtectedEnv = errors.New("config: AUTH_LOCAL_ENABLED must be false in sit/uat/prod")

// Config is the validated application configuration.
type Config struct {
	Env              string
	HTTPAddr         string
	DatabaseURL      string
	TemporalHostPort string
	FileStore        FileStoreKind
	AuthLocalEnabled bool
}

// protectedEnvs are the environments where local auth is forbidden.
var protectedEnvs = map[string]bool{EnvSIT: true, EnvUAT: true, EnvProd: true}

// LoadFromEnv reads configuration from the process environment, applies
// defaults, validates it, and enforces the local-auth guard.
func LoadFromEnv() (Config, error) {
	cfg := Config{
		Env:              getenvDefault("APP_ENV", EnvDev),
		HTTPAddr:         getenvDefault("HTTP_ADDR", ":8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		TemporalHostPort: getenvDefault("TEMPORAL_HOSTPORT", "localhost:7233"),
		FileStore:        FileStoreKind(getenvDefault("FILE_STORE", string(FileStoreLocal))),
	}

	local, err := parseBool(os.Getenv("AUTH_LOCAL_ENABLED"), false)
	if err != nil {
		return Config{}, fmt.Errorf("config: invalid AUTH_LOCAL_ENABLED: %w", err)
	}
	cfg.AuthLocalEnabled = local

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks invariants and enforces the local-auth guard.
func (c Config) Validate() error {
	switch c.Env {
	case EnvDev, EnvSIT, EnvUAT, EnvProd:
	default:
		return fmt.Errorf("config: unknown APP_ENV %q", c.Env)
	}
	switch c.FileStore {
	case FileStoreLocal, FileStoreAzureBlob:
	default:
		return fmt.Errorf("config: unknown FILE_STORE %q", c.FileStore)
	}
	if c.HTTPAddr == "" {
		return errors.New("config: HTTP_ADDR must not be empty")
	}
	if c.AuthLocalEnabled && protectedEnvs[c.Env] {
		return ErrLocalAuthInProtectedEnv
	}
	return nil
}

// IsProtectedEnv reports whether the environment forbids local auth.
func (c Config) IsProtectedEnv() bool { return protectedEnvs[c.Env] }

func getenvDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func parseBool(v string, def bool) (bool, error) {
	if strings.TrimSpace(v) == "" {
		return def, nil
	}
	return strconv.ParseBool(v)
}
