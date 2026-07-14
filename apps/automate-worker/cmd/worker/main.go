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

	// E1-S7: connect Temporal client, register interpreter workflow + activity
	// dispatcher on b.TaskQueue, then block on worker.Run(). Until then the
	// bootstrap summary is emitted so the container has a valid entrypoint.
}
