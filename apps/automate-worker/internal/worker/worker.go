// Package worker holds the automate-worker's shared constants and the
// config-driven bootstrap summary (docs/spec/04 §2.3). The runtime wiring that
// dials Temporal and serves the task queue lives in the composition root
// (cmd/worker), since it can only be exercised by an integration environment.
package worker

import (
	"fmt"

	"github.com/mywork/automate/internal/config"
)

// TaskQueue is the Temporal task queue automate workflows/activities use.
const TaskQueue = "automate-task-queue"

// Namespace is the dedicated Temporal namespace for automate (spec 04 §4).
const Namespace = "automate"

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
