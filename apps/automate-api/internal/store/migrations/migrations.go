// Package migrations embeds the numbered SQL migration files and exposes them
// as an fs.FS for the startup migrator (store/postgres.Migrate).
package migrations

import "embed"

// FS holds every *.sql migration, applied in lexical (numbered) order.
//
//go:embed *.sql
var FS embed.FS
