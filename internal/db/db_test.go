package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := InitDB(dbPath, 1024)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestInitDBCreatesTables(t *testing.T) {
	db := testDB(t)

	// Check that all expected tables exist.
	tables := []string{"sessions", "messages", "schema_version", "app_config"}
	for _, table := range tables {
		var name string
		err := db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}

	// Check the vec_messages virtual table exists.
	var name string
	err := db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='vec_messages'",
	).Scan(&name)
	if err != nil {
		t.Errorf("virtual table vec_messages not found: %v", err)
	}

	// Check the index exists.
	err = db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_messages_session'",
	).Scan(&name)
	if err != nil {
		t.Errorf("index idx_messages_session not found: %v", err)
	}
}

func TestSchemaVersionIsOne(t *testing.T) {
	db := testDB(t)

	v := SchemaVersion(db)
	if v != 1 {
		t.Errorf("expected schema version 1, got %d", v)
	}
}

func TestWALModeActive(t *testing.T) {
	db := testDB(t)

	var mode string
	err := db.QueryRow("PRAGMA journal_mode").Scan(&mode)
	if err != nil {
		t.Fatalf("failed to query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected journal_mode=wal, got %q", mode)
	}
}

func TestInitDBIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// First init.
	db1, err := InitDB(dbPath, 1024)
	if err != nil {
		t.Fatalf("first InitDB failed: %v", err)
	}
	db1.Close()

	// Second init on same file — should not error.
	db2, err := InitDB(dbPath, 1024)
	if err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer db2.Close()

	v := SchemaVersion(db2)
	if v != 1 {
		t.Errorf("expected schema version 1 after re-init, got %d", v)
	}
}

func TestInitDBCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "new.db")

	db, err := InitDB(dbPath, 512)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	db.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("expected database file to be created")
	}
}
