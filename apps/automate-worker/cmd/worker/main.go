// Command worker runs the automate-worker Temporal worker (docs/spec/04 §2.3).
package main

import (
	"log/slog"
	"os"

	"github.com/mywork/automate/apps/automate-worker/internal/worker"
	"github.com/mywork/automate/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}

	b := worker.NewBootstrap(cfg)
	logger.Info("automate-worker bootstrap", "summary", b.String())

	// E1-S7: dial Temporal, register the interpreter workflow + activity
	// dispatcher on the task queue, and block until interrupted. Run wraps every
	// failure (including Temporal being unavailable) with context; log and exit
	// non-zero rather than panicking so the container restarts cleanly.
	if err := worker.Run(cfg); err != nil {
		logger.Error("worker run failed", "err", err)
		os.Exit(1)
	}
}
