package notifier

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amanda-wee/ssh-session-notifier/session"
)

// Implements the Notifier interface for ntfy notifications.
type ntfyNotifier struct {
	httpClient *http.Client
	hostname   string
	topicURL   string
	token      string
}

// Returns new Notifier for ntfy.
func NewNtfyNotifier(client *http.Client, hostname string, topicURL string, token string) Notifier {
	return &ntfyNotifier{
		httpClient: client,
		hostname:   hostname,
		topicURL:   topicURL,
		token:      token,
	}
}

// Sends the given event as a Discord notification.
func (ntfy *ntfyNotifier) Notify(ctx context.Context, event *session.Event) error {
	payload, err := ntfy.formatPayload(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, ntfy.topicURL, strings.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Title", ntfy.hostname)
	req.Header.Set("Priority", "urgent")
	req.Header.Set("Tags", "rotating_light")
	if ntfy.token != "" {
		req.Header.Set("Authorization", "Bearer "+ntfy.token)
	}
	resp, err := ntfy.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// HTTP 2xx indicates successful notification:
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	if resp.StatusCode != http.StatusTooManyRequests {
		return fmt.Errorf("received HTTP %d error from ntfy", resp.StatusCode)
	}

	// ntfy does not return a Retry-After value, but as of 2026-04-15, the rule is:
	// By default, the server is configured to allow 60 requests per visitor at once,
	// and then refills the allowed requests bucket at a rate of one request per 5 seconds.
	return RateLimitError{
		RetryAfter: 6, // 5+1 seconds to ensure refill
	}
}

// Format the session event as a ntfy POST request payload.
func (ntfy *ntfyNotifier) formatPayload(event *session.Event) (string, error) {
	user := event.User

	remoteHost := event.RemoteHost
	if remoteHost != "" {
		remoteHost = fmt.Sprintf(" (%s)", remoteHost)
	}

	terminal := event.Terminal
	if terminal != "" {
		terminal = fmt.Sprintf("via %s ", terminal)
	}

	service := fmt.Sprintf("(%s)", event.Service)

	eventDateTime := event.SessionDatetime.Format("2006-01-02 15:04:05.000000-07:00")

	var content string

	switch event.Type {
	case "open_session":
		content = fmt.Sprintf(
			"'%s'%s logged in to %s %s%s at %s",
			user, remoteHost, ntfy.hostname, terminal, service, eventDateTime,
		)
	case "close_session":
		content = fmt.Sprintf(
			"'%s'%s logged out from %s %s%s at %s",
			user, remoteHost, ntfy.hostname, terminal, service, eventDateTime,
		)
	default:
		return "", fmt.Errorf("unrecognised event type: %s", event.Type)
	}

	return content, nil
}
