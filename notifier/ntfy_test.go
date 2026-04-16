package notifier

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/amanda-wee/ssh-session-notifier/session"
)

func TestNewNtfyNotifier(t *testing.T) {
	client := &http.Client{}
	notifier := NewNtfyNotifier(client, "myserver", "https://ntfy.sh/mytopic", "")

	ntfy := notifier.(*ntfyNotifier)

	if ntfy.hostname != "myserver" {
		t.Errorf("hostname: got %q, want %q", ntfy.hostname, "myserver")
	}
	if ntfy.httpClient != client {
		t.Error("httpClient: got different client than provided")
	}
	if ntfy.topicURL != "https://ntfy.sh/mytopic" {
		t.Errorf("topic: got %q, want %q", ntfy.topicURL, "https://ntfy.sh/mytopic")
	}
}

func TestNtfyNotifier_Notify(t *testing.T) {
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
			name:  "request has correct method and headers",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method: got %q, want POST", r.Method)
				}
				if ct := r.Header.Get("Title"); ct != "myserver" {
					t.Errorf("Title: got %q, want myserver", ct)
				}
				if ct := r.Header.Get("Priority"); ct != "urgent" {
					t.Errorf("Priority: got %q, want urgent", ct)
				}
				if ct := r.Header.Get("Tags"); ct != "rotating_light" {
					t.Errorf("Tags: got %q, want rotating_light", ct)
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
			wantErrMessage: "received HTTP 500 error from ntfy",
		},
		{
			name:  "429 with valid body returns RateLimitError",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
			},
			wantRateLimit: &RateLimitError{RetryAfter: 6},
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

			notifier := &ntfyNotifier{
				hostname:   "myserver",
				topicURL:   server.URL,
				token:      "",
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

func TestNtfyNotifier_Notify_AccessToken(t *testing.T) {
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
			name:  "request with access token has correct method and headers",
			event: validEvent,
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method: got %q, want POST", r.Method)
				}
				if ct := r.Header.Get("Title"); ct != "myserver" {
					t.Errorf("Title: got %q, want myserver", ct)
				}
				if ct := r.Header.Get("Priority"); ct != "urgent" {
					t.Errorf("Priority: got %q, want urgent", ct)
				}
				if ct := r.Header.Get("Tags"); ct != "rotating_light" {
					t.Errorf("Tags: got %q, want rotating_light", ct)
				}
				if ct := r.Header.Get("Authorization"); ct != "Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2" {
					t.Errorf("Authorization: got %q, want Bearer tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2", ct)
				}
				w.WriteHeader(http.StatusNoContent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			notifier := &ntfyNotifier{
				hostname:   "myserver",
				topicURL:   server.URL,
				token:      "tk_AgQdq7mVBoFD37zQVN29RhuMzNIz2",
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

func TestNtfyNotifier_FormatPayload(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC)

	notifier := &ntfyNotifier{hostname: "myserver"}

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
				Terminal:        "/dev/pts/0",
				Service:         "sshd",
				SessionDatetime: fixedTime,
			},
			wantContent: "'amanda' (192.168.1.50) logged in to myserver via /dev/pts/0 (sshd) at 2024-06-15 09:30:00.000000+00:00",
		},
		{
			name: "close session",
			event: &session.Event{
				Type:            "close_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				Terminal:        "/dev/pts/0",
				Service:         "sshd",
				SessionDatetime: fixedTime,
			},
			wantContent: "'amanda' (192.168.1.50) logged out from myserver via /dev/pts/0 (sshd) at 2024-06-15 09:30:00.000000+00:00",
		},
		{
			name: "empty remoteHost is gracefully skipped",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "",
				Terminal:        "/dev/pts/0",
				Service:         "sshd",
				SessionDatetime: fixedTime,
			},
			wantContent: "'amanda' logged in to myserver via /dev/pts/0 (sshd) at 2024-06-15 09:30:00.000000+00:00",
		},
		{
			name: "empty terminal is gracefully skipped",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				Terminal:        "",
				Service:         "login",
				SessionDatetime: fixedTime,
			},
			wantContent: "'amanda' (192.168.1.50) logged in to myserver (login) at 2024-06-15 09:30:00.000000+00:00",
		},
		{
			name: "empty remoteHost and terminal are gracefully skipped",
			event: &session.Event{
				Type:            "open_session",
				User:            "amanda",
				RemoteHost:      "",
				Terminal:        "",
				Service:         "login",
				SessionDatetime: fixedTime,
			},
			wantContent: "'amanda' logged in to myserver (login) at 2024-06-15 09:30:00.000000+00:00",
		},
		{
			name: "unrecognised event type returns error",
			event: &session.Event{
				Type:            "unknown_event",
				User:            "amanda",
				RemoteHost:      "192.168.1.50",
				Terminal:        "/dev/pts/0",
				Service:         "sshd",
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
				Terminal:        "/dev/pts/0",
				Service:         "sshd",
				SessionDatetime: time.Date(2024, 6, 15, 9, 30, 0, 0, time.FixedZone("NZST", 12*60*60)),
			},
			wantContent: "'amanda' (192.168.1.50) logged in to myserver via /dev/pts/0 (sshd) at 2024-06-15 09:30:00.000000+12:00",
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

			if got != tt.wantContent {
				t.Errorf("content:\n got %q\n want %q", got, tt.wantContent)
			}
		})
	}
}
