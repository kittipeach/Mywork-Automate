// Command worker runs the automate-worker Temporal worker (docs/spec/04 §2.3).
// This is the composition root: it wires the real dependencies (pgx pool,
// masking, secrets, the interpreter activities) and serves the task queue.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	temporalworker "go.temporal.io/sdk/worker"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/apps/automate-worker/internal/interpreter"
	"github.com/mywork/automate/apps/automate-worker/internal/pgxquerier"
	"github.com/mywork/automate/apps/automate-worker/internal/worker"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
)

const (
	// stmtTimeout bounds a single db.query statement (FR-DB-007); the
	// interpreter's per-activity StartToCloseTimeout is the outer Temporal bound.
	stmtTimeout = 120 * time.Second
	// secretsFile is the dev/local secret store read by the file resolver. In
	// SIT+ the Key Vault resolver (Workload Identity) slots in behind the same
	// caching resolver.
	secretsFile = "secrets.local.yaml"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	logger.Info("automate-worker bootstrap", "summary", worker.NewBootstrap(cfg).String())

	if err := run(cfg, logger); err != nil {
		logger.Error("worker exited", "err", err)
		os.Exit(1)
	}
}

// run wires the real dependencies and serves the automate task queue until the
// process is interrupted. Every failure is wrapped with context; Temporal being
// unavailable returns a clear error rather than panicking.
func run(cfg config.Config, logger *slog.Logger) error {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("build pgx pool: %w", err)
	}
	defer pool.Close()
	querier := pgxquerier.New(pool, stmtTimeout)

	maskingEngine, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		return fmt.Errorf("build masking engine: %w", err)
	}

	fileResolver, err := secrets.NewFileResolver(secretsFile)
	if err != nil {
		return fmt.Errorf("build secret resolver: %w", err)
	}
	resolver := secrets.NewCachingResolver(fileResolver, 5*time.Minute, nil)

	activities := interpreter.NewActivities(dbquery.Deps{
		Secrets:   resolver,
		Querier:   querier,
		Masking:   maskingEngine,
		MaskPoint: masking.PointPreview,
	})

	c, err := client.Dial(client.Options{HostPort: cfg.TemporalHostPort, Namespace: worker.Namespace})
	if err != nil {
		return fmt.Errorf("dial temporal at %s: %w", cfg.TemporalHostPort, err)
	}
	defer c.Close()

	w := temporalworker.New(c, worker.TaskQueue, temporalworker.Options{})
	w.RegisterWorkflow(interpreter.FlowWorkflow)
	w.RegisterActivity(activities)

	logger.Info("automate-worker serving", "taskQueue", worker.TaskQueue, "temporal", cfg.TemporalHostPort)
	if err := w.Run(temporalworker.InterruptCh()); err != nil {
		return fmt.Errorf("run task queue %s: %w", worker.TaskQueue, err)
	}
	return nil
}
