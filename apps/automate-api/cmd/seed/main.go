// Command seed provisions a local dev admin user (docs/spec/07 §1.2).
// `make seed-dev` runs this. Full argon2id seeding lands in E2-S2; this
// entrypoint refuses to run outside a local-auth-enabled environment so the
// scaffold cannot accidentally seed credentials into a protected env.
package main

import (
	"log/slog"
	"os"

	"github.com/mywork/automate/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.LoadFromEnv()
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	if !cfg.AuthLocalEnabled || cfg.IsProtectedEnv() {
		logger.Error("seed refused: local auth must be enabled and env must not be protected",
			"env", cfg.Env, "authLocalEnabled", cfg.AuthLocalEnabled)
		os.Exit(1)
	}

	logger.Info("seed-dev placeholder — argon2id local admin seeding implemented in E2-S2", "env", cfg.Env)
}
