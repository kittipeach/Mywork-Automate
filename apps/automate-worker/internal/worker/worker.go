// Package worker bootstraps the automate-worker Temporal worker
// (docs/spec/04 §2.3). E1-S1 provides the config-driven bootstrap summary;
// E1-S7 adds Run, which dials Temporal, wires the db.query executor into the
// generic interpreter workflow and blocks serving the task queue.
package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/mywork/automate/apps/automate-worker/internal/executors/dbquery"
	"github.com/mywork/automate/apps/automate-worker/internal/interpreter"
	"github.com/mywork/automate/apps/automate-worker/internal/pgxquerier"
	"github.com/mywork/automate/internal/config"
	"github.com/mywork/automate/pkg/masking"
	"github.com/mywork/automate/pkg/secrets"
)

// TaskQueue is the Temporal task queue automate workflows/activities use.
const TaskQueue = "automate-task-queue"

// Namespace is the dedicated Temporal namespace for automate (spec 04 §4).
const Namespace = "automate"

// stmtTimeout bounds a single db.query statement (FR-DB-007); the interpreter's
// per-activity StartToCloseTimeout is the outer Temporal bound on top of this.
const stmtTimeout = 120 * time.Second

// secretsFile is the dev/local secret store read by the file resolver. In
// SIT+ the Key Vault resolver (Workload Identity) slots in behind the same
// caching resolver; that path is deferred until Workload Identity is available.
const secretsFile = "secrets.local.yaml"

// Bootstrap describes the worker's resolved runtime wiring. It is returned
// (rather than acted on) so it can be asserted in tests before any Temporal
// connection is attempted.
type Bootstrap struct {
	TemporalHostPort string
	TaskQueue        string
	FileStore        config.FileStoreKind
}

// NewBootstrap resolves worker wiring from validated config.
func NewBootstrap(cfg config.Config) Bootstrap {
	return Bootstrap{
		TemporalHostPort: cfg.TemporalHostPort,
		TaskQueue:        TaskQueue,
		FileStore:        cfg.FileStore,
	}
}

// String renders a human-readable summary for startup logs.
func (b Bootstrap) String() string {
	return fmt.Sprintf("worker[queue=%s temporal=%s filestore=%s]", b.TaskQueue, b.TemporalHostPort, b.FileStore)
}

// Run wires the real dependencies and serves the automate task queue until the
// process is interrupted (SIGINT/SIGTERM). It builds, in order:
//
//	pgxpool(cfg.DatabaseURL) -> pgxquerier.New(pool, 120s)  — the db.query Querier
//	masking.NewEngine(DefaultRules())                        — fail-closed masking
//	secrets file resolver behind a 5-min caching resolver    — credential lookup
//	interpreter.NewActivities(dbquery.Deps{...})             — the node dispatcher
//	client.Dial{HostPort, Namespace:"automate"} -> worker    — durable execution
//
// then registers FlowWorkflow + the Activities struct and blocks on Run. Every
// failure is wrapped with context; Temporal being unavailable returns a clear
// error rather than panicking.
func Run(cfg config.Config) error {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("worker: build pgx pool: %w", err)
	}
	defer pool.Close()
	querier := pgxquerier.New(pool, stmtTimeout)

	maskingEngine, err := masking.NewEngine(masking.DefaultRules())
	if err != nil {
		return fmt.Errorf("worker: build masking engine: %w", err)
	}

	fileResolver, err := secrets.NewFileResolver(secretsFile)
	if err != nil {
		return fmt.Errorf("worker: build secret resolver: %w", err)
	}
	resolver := secrets.NewCachingResolver(fileResolver, 5*time.Minute, nil)

	activities := interpreter.NewActivities(dbquery.Deps{
		Secrets:   resolver,
		Querier:   querier,
		Masking:   maskingEngine,
		MaskPoint: masking.PointPreview,
	})

	c, err := client.Dial(client.Options{
		HostPort:  cfg.TemporalHostPort,
		Namespace: Namespace,
	})
	if err != nil {
		return fmt.Errorf("worker: dial temporal at %s: %w", cfg.TemporalHostPort, err)
	}
	defer c.Close()

	w := worker.New(c, TaskQueue, worker.Options{})
	w.RegisterWorkflow(interpreter.FlowWorkflow)
	w.RegisterActivity(activities)

	if err := w.Run(worker.InterruptCh()); err != nil {
		return fmt.Errorf("worker: run task queue %s: %w", TaskQueue, err)
	}
	return nil
}
