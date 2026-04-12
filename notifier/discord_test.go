package notifier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amanda-wee/ssh-session-notifier/session"
)

func TestNewDiscordNotifier(t *testing.T) {
	client := &http.Client{}
	notifier := NewDiscordNotifier(client, "my`server", "https://discord.com/api/webhooks/example")

	dn := notifier.(*discordNotifier)

	if dn.hostname != "myserver" {
		t.Errorf("hostname: got %q, want %q", dn.hostname, "myserver")
	}
	if dn.httpClient != client {
		t.Error("httpClient: got different client than provided")
	}
	if dn.webhookURL != "https://discord.com/api/webhooks/example" {
		t.Errorf("webhookURL: got %q, want %q", dn.webhookURL, "https://discord.com/api/webhooks/example")
	}
}

func TestDiscordNotifier_Notify(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	validEvent := &session.Event{
		Type:            "open_session",
		User:            "amanda",
		RemoteHost:      "192.168.1.50",
		SessionDatetime: fixedTime,
	}

	tests := []struct {
		name           string
		event          *session.Event
		handler        http.HandlerFunc
		wantErr        bool
		wantErrMessage string
		wantRateLimit  *RateLimitError
	}{
		{
			name:  "2xx returns nil",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:  "request has correct content-type and method",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method: got %q, want POST", r.Method)
				}
				if ct := r.Header.Get("Content-Type"); ct != "application/json" {
					t.Errorf("content-type: got %q, want application/json", ct)
				}
				w.WriteHeader(http.StatusNoContent)
			},
		},
		{
			name:  "non-2xx non-429 returns error",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			wantErr:        true,
			wantErrMessage: "received HTTP 500 error from Discord",
		},
		{
			name:  "429 with valid body returns RateLimitError",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `{"retry_after": 2.5}`)
			},
			wantRateLimit: &RateLimitError{RetryAfter: 2.5},
		},
		{
			name:  "429 with invalid body returns decode error",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, `not json`)
			},
			wantErr: true,
		},
		{
			name: "formatPayload error propagates",
			event: &session.Event{
				Type:            "unknown_event",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
			handler:        func(w http.ResponseWriter, r *http.Request) {},
			wantErr:        true,
			wantErrMessage: "unrecognised event type: unknown_event",
		},
		{
			name:  "cancelled context returns error",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			notifier := &discordNotifier{
				hostname:   "myserver",
				webhookURL: server.URL,
				httpClient: server.Client(),
			}

			ctx := context.Background()
			if tt.name == "cancelled context returns error" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			err := notifier.Notify(ctx, tt.event)

			if tt.wantRateLimit != nil {
				var rlErr RateLimitError
				if !errors.As(err, &rlErr) {
					t.Fatalf("expected RateLimitError, got %v", err)
				}
				if rlErr.RetryAfter != tt.wantRateLimit.RetryAfter {
					t.Errorf("RetryAfter: got %v, want %v", rlErr.RetryAfter, tt.wantRateLimit.RetryAfter)
				}
				return
			}

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if tt.wantErrMessage != "" && err.Error() != tt.wantErrMessage {
					t.Errorf("error message: got %q, want %q", err.Error(), tt.wantErrMessage)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDiscordNotifier_FormatPayload(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	notifier := &discordNotifier{hostname: "myserver"}

	tests := []struct {
		name        string
		event       *session.Event
		wantContent string
		wantErr     bool
	}{
		{
			name: "open session",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
			wantContent: "```- 'amanda' logged in to myserver from 192.168.1.50 at 2024-06-15 09:30:00.000000+00:00```",
		},
		{
			name: "close session",
			event: &session.Event{
				Type:            "close_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
			wantContent: "```- 'amanda' logged out of myserver back to 192.168.1.50 at 2024-06-15 09:30:00.000000+00:00```",
		},
		{
			name: "backticks stripped from user and remoteHost",
			event: &session.Event{
				Type:            "open_session",
				User:            "bad`user",
				RemoteHost:      "evil`host",
				SessionDatetime: fixedTime,
			},
			wantContent: "```- 'baduser' logged in to myserver from evilhost at 2024-06-15 09:30:00.000000+00:00```",
		},
		{
			name: "empty user falls back to default",
			event: &session.Event{
				Type:            "open_session",
				User:            "",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
			wantContent: "```- 'unknown user' logged in to myserver from 192.168.1.50 at 2024-06-15 09:30:00.000000+00:00```",
		},
		{
			name: "empty remoteHost falls back to default",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "",
				SessionDatetime: fixedTime,
			},
			wantContent: "```- 'amanda' logged in to myserver from unknown remote host at 2024-06-15 09:30:00.000000+00:00```",
		},
		{
			name: "unrecognised event type returns error",
			event: &session.Event{
				Type:            "unknown_event",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: fixedTime,
			},
			wantErr: true,
		},
		{
			name: "open session non-UTC timezone",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				SessionDatetime: time.Date(2024, 6, 15, 9, 30, 0, 0, time.FixedZone("NZST", 12*60*60)),
			},
			wantContent: "```- 'amanda' logged in to myserver from 192.168.1.50 at 2024-06-15 09:30:00.000000+12:00```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notifier.formatPayload(tt.event)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var payload struct {
				Username string `json:"username"`
				Content  string `json:"content"`
			}
			if err := json.Unmarshal(got, &payload); err != nil {
				t.Fatalf("failed to unmarshal payload: %v", err)
			}

			if payload.Username != notifier.hostname {
				t.Errorf("username: got %q, want %q", payload.Username, notifier.hostname)
			}
			if payload.Content != tt.wantContent {
				t.Errorf("content:\n got %q\n want %q", payload.Content, tt.wantContent)
			}
		})
	}
}
