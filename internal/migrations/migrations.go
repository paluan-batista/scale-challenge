// Package migrations applies the project's versioned PostgreSQL migrations.
package migrations

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5/pgxpool"

	migrationfiles "scale-challenge/db/migrations"
)

// Apply applies every embedded up migration exactly once.
func Apply(ctx context.Context, database *pgxpool.Pool) error {
	if _, err := database.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	entries, err := fs.ReadDir(migrationfiles.Files, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		var alreadyApplied bool
		if err := database.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, entry.Name()).Scan(&alreadyApplied); err != nil {
			return fmt.Errorf("read migration ledger: %w", err)
		}
		if alreadyApplied {
			continue
		}
		contents, err := migrationfiles.Files.ReadFile(entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		transaction, err := database.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", entry.Name(), err)
		}
		if _, err = transaction.Exec(ctx, string(contents)); err == nil {
			_, err = transaction.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, entry.Name())
		}
		if err != nil {
			_ = transaction.Rollback(ctx)
			return fmt.Errorf("apply migration %s: %w", entry.Name(), err)
		}
		if err := transaction.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}
