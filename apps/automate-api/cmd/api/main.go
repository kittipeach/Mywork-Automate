// Command api is the automate-api control-plane server (docs/spec/04 §2.2).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	temporalclient "go.temporal.io/sdk/client"

	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/httpapi"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/store/postgres"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
)

// devJWTSecret is used only when AUTH_LOCAL_ENABLED=true and AUTH_JWT_SECRET is
// unset — i.e. local dev. The config guard already prevents local auth from
// running in protected environments, so this default can never apply there.
const devJWTSecret = "dev-only-insecure-jwt-secret-change-me"

// buildAuthConfig wires the local-auth service when local auth is enabled and
// permitted. When disabled it returns a zero AuthConfig; the RBAC middleware
// then falls back to the X-Role header / defaultRole.
func buildAuthConfig(cfg config.Config, logger *slog.Logger) (httpapi.AuthConfig, error) {
	if !cfg.AuthLocalEnabled || cfg.IsProtectedEnv() {
		return httpapi.AuthConfig{Logger: logger}, nil
	}

	secret := os.Getenv("AUTH_JWT_SECRET")
	if secret == "" {
		secret = devJWTSecret
		logger.Warn("AUTH_JWT_SECRET unset; using insecure dev default (local auth only)")
	}
	tokens, err := auth.NewTokenIssuer(secret)
	if err != nil {
		return httpapi.AuthConfig{}, err
	}
	users, err := auth.SeedDevAdmin()
	if err != nil {
		return httpapi.AuthConfig{}, err
	}
	svc := auth.NewService(users, tokens, auth.NewLockoutTracker(nil))
	logger.Info("local auth enabled", "devAdmin", auth.DevAdminEmail)
	return httpapi.AuthConfig{Service: svc, Logger: logger}, nil
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required (postgres connection string)")
		os.Exit(1)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	pingCtx, cancelPing := context.WithTimeout(ctx, 10*time.Second)
	defer cancelPing()
	if err := pool.Ping(pingCtx); err != nil {
		logger.Error("database unreachable", "err", err)
		os.Exit(1)
	}

	if err := postgres.Migrate(ctx, pool); err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	if err := postgres.Seed(ctx, pool); err != nil {
		logger.Error("seed failed", "err", err)
		os.Exit(1)
	}

	st := postgres.New(pool)

	// Temporal is optional at startup: the read endpoints work without it, and
	// POST /flows/{id}/run degrades to 503 until it is reachable. Dial once; on
	// failure log a WARN and pass a nil Runner so the server still starts.
	var run runner.Runner
	tc, err := temporalclient.Dial(temporalclient.Options{
		HostPort:  cfg.TemporalHostPort,
		Namespace: flowspec.Namespace,
	})
	if err != nil {
		logger.Warn("temporal unavailable; POST /flows/{id}/run will return 503",
			"hostport", cfg.TemporalHostPort, "err", err)
	} else {
		defer tc.Close()
		run = runner.New(tc)
		logger.Info("temporal client connected", "hostport", cfg.TemporalHostPort, "namespace", flowspec.Namespace)
	}

	authCfg, err := buildAuthConfig(cfg, logger)
	if err != nil {
		logger.Error("auth config failed", "err", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(cfg, st, run, authCfg),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("automate-api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	shutdownSig, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-shutdownSig.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
	}
	logger.Info("automate-api stopped")
}
