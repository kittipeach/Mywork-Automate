// Package postgres is the pgx implementation of store.Store plus the startup
// migrator and seeder. It is exercised by an integration test (build tag
// `integration`) against a live database, so it is excluded from the
// unit-coverage gate like automate-worker/pgxquerier and the cmd/ entrypoints.
package postgres

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mywork/automate/apps/automate-api/internal/store/migrations"
)

// Migrate applies every embedded *.sql migration in numbered order, exactly
// once, recording applied versions in schema_migrations. It is idempotent:
// re-running is a no-op. Each migration file's basename is its version key.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("postgres: ensure schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("postgres: read migrations dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && hasSQLSuffix(e.Name()) {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		applied, err := migrationApplied(ctx, pool, name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		body, err := migrations.FS.ReadFile(name)
		if err != nil {
			return fmt.Errorf("postgres: read migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			return fmt.Errorf("postgres: apply migration %s: %w", name, err)
		}
		if _, err := pool.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("postgres: record migration %s: %w", name, err)
		}
	}
	return nil
}

func migrationApplied(ctx context.Context, pool *pgxpool.Pool, version string) (bool, error) {
	var exists bool
	err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("postgres: check migration %s: %w", version, err)
	}
	return exists, nil
}

func hasSQLSuffix(name string) bool {
	const suffix = ".sql"
	return len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix
}
