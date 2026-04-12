package session_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/amanda-wee/ssh-session-notifier/database"
	"github.com/amanda-wee/ssh-session-notifier/session"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := database.NewHandle(filepath.Join(tmpDir, "session_events.db"))
	if err != nil {
		t.Fatalf("could not open test database: %v", err)
	}
	err = database.Init(context.Background(), db)
	if err != nil {
		t.Fatalf("could not init test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func insertEvent(t *testing.T, db *sql.DB, event *session.Event) int {
	t.Helper()
	err := event.Enqueue(context.Background(), db)
	if err != nil {
		t.Fatalf("could not enqueue event: %v", err)
	}
	var id int
	err = db.QueryRowContext(
		context.Background(),
		`SELECT id FROM session_events ORDER BY id DESC LIMIT 1;`,
	).Scan(&id)
	if err != nil {
		t.Fatalf("could not retrieve inserted event ID: %v", err)
	}
	return id
}

func TestNewEventFromEnv(t *testing.T) {
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatalf("could not load timezone: %v", err)
	}

	tests := []struct {
		name         string
		pamType      string
		pamUser      string
		pamRhost     string
		allowlist    []string
		wantNil      bool
		wantType     string
		wantUser     string
		wantRemote   string
		wantLocation *time.Location
	}{
		{
			name:         "open_session returns event with correct fields",
			pamType:      "open_session",
			pamUser:      "amanda",
			pamRhost:     "192.168.1.50",
			allowlist:    []string{},
			wantType:     "open_session",
			wantUser:     "amanda",
			wantRemote:   "192.168.1.50",
			wantLocation: auckland,
		},
		{
			name:         "close_session returns event with correct fields",
			pamType:      "close_session",
			pamUser:      "amanda",
			pamRhost:     "192.168.1.50",
			allowlist:    []string{},
			wantType:     "close_session",
			wantUser:     "amanda",
			wantRemote:   "192.168.1.50",
			wantLocation: auckland,
		},
		{
			name:      "unrecognised PAM_TYPE returns nil",
			pamType:   "other_event",
			pamUser:   "amanda",
			pamRhost:  "192.168.1.50",
			allowlist: []string{},
			wantNil:   true,
		},
		{
			name:      "PAM_RHOST in allowlist returns nil",
			pamType:   "open_session",
			pamUser:   "amanda",
			pamRhost:  "192.168.1.1",
			allowlist: []string{"192.168.1.1", "192.168.1.2"},
			wantNil:   true,
		},
		{
			name:         "PAM_RHOST not in allowlist is included in event",
			pamType:      "open_session",
			pamUser:      "amanda",
			pamRhost:     "192.168.1.50",
			allowlist:    []string{"192.168.1.1", "192.168.1.2"},
			wantType:     "open_session",
			wantUser:     "amanda",
			wantRemote:   "192.168.1.50",
			wantLocation: auckland,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PAM_TYPE", tt.pamType)
			t.Setenv("PAM_USER", tt.pamUser)
			t.Setenv("PAM_RHOST", tt.pamRhost)

			before := time.Now().In(auckland)
			event := session.NewEventFromEnv(auckland, tt.allowlist)
			after := time.Now().In(auckland)

			if tt.wantNil {
				if event != nil {
					t.Errorf("expected nil event, got %+v", event)
				}
				return
			}

			if event == nil {
				t.Fatal("expected event, got nil")
			}

			if event.Type != tt.wantType {
				t.Errorf("Type: got %q, want %q", event.Type, tt.wantType)
			}
			if event.User != tt.wantUser {
				t.Errorf("User: got %q, want %q", event.User, tt.wantUser)
			}
			if event.RemoteHost != tt.wantRemote {
				t.Errorf("RemoteHost: got %q, want %q", event.RemoteHost, tt.wantRemote)
			}
			if event.SessionDatetime.Location().String() != tt.wantLocation.String() {
				t.Errorf("Location: got %q, want %q",
					event.SessionDatetime.Location(), tt.wantLocation)
			}
			if event.SessionDatetime.Before(before) || event.SessionDatetime.After(after) {
				t.Errorf("SessionDatetime %v not between %v and %v",
					event.SessionDatetime, before, after)
			}
		})
	}
}

func TestNewEventFromQueue(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)
	olderTime := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)

	t.Run("empty queue returns nil", func(t *testing.T) {
		db := newTestDB(t)

		event, err := session.NewEventFromQueue(context.Background(), db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event != nil {
			t.Errorf("expected nil event, got %+v", event)
		}
	})

	t.Run("only locked events returns nil", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		e := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id := insertEvent(t, db, e)
		_, err := db.ExecContext(ctx,
			`UPDATE session_events SET locked_at = datetime('now') WHERE id = ?;`, id)
		if err != nil {
			t.Fatalf("could not lock event: %v", err)
		}

		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event != nil {
			t.Errorf("expected nil event, got %+v", event)
		}
	})

	t.Run("returns unlocked event with correct fields", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		e := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id := insertEvent(t, db, e)

		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}

		if event.ID != id {
			t.Errorf("ID: got %d, want %d", event.ID, id)
		}
		if event.Type != e.Type {
			t.Errorf("Type: got %q, want %q", event.Type, e.Type)
		}
		if event.User != e.User {
			t.Errorf("User: got %q, want %q", event.User, e.User)
		}
		if event.RemoteHost != e.RemoteHost {
			t.Errorf("RemoteHost: got %q, want %q", event.RemoteHost, e.RemoteHost)
		}
		if !event.SessionDatetime.Equal(e.SessionDatetime) {
			t.Errorf("SessionDatetime: got %v, want %v", event.SessionDatetime, e.SessionDatetime)
		}
	})

	t.Run("sets locked_at on returned event", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		e := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id := insertEvent(t, db, e)

		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}

		var lockedAt sql.NullString
		err = db.QueryRowContext(ctx,
			`SELECT locked_at FROM session_events WHERE id = ?;`, id,
		).Scan(&lockedAt)
		if err != nil {
			t.Fatalf("could not retrieve locked_at: %v", err)
		}
		if !lockedAt.Valid {
			t.Error("expected locked_at to be set, got NULL")
		}
	})

	t.Run("returns oldest event first", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		older := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: olderTime,
		}
		newer := &session.Event{
			Type: "close_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		olderId := insertEvent(t, db, older)
		insertEvent(t, db, newer)

		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.ID != olderId {
			t.Errorf("expected oldest event (ID %d), got ID %d", olderId, event.ID)
		}
	})

	t.Run("skips locked events and returns unlocked one", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		locked := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: olderTime,
		}
		unlocked := &session.Event{
			Type: "close_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		lockedId := insertEvent(t, db, locked)
		unlockedId := insertEvent(t, db, unlocked)

		_, err := db.ExecContext(ctx,
			`UPDATE session_events SET locked_at = datetime('now') WHERE id = ?;`, lockedId)
		if err != nil {
			t.Fatalf("could not lock event: %v", err)
		}

		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event == nil {
			t.Fatal("expected event, got nil")
		}
		if event.ID != unlockedId {
			t.Errorf("expected unlocked event (ID %d), got ID %d", unlockedId, event.ID)
		}
	})
}

func TestEvent_Enqueue(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		event *session.Event
	}{
		{
			name: "open session event is inserted with correct fields",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
		},
		{
			name: "close session event is inserted with correct fields",
			event: &session.Event{
				Type:            "close_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			ctx := context.Background()

			err := tt.event.Enqueue(ctx, db)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var got session.Event
			err = db.QueryRowContext(
				ctx,
				`SELECT id, event_type, user, remote_host, session_datetime
				FROM session_events LIMIT 1;`,
			).Scan(&got.ID, &got.Type, &got.User, &got.RemoteHost, &got.SessionDatetime)
			if err != nil {
				t.Fatalf("could not retrieve inserted event: %v", err)
			}

			if got.Type != tt.event.Type {
				t.Errorf("event_type: got %q, want %q", got.Type, tt.event.Type)
			}
			if got.User != tt.event.User {
				t.Errorf("user: got %q, want %q", got.User, tt.event.User)
			}
			if got.RemoteHost != tt.event.RemoteHost {
				t.Errorf("remote_host: got %q, want %q", got.RemoteHost, tt.event.RemoteHost)
			}
			if !got.SessionDatetime.Equal(tt.event.SessionDatetime) {
				t.Errorf("session_datetime: got %v, want %v", got.SessionDatetime, tt.event.SessionDatetime)
			}
		})
	}
}

func TestEvent_DeleteRecord(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	t.Run("deletes the correct row by ID", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		event := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id := insertEvent(t, db, event)
		event.ID = id

		err := event.DeleteRecord(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		err = db.QueryRowContext(
			ctx, `SELECT count(*) FROM session_events WHERE id = ?;`, id,
		).Scan(&count)
		if err != nil {
			t.Fatalf("could not query row count: %v", err)
		}
		if count != 0 {
			t.Errorf("expected row to be deleted, got count %d", count)
		}
	})

	t.Run("does not affect other rows", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		event1 := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		event2 := &session.Event{
			Type: "close_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id1 := insertEvent(t, db, event1)
		id2 := insertEvent(t, db, event2)
		event1.ID = id1

		err := event1.DeleteRecord(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var count int
		err = db.QueryRowContext(
			ctx, `SELECT count(*) FROM session_events WHERE id = ?;`, id2,
		).Scan(&count)
		if err != nil {
			t.Fatalf("could not query row count: %v", err)
		}
		if count != 1 {
			t.Errorf("expected other row to be unaffected, got count %d", count)
		}
	})
}

func TestEvent_ReleaseLock(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	t.Run("sets locked_at to NULL for the correct row", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		event := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id := insertEvent(t, db, event)
		event.ID = id

		_, err := db.ExecContext(
			ctx,
			`UPDATE session_events SET locked_at = datetime('now') WHERE id = ?;`, id,
		)
		if err != nil {
			t.Fatalf("could not lock event: %v", err)
		}

		err = event.ReleaseLock(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var lockedAt sql.NullString
		err = db.QueryRowContext(
			ctx, `SELECT locked_at FROM session_events WHERE id = ?;`, id,
		).Scan(&lockedAt)
		if err != nil {
			t.Fatalf("could not retrieve locked_at: %v", err)
		}
		if lockedAt.Valid {
			t.Errorf("expected locked_at to be NULL, got %q", lockedAt.String)
		}
	})

	t.Run("does not affect other rows", func(t *testing.T) {
		db := newTestDB(t)
		ctx := context.Background()

		event1 := &session.Event{
			Type: "open_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		event2 := &session.Event{
			Type: "close_session", User: "amanda",
			RemoteHost: "192.168.1.50", SessionDatetime: fixedTime,
		}
		id1 := insertEvent(t, db, event1)
		id2 := insertEvent(t, db, event2)
		event1.ID = id1

		_, err := db.ExecContext(
			ctx,
			`UPDATE session_events SET locked_at = datetime('now') WHERE id = ? OR id = ?;`, id1, id2,
		)
		if err != nil {
			t.Fatalf("could not lock events: %v", err)
		}

		err = event1.ReleaseLock(ctx, db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var lockedAt sql.NullString
		err = db.QueryRowContext(
			ctx, `SELECT locked_at FROM session_events WHERE id = ?;`, id2,
		).Scan(&lockedAt)
		if err != nil {
			t.Fatalf("could not retrieve locked_at: %v", err)
		}
		if !lockedAt.Valid {
			t.Errorf("expected locked_at to be set on other row, got NULL")
		}
	})
}
