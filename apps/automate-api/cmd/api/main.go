// Command api is the automate-api control-plane server (docs/spec/04 §2.2).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	temporalclient "go.temporal.io/sdk/client"

	"github.com/mywork/automate/apps/automate-api/internal/audit"
	auditpg "github.com/mywork/automate/apps/automate-api/internal/audit/postgres"
	"github.com/mywork/automate/apps/automate-api/internal/auth"
	"github.com/mywork/automate/apps/automate-api/internal/httpapi"
	"github.com/mywork/automate/apps/automate-api/internal/notify"
	"github.com/mywork/automate/apps/automate-api/internal/preview"
	"github.com/mywork/automate/apps/automate-api/internal/runner"
	"github.com/mywork/automate/apps/automate-api/internal/scheduler"
	"github.com/mywork/automate/apps/automate-api/internal/store/postgres"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/internal/flowspec"
	"github.com/mywork/automate/pkg/logscrub"
	mailersmtp "github.com/mywork/automate/pkg/mailer/smtp"
	"github.com/mywork/automate/pkg/obs"
	"github.com/mywork/automate/pkg/secrets"
)

// pgxDialer implements httpapi.ConnDialer: it opens a short-lived connection to a
// DSN and pings it, so the test-connection endpoint reports real reachability.
type pgxDialer struct{}

func (pgxDialer) Ping(ctx context.Context, dsn string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(context.Background()) }()
	return conn.Ping(ctx)
}

// SMTP defaults for run-failure notifications (E5-S6). These mirror the worker's
// delivery.email defaults so both connect to the same dev mailhog by default.
const (
	defaultSMTPAddr = "localhost:1025"
	defaultSMTPFrom = "automate@mywork.local"
)

// getenv returns the trimmed env var or a default when unset/blank.
func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// devJWTSecret is used only when AUTH_LOCAL_ENABLED=true and AUTH_JWT_SECRET is
// unset — i.e. local dev. The config guard already prevents local auth from
// running in protected environments, so this default can never apply there.
const devJWTSecret = "dev-only-insecure-jwt-secret-change-me" //nolint:gosec // G101: dev-only default, unreachable in protected envs (config guard)

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
	// Secrets must never reach the logs: wrap the JSON handler with logscrub so
	// every record is scrubbed before it is written. Install as the default so
	// libraries logging via slog are scrubbed too.
	logger := slog.New(logscrub.NewHandler(slog.NewJSONHandler(os.Stdout, nil)))
	slog.SetDefault(logger)

	ctx := context.Background()

	// Observability: install the OTel tracer provider (one trace per run, one
	// span per node). A failure here must not stop the API — log and continue.
	shutdownObs, err := obs.Init(ctx, "automate-api")
	if err != nil {
		logger.Warn("observability init failed; continuing without tracing", "err", err)
	} else {
		defer func() { _ = shutdownObs(context.Background()) }()
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	if cfg.DatabaseURL == "" {
		logger.Error("DATABASE_URL is required (postgres connection string)")
		os.Exit(1)
	}

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

	// Audit trail: the pgx-backed Service when a pool is configured, otherwise a
	// Noop so handlers can Record unconditionally. pool is always non-nil here
	// (DATABASE_URL is required above), but keep the guard so the contract is
	// explicit and testable.
	var auditSvc audit.Service = audit.NewNoop()
	if pool != nil {
		auditSvc = auditpg.New(pool)
	}

	// Temporal is optional at startup: the read endpoints work without it, and
	// POST /flows/{id}/run degrades to 503 until it is reachable. Dial once; on
	// failure log a WARN and pass a nil Runner + Noop Scheduler so the server
	// still starts (publish then no-ops its schedule sync).
	var run runner.Runner
	var sched scheduler.Scheduler = scheduler.Noop{}
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
		sched = scheduler.New(tc)
		logger.Info("temporal client connected", "hostport", cfg.TemporalHostPort, "namespace", flowspec.Namespace)
	}

	authCfg, err := buildAuthConfig(cfg, logger)
	if err != nil {
		logger.Error("auth config failed", "err", err)
		os.Exit(1)
	}

	// Query preview + schema introspection run against a pgx pool behind the
	// preview.Querier seam. For the demo this reuses the API's own pool; a
	// production build would resolve each connection's credentials from Key Vault
	// (pkg/secrets) and build a dedicated, least-privilege pool per connection.
	querier := preview.PoolQuerier{Pool: pool}

	// Run-failure notifications (E5-S6): the SMTP sender points at the same
	// endpoint as the worker's delivery.email (dev mailhog by default). onDone
	// emails the flow's configured recipients when a run finishes "failed".
	sender := mailersmtp.New(getenv("SMTP_ADDR", defaultSMTPAddr), getenv("SMTP_FROM", defaultSMTPFrom))
	notifier := notify.New(sender)

	// External-connection secret store (E6-S1). In dev the file resolver both
	// resolves and saves connection passwords (secrets.local.yaml); in prod this
	// is Key Vault (resolve-only — passwords are provisioned out of band). A
	// missing file is non-fatal: connections can still be created with a
	// pre-provisioned secretRef, but the admin "save password" convenience and
	// the test-connection probe degrade gracefully.
	var connWriter secrets.Writer
	var connResolver secrets.Resolver
	if fr, ferr := secrets.NewFileResolver(getenv("SECRETS_FILE", "secrets.local.yaml")); ferr != nil {
		logger.Warn("secret store unavailable; connection password save/test disabled", "err", ferr)
	} else {
		connWriter, connResolver = fr, fr
	}
	connOpt := httpapi.WithConnDeps(connWriter, connResolver, pgxDialer{})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(cfg, st, run, auditSvc, sched, authCfg, querier, notifier, connOpt),
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
