package session

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"time"
)

// Event models a SSH session event.
type Event struct {
	ID              int
	Type            string
	User            string
	RemoteHost      string
	SessionDatetime time.Time
}

// Returns a new session Event populated using PAM environment variables.
func NewEventFromEnv(location *time.Location, allowlistIPs []string) *Event {
	remoteHost := os.Getenv("PAM_RHOST")
	if slices.Contains(allowlistIPs, remoteHost) {
		return nil
	}

	eventType := os.Getenv("PAM_TYPE")
	if eventType != "open_session" && eventType != "close_session" {
		return nil
	}

	user := os.Getenv("PAM_USER")

	return &Event{
		Type:            eventType,
		User:            user,
		RemoteHost:      remoteHost,
		SessionDatetime: time.Now().In(location),
	}
}

// Returns a new session Event from the session events queue in the database,
// locking the record in the database table for processing.
func NewEventFromQueue(ctx context.Context, db *sql.DB) (*Event, error) {
	var event Event

	for {
		// find the unlocked record with the oldest session datetime:
		err := db.QueryRowContext(
			ctx,
			`SELECT id, event_type, user, remote_host, session_datetime
		    FROM session_events WHERE locked_at IS NULL
		    ORDER BY session_datetime ASC LIMIT 1;`,
		).Scan(&event.ID, &event.Type, &event.User, &event.RemoteHost, &event.SessionDatetime)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil // queue is empty or only contains locked session events
			}
			return nil, err
		}

		// attempt to lock the record:
		result, err := db.ExecContext(
			ctx,
			`UPDATE session_events SET locked_at = datetime('now')
		    WHERE id = ? AND locked_at IS NULL;`,
			event.ID,
		)
		if err != nil {
			return nil, err
		}
		// check rows affected to ensure that another process did not acquire
		// the lock in between the select and update:
		n, err := result.RowsAffected()
		if err != nil {
			return nil, err
		}
		if n > 0 {
			break // record locked
		}
	}

	return &event, nil
}

// Inserts the session Event into the session event queue.
func (event *Event) Enqueue(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO session_events
		    (event_type, user, remote_host, session_datetime)
		    VALUES (?, ?, ?, ?);`,
		event.Type, event.User, event.RemoteHost, event.SessionDatetime,
	)
	return err
}

// Deletes the record of the session Event from the session event queue database table.
func (event *Event) DeleteRecord(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		`DELETE FROM session_events WHERE id = ?;`,
		event.ID,
	)
	return err
}

// Releases the lock in the session events queue database table for the session Event.
func (event *Event) ReleaseLock(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(
		ctx,
		`UPDATE session_events SET locked_at = NULL WHERE id = ?;`,
		event.ID,
	)
	return err
}
