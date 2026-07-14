// Package worker bootstraps the automate-worker Temporal worker
// (docs/spec/04 §2.3). E1-S1 provides the config-driven bootstrap summary;
// the Temporal client, interpreter workflow and activity registration land in
// E1-S7.
package worker

import (
	"fmt"

	"github.com/mywork/automate/internal/config"
)

// TaskQueue is the Temporal task queue automate workflows/activities use.
const TaskQueue = "automate-task-queue"

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
