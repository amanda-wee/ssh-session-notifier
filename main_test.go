package main

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amanda-wee/ssh-session-notifier/database"
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

func TestQueue(t *testing.T) {
	validConfig := &Config{
		Host: HostConfig{
			Timezone: "Pacific/Auckland",
			Name:     "test-server",
		},
		Allowlist: AllowlistConfig{
			IPs: []string{"192.168.1.1"},
		},
	}

	tests := []struct {
		name          string
		cfg           *Config
		pamType       string
		pamRhost      string
		pamUser       string
		wantErr       bool
		wantErrSubstr string
		wantRowCount  int
	}{
		{
			name:         "valid event is enqueued",
			cfg:          validConfig,
			pamType:      "open_session",
			pamRhost:     "192.168.1.50",
			pamUser:      "amanda",
			wantRowCount: 1,
		},
		{
			name:         "allowlisted IP is not enqueued",
			cfg:          validConfig,
			pamType:      "open_session",
			pamRhost:     "192.168.1.1",
			pamUser:      "amanda",
			wantRowCount: 0,
		},
		{
			name:         "unrecognised PAM_TYPE is not enqueued",
			cfg:          validConfig,
			pamType:      "other_event",
			pamRhost:     "192.168.1.50",
			pamUser:      "amanda",
			wantRowCount: 0,
		},
		{
			name: "invalid timezone returns error",
			cfg: &Config{
				Host: HostConfig{
					Timezone: "Not/ATimezone",
					Name:     "test-server",
				},
			},
			pamType:       "open_session",
			pamRhost:      "192.168.1.50",
			pamUser:       "amanda",
			wantErr:       true,
			wantErrSubstr: "Not/ATimezone",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			ctx := context.Background()

			t.Setenv("PAM_TYPE", tt.pamType)
			t.Setenv("PAM_RHOST", tt.pamRhost)
			t.Setenv("PAM_USER", tt.pamUser)

			err := queue(ctx, tt.cfg, db)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var rowCount int
			err = db.QueryRowContext(ctx, `SELECT count(*) FROM session_events;`).Scan(&rowCount)
			if err != nil {
				t.Fatalf("could not query row count: %v", err)
			}
			if rowCount != tt.wantRowCount {
				t.Errorf("row count: got %d, want %d", rowCount, tt.wantRowCount)
			}
		})
	}
}

func TestSend(t *testing.T) {
	tests := []struct {
		name          string
		cfg           *Config
		wantErr       bool
		wantErrSubstr string
		needsDB       bool
	}{
		{
			name: "unsupported notification service returns error",
			cfg: &Config{
				Host: HostConfig{
					Name: "test-server",
				},
				Notification: NotificationConfig{
					Service: "example",
				},
			},
			wantErr:       true,
			wantErrSubstr: "unsupported notification service: example",
		},
		{
			name: "discord with empty queue returns nil",
			cfg: &Config{
				Host: HostConfig{
					Name: "test-server",
				},
				Notification: NotificationConfig{
					Service: "discord",
					Discord: DiscordConfig{
						WebhookURL: "https://discord.com/api/webhooks/xyz",
					},
				},
			},
			needsDB: true,
		},
		{
			name: "ntfy with empty queue returns nil",
			cfg: &Config{
				Host: HostConfig{
					Name: "test-server",
				},
				Notification: NotificationConfig{
					Service: "ntfy",
					Ntfy: NtfyConfig{
						Topic: "https://ntfy.sh/mytopic",
					},
				},
			},
			needsDB: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var db *sql.DB
			if tt.needsDB {
				db = newTestDB(t)
			}

			err := send(context.Background(), tt.cfg, db)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				if tt.wantErrSubstr != "" && !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantErrSubstr)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
