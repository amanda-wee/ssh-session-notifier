package database

import (
	"context"
	"database/sql"
	"errors"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const DefaultDataSourceName = "/var/lib/ssh-session-notifier/session_events.db"

func NewHandle(dataSourceName string) (*sql.DB, error) {
	return sql.Open("sqlite3", dataSourceName)
}

var databaseMigrations = [][]string{
	{
		`CREATE TABLE IF NOT EXISTS session_events (
			id               INTEGER PRIMARY KEY,
			event_type       TEXT NOT NULL,
			user             TEXT NOT NULL,
			remote_host      TEXT NOT NULL,
			terminal         TEXT NOT NULL,
			service          TEXT NOT NULL,
			session_datetime DATETIME NOT NULL,
			locked_at        DATETIME
		);`,
		`CREATE INDEX IF NOT EXISTS idx_session_events_session_datetime
		ON session_events (session_datetime);`,
	},
}

func Init(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS schema_version (
		    version INTEGER NOT NULL
		);`,
	)
	if err != nil {
		return err
	}

	version := 0
	err = db.QueryRowContext(
		ctx, `SELECT version FROM schema_version LIMIT 1;`,
	).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = db.ExecContext(
			ctx, `INSERT INTO schema_version (version) VALUES (0);`,
		)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if version < len(databaseMigrations) {
		if err = migrateDatabase(ctx, db, version); err != nil {
			return err
		}
	}

	return nil
}

func migrateDatabase(ctx context.Context, db *sql.DB, version int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, migration := range databaseMigrations[version:] {
		for _, query := range migration {
			_, err = tx.ExecContext(ctx, query)
			if err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(
		ctx, `UPDATE schema_version SET version = ?;`, len(databaseMigrations),
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}
