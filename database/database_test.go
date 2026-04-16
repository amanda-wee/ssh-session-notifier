package database

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewHandle(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDatabasePath := filepath.Join(tmpDir, "session_events.db")
	db, err := NewHandle(tmpDatabasePath)
	if err != nil {
		t.Fatalf("could not open temporary test database: %v", err)
	}
	defer db.Close()

	// Check that the database handle is active:
	if err = db.Ping(); err != nil {
		t.Errorf("could not ping temporary test database: %v", err)
	}

	// Check that the database is in WAL journal mode:
	var mode string
	err = db.QueryRow("PRAGMA journal_mode;").Scan(&mode)
	if err != nil {
		t.Fatalf("could not query for journal mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected journal mode wal, got: %s", mode)
	}
}

func TestInit(t *testing.T) {
	tmpDir := t.TempDir()
	tmpDatabasePath := filepath.Join(tmpDir, "session_events.db")
	db, err := NewHandle(tmpDatabasePath)
	if err != nil {
		t.Fatalf("could not open temporary test database: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	err = Init(ctx, db)
	if err != nil {
		t.Fatalf("could not init temporary test database: %v", err)
	}

	var version int
	err = db.QueryRowContext(ctx, `SELECT version FROM schema_version LIMIT 1;`).Scan(&version)
	if err != nil {
		t.Fatalf("could not retrieve migration version: %v", err)
	}
	if version != len(databaseMigrations) {
		t.Errorf("expected migration version %d; got %d", len(databaseMigrations), version)
	}

	var rowCount int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM schema_version;`).Scan(&rowCount)
	if err != nil {
		t.Fatalf("could not retrieve schema_version row count: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("expected schema_version row count %d; got %d", 1, rowCount)
	}

	err = db.QueryRowContext(ctx, `SELECT count(*) FROM session_events;`).Scan(&rowCount)
	if err != nil {
		t.Fatalf("could not retrieve session_events row count: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("expected session_events row count %d; got %d", 0, rowCount)
	}

	err = Init(ctx, db)
	if err != nil {
		t.Fatalf("could not re-init temporary test database: %v", err)
	}
}
