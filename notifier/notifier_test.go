package notifier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/amanda-wee/ssh-session-notifier/session"
)

type mockNotifier struct {
	responses []error
}

func (m *mockNotifier) Notify(_ context.Context, _ *session.Event) error {
	if len(m.responses) == 1 {
		return m.responses[0]
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return resp
}

func instantSleep(_ context.Context, _ time.Duration) error { return nil }

func TestRateLimitError_Error(t *testing.T) {
	rateLimitError := RateLimitError{
		RetryAfter: 1.23,
	}

	if rateLimitError.Error() != "rate limited, retry after 1.23 seconds" {
		t.Errorf("expected 'rate limited, retry after 1.23 seconds'; got '%s'", rateLimitError.Error())
	}
}

var eventColumns = []string{"id", "event_type", "user", "remote_host", "terminal", "service", "session_datetime"}

func expectDequeue(mock sqlmock.Sqlmock, id int, eventType string) {
	mock.ExpectQuery(`SELECT id, event_type, user, remote_host, terminal, service, session_datetime`).
		WillReturnRows(sqlmock.NewRows(eventColumns).AddRow(
			id, eventType, "amanda", "192.168.1.50", "/dev/pts/0", "sshd",
			time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC),
		))
	mock.ExpectExec(`UPDATE session_events SET locked_at = datetime\('now'\)`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectEmptyQueue(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(`SELECT id, event_type, user, remote_host, terminal, service, session_datetime`).
		WillReturnRows(sqlmock.NewRows(eventColumns))
}

func expectDelete(mock sqlmock.Sqlmock, id int) {
	mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func expectReleaseLock(mock sqlmock.Sqlmock, id int) {
	mock.ExpectExec(`UPDATE session_events SET locked_at = NULL WHERE id = ?`).
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))
}

func TestSendAll(t *testing.T) {
	notifyErr := fmt.Errorf("notify failed")
	queryErr := fmt.Errorf("query failed")

	tests := []struct {
		name          string
		notifier      *mockNotifier
		setupDB       func(sqlmock.Sqlmock)
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:     "empty queue returns nil",
			notifier: &mockNotifier{responses: []error{}},
			setupDB: func(mock sqlmock.Sqlmock) {
				expectEmptyQueue(mock)
			},
		},
		{
			name:     "single event processed successfully",
			notifier: &mockNotifier{responses: []error{nil}},
			setupDB: func(mock sqlmock.Sqlmock) {
				expectDequeue(mock, 1, "open_session")
				expectDelete(mock, 1)
				expectEmptyQueue(mock)
			},
		},
		{
			name:     "multiple events processed in sequence",
			notifier: &mockNotifier{responses: []error{nil, nil}},
			setupDB: func(mock sqlmock.Sqlmock) {
				expectDequeue(mock, 1, "open_session")
				expectDelete(mock, 1)
				expectDequeue(mock, 2, "close_session")
				expectDelete(mock, 2)
				expectEmptyQueue(mock)
			},
		},
		{
			name:     "NewEventFromQueue error is returned",
			notifier: &mockNotifier{responses: []error{}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(`SELECT id, event_type, user, remote_host, terminal, service, session_datetime`).
					WillReturnError(queryErr)
			},
			wantErr:       true,
			wantErrSubstr: "query failed",
		},
		{
			name:     "send error is returned",
			notifier: &mockNotifier{responses: []error{notifyErr}},
			setupDB: func(mock sqlmock.Sqlmock) {
				expectDequeue(mock, 1, "open_session")
				expectReleaseLock(mock, 1)
			},
			wantErr:       true,
			wantErrSubstr: "notify failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()
			defer func() {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Errorf("unfulfilled sqlmock expectations: %v", err)
				}
			}()

			original := sleepWithContext
			defer func() { sleepWithContext = original }()
			sleepWithContext = instantSleep

			tt.setupDB(mock)

			err = SendAll(context.Background(), tt.notifier, db)

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

func TestSend(t *testing.T) {
	fixedEvent := &session.Event{
		ID:              42,
		Type:            "open_session",
		User:            "amanda",
		RemoteHost:      "192.168.1.50",
		Terminal:        "/dev/pts/0",
		Service:         "sshd",
		SessionDatetime: time.Date(2024, 6, 15, 9, 30, 0, 0, time.UTC),
	}

	notifyErr := fmt.Errorf("notify failed")
	deleteErr := fmt.Errorf("delete failed")
	releaseLockErr := fmt.Errorf("release lock failed")

	tests := []struct {
		name          string
		notifier      *mockNotifier
		setupDB       func(sqlmock.Sqlmock)
		setupSleep    func(ctx context.Context, d time.Duration) error
		wantErr       bool
		wantErrSubstr string
	}{
		{
			name:     "success on first attempt deletes record",
			notifier: &mockNotifier{responses: []error{nil}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep: instantSleep,
		},
		{
			name:     "delete failure is returned on success path",
			notifier: &mockNotifier{responses: []error{nil}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
					WithArgs(42).
					WillReturnError(deleteErr)
			},
			setupSleep:    instantSleep,
			wantErr:       true,
			wantErrSubstr: "delete failed",
		},
		{
			name:     "non-rate-limit error releases lock",
			notifier: &mockNotifier{responses: []error{notifyErr}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE session_events SET locked_at = NULL WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep:    instantSleep,
			wantErr:       true,
			wantErrSubstr: "notify failed",
		},
		{
			name:     "non-rate-limit error with release lock failure returns both errors",
			notifier: &mockNotifier{responses: []error{notifyErr}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE session_events SET locked_at = NULL WHERE id = ?`).
					WithArgs(42).
					WillReturnError(releaseLockErr)
			},
			setupSleep:    instantSleep,
			wantErr:       true,
			wantErrSubstr: "release lock failed",
		},
		{
			name: "success after rate limiting deletes record",
			notifier: &mockNotifier{responses: []error{
				RateLimitError{RetryAfter: 2.0},
				nil,
			}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep: instantSleep,
		},
		{
			name: "retry_after below minimum is clamped up",
			notifier: &mockNotifier{responses: []error{
				RateLimitError{RetryAfter: minRetryAfter - 0.5},
				nil,
			}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep: func(_ context.Context, d time.Duration) error {
				want := time.Duration(minRetryAfter * float64(time.Second))
				if d != want {
					return fmt.Errorf("sleep duration: got %v, want %v", d, want)
				}
				return nil
			},
		},
		{
			name: "retry_after above maximum is clamped down",
			notifier: &mockNotifier{responses: []error{
				RateLimitError{RetryAfter: maxRetryAfter + 10.0},
				nil,
			}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`DELETE FROM session_events WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep: func(_ context.Context, d time.Duration) error {
				want := time.Duration(maxRetryAfter * float64(time.Second))
				if d != want {
					return fmt.Errorf("sleep duration: got %v, want %v", d, want)
				}
				return nil
			},
		},
		{
			name: "context cancelled during sleep releases lock",
			notifier: &mockNotifier{responses: []error{
				RateLimitError{RetryAfter: 2.0},
			}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE session_events SET locked_at = NULL WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep: func(_ context.Context, _ time.Duration) error {
				return context.Canceled
			},
			wantErr:       true,
			wantErrSubstr: "context canceled",
		},
		{
			name: "exhausting all attempts releases lock",
			notifier: &mockNotifier{responses: []error{
				RateLimitError{RetryAfter: 2.0},
			}},
			setupDB: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(`UPDATE session_events SET locked_at = NULL WHERE id = ?`).
					WithArgs(42).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			setupSleep:    instantSleep,
			wantErr:       true,
			wantErrSubstr: fmt.Sprintf("exceeded max of %d attempts", maxAttempts),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("failed to create sqlmock: %v", err)
			}
			defer db.Close()
			defer func() {
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Errorf("unfulfilled sqlmock expectations: %v", err)
				}
			}()

			original := sleepWithContext
			defer func() { sleepWithContext = original }()

			tt.setupDB(mock)
			sleepWithContext = tt.setupSleep

			err = send(context.Background(), tt.notifier, db, fixedEvent)

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

func TestSleepWithContext_NormalCompletion(t *testing.T) {
	ctx := context.Background()
	err := sleepWithContext(ctx, 10*time.Millisecond)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestSleepWithContext_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err := sleepWithContext(ctx, 10*time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
