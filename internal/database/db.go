package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the SQLite database connection
type DB struct {
	db *sql.DB
}

// NewDB creates a new database connection
func NewDB(dbPath string) (*DB, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	database := &DB{db: db}

	// Initialize schema
	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Ensure database file has proper permissions (readable/writable by owner and group)
	if err := os.Chmod(dbPath, 0644); err != nil {
		// Log warning but don't fail - permissions might not be critical in all environments
		fmt.Printf("Warning: failed to set database file permissions: %v\n", err)
	}

	return database, nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// GetDB returns the underlying sql.DB connection
func (d *DB) GetDB() *sql.DB {
	return d.db
}

// initSchema initializes the database schema
func (d *DB) initSchema() error {
	schema := `
	-- File metadata and identifiers
	CREATE TABLE IF NOT EXISTS files (
		file_id TEXT PRIMARY KEY,
		path TEXT NOT NULL,
		checksum TEXT NOT NULL,
		size INTEGER NOT NULL,
		mtime REAL NOT NULL,
		mode INTEGER,
		peer_id TEXT NOT NULL,
		vector_clock TEXT NOT NULL,
		compressed INTEGER DEFAULT 0,
		original_size INTEGER,
		compression_algorithm TEXT,
		created_at REAL DEFAULT (unixepoch()),
		updated_at REAL DEFAULT (unixepoch())
	);

	-- Operation log for recovery and state reconstruction
	CREATE TABLE IF NOT EXISTS operations (
		sequence INTEGER PRIMARY KEY AUTOINCREMENT,
		operation_id TEXT UNIQUE NOT NULL,
		timestamp REAL NOT NULL,
		operation_type TEXT NOT NULL,
		peer_id TEXT NOT NULL,
		vector_clock TEXT NOT NULL,
		acknowledged INTEGER DEFAULT 0,
		persisted INTEGER DEFAULT 0,
		file_id TEXT,
		path TEXT NOT NULL,
		from_path TEXT,
		checksum TEXT,
		size INTEGER,
		mtime REAL,
		mode INTEGER,
		chunk_count INTEGER DEFAULT 0,
		data BLOB,
		compressed INTEGER DEFAULT 0,
		original_size INTEGER,
		compression_algorithm TEXT
	);

	-- Peer registry and connection state
	CREATE TABLE IF NOT EXISTS peers (
		peer_id TEXT PRIMARY KEY,
		address TEXT,
		port INTEGER,
		public_key TEXT,
		certificate TEXT,
		capabilities TEXT,
		last_seen REAL,
		connection_state TEXT DEFAULT 'disconnected',
		trust_level TEXT DEFAULT 'unknown',
		created_at REAL DEFAULT (unixepoch())
	);

	-- Chunk tracking for resumable transfers
	CREATE TABLE IF NOT EXISTS chunks (
		file_id TEXT NOT NULL,
		chunk_id INTEGER NOT NULL,
		chunk_hash TEXT NOT NULL,
		offset INTEGER NOT NULL,
		length INTEGER NOT NULL,
		received INTEGER DEFAULT 0,
		received_at REAL,
		PRIMARY KEY (file_id, chunk_id),
		FOREIGN KEY (file_id) REFERENCES files(file_id)
	);

	-- Configuration overrides
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at REAL DEFAULT (unixepoch())
	);

	-- Indexes for performance
	CREATE INDEX IF NOT EXISTS idx_operations_timestamp ON operations(timestamp);
	CREATE INDEX IF NOT EXISTS idx_operations_file_id ON operations(file_id);
	CREATE INDEX IF NOT EXISTS idx_operations_acknowledged ON operations(acknowledged);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	CREATE INDEX IF NOT EXISTS idx_peers_last_seen ON peers(last_seen);
	CREATE INDEX IF NOT EXISTS idx_chunks_file_id ON chunks(file_id);
	`

	if _, err := d.db.Exec(schema); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// BeginTx starts a new transaction
func (d *DB) BeginTx() (*sql.Tx, error) {
	return d.db.Begin()
}

// Exec executes a query without returning rows
func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

// Query executes a query that returns rows
func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

// QueryRow executes a query that returns a single row
func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.db.QueryRow(query, args...)
}


