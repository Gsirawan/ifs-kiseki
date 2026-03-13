// Package db — Database initialization, schema, and migrations.
package db

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func init() {
	sqlite_vec.Auto()
}

// migration represents a single schema migration step.
type migration struct {
	Version     int
	Description string
	Up          func(db *sql.DB) error
}

// migrations is the ordered list of schema migrations.
// V1 is the base schema — created inline by InitDB.
// Future migrations go here starting at V2.
var migrations = []migration{}

// InitDB opens the SQLite database, sets pragmas, creates the schema,
// and runs any pending migrations. embedDim controls the vector dimension
// for the vec_messages virtual table.
func InitDB(dbPath string, embedDim int) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Verify the connection is alive.
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	// Set pragmas.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}

	// Create base schema.
	schema := buildSchema(embedDim)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	// Ensure schema_version tracking is set up.
	if err := ensureSchemaVersion(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema version: %w", err)
	}

	// Run pending migrations.
	if err := RunMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// buildSchema returns the base DDL for IFS-Kiseki.
func buildSchema(embedDim int) string {
	dim := strconv.Itoa(embedDim)
	return `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    started_at INTEGER NOT NULL,
    ended_at INTEGER,
    summary TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, timestamp);

CREATE VIRTUAL TABLE IF NOT EXISTS vec_messages USING vec0(
    message_id TEXT PRIMARY KEY,
    embedding float[` + dim + `] distance_metric=cosine
);

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS app_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`
}

// SchemaVersion reads the current schema version from the database.
// Returns 0 if the table is empty or doesn't exist.
func SchemaVersion(db *sql.DB) int {
	var v int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&v)
	if err != nil {
		return 0
	}
	return v
}

// ensureSchemaVersion makes sure the schema_version table has an initial row.
// For a fresh database, inserts version 1 (base schema).
func ensureSchemaVersion(db *sql.DB) error {
	v := SchemaVersion(db)
	if v == 0 {
		// Fresh database — base schema is V1.
		_, err := db.Exec("INSERT INTO schema_version (version) VALUES (1)")
		return err
	}
	return nil
}

// RunMigrations applies any pending migrations in order.
func RunMigrations(db *sql.DB) error {
	current := SchemaVersion(db)

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}
		log.Printf("[db] running migration V%d: %s", m.Version, m.Description)
		if err := m.Up(db); err != nil {
			return fmt.Errorf("migration V%d (%s): %w", m.Version, m.Description, err)
		}
		if _, err := db.Exec("UPDATE schema_version SET version = ?", m.Version); err != nil {
			return fmt.Errorf("update schema_version to V%d: %w", m.Version, err)
		}
	}

	return nil
}
