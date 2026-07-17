// Package migrations exposes versioned SQL migrations to the migration runner.
package migrations

import "embed"

// Files contains the migration SQL files.
//
//go:embed *.up.sql
var Files embed.FS
