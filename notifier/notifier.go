package notifier

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/amanda-wee/ssh-session-notifier/session"
)

const (
	maxAttempts   = 5    // maximum number of attempts to send a notification
	minRetryAfter = 1.0  // minimum time in seconds to wait before retrying
	maxRetryAfter = 60.0 // maximum time in seconds to wait before retrying
)

// Notifier interface defines the methods for sending notifications.
type Notifier interface {
	// Sends the given event as a notification.
	Notify(context.Context, *session.Event) error
}

// RateLimitError is returned by Notify to indicate that the notification service has rate-limited
// the request. Hence, the lock on the session event should not be released.
type RateLimitError struct {
	// Number of seconds to wait before retrying the request.
	RetryAfter float64
}

// Returns a string representation of the RateLimitError.
func (e RateLimitError) Error() string {
	return fmt.Sprintf("rate limited, retry after %.2f seconds", e.RetryAfter)
}

// Sends all outstanding events to the notification service.
func SendAll(ctx context.Context, notifier Notifier, db *sql.DB) error {
	for {
		event, err := session.NewEventFromQueue(ctx, db)
		if err != nil {
			return err
		}
		// queue is empty or only contains locked events, so nothing to send:
		if event == nil {
			return nil
		}

		err = send(ctx, notifier, db, event)
		if err != nil {
			return err
		}
	}
}

// Sends the given outstanding event to the notification service and updates the session events
// database accordingly.
func send(ctx context.Context, notifier Notifier, db *sql.DB, event *session.Event) error {
	for range maxAttempts {
		err := notifier.Notify(ctx, event)
		if err == nil {
			// successful notification, so delete the session event record:
			return event.DeleteRecord(ctx, db)
		}

		var rateLimitErr RateLimitError
		if !errors.As(err, &rateLimitErr) {
			// failed notification, so release the lock:
			return errors.Join(err, event.ReleaseLock(ctx, db))
		}

		// rate limited, so clamp the retry duration and sleep then retry:
		retryAfter := rateLimitErr.RetryAfter
		if retryAfter < minRetryAfter {
			retryAfter = minRetryAfter
		} else if retryAfter > maxRetryAfter {
			retryAfter = maxRetryAfter
		}
		if err = sleepWithContext(ctx, time.Duration(retryAfter*float64(time.Second))); err != nil {
			return errors.Join(err, event.ReleaseLock(ctx, db))
		}
	}
	// attempts exhausted, so release the lock:
	return errors.Join(
		fmt.Errorf("exceeded max of %d attempts to send notification", maxAttempts),
		event.ReleaseLock(ctx, db),
	)
}

// Sleep that respects context cancellation.
var sleepWithContext = func(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
