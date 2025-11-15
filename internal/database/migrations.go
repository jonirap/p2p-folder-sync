package database

import (
	"database/sql"
	"fmt"
)

// Migration represents a database migration
type Migration struct {
	Version int
	Up      func(*sql.DB) error
	Down    func(*sql.DB) error
}

// Migrate applies all pending migrations
func (d *DB) Migrate() error {
	// Create migrations table if it doesn't exist
	createMigrationsTable := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at REAL DEFAULT (unixepoch())
	);
	`
	if _, err := d.db.Exec(createMigrationsTable); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get current version
	var currentVersion int
	err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	// Apply migrations (currently schema is created in initSchema, so no migrations needed yet)
	// This structure is here for future migrations

	return nil
}


