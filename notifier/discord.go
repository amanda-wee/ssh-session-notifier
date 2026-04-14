package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/amanda-wee/ssh-session-notifier/session"
)

// Implements the Notifier interface for Discord notifications.
type discordNotifier struct {
	httpClient *http.Client
	hostname   string
	webhookURL string
}

// Returns new Notifier for Discord.
func NewDiscordNotifier(client *http.Client, hostname string, webhookURL string) Notifier {
	return &discordNotifier{
		httpClient: client,
		hostname:   strings.ReplaceAll(hostname, "`", ""),
		webhookURL: webhookURL,
	}
}

// Sends the given event as a Discord notification.
func (discord *discordNotifier) Notify(ctx context.Context, event *session.Event) error {
	payload, err := discord.formatPayload(event)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, discord.webhookURL, bytes.NewReader(payload),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := discord.httpClient.Do(req)
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
		return fmt.Errorf("received HTTP %d error from Discord", resp.StatusCode)
	}

	var rateLimitRespBody struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rateLimitRespBody); err != nil {
		return err
	}
	return RateLimitError{
		RetryAfter: rateLimitRespBody.RetryAfter,
	}
}

// Format the session event as a Discord webhook request payload.
func (discord *discordNotifier) formatPayload(event *session.Event) ([]byte, error) {
	user := strings.ReplaceAll(event.User, "`", "")

	remoteHost := strings.ReplaceAll(event.RemoteHost, "`", "")
	if remoteHost != "" {
		remoteHost = fmt.Sprintf(" (%s)", remoteHost)
	}

	terminal := strings.ReplaceAll(event.Terminal, "`", "")
	if terminal != "" {
		terminal = fmt.Sprintf("via %s ", terminal)
	}

	service := fmt.Sprintf("(%s)", event.Service)

	eventDateTime := event.SessionDatetime.Format("2006-01-02 15:04:05.000000-07:00")

	var payload struct {
		Username string `json:"username"`
		Content  string `json:"content"`
	}
	payload.Username = discord.hostname

	switch event.Type {
	case "open_session":
		payload.Content = fmt.Sprintf(
			"```- '%s'%s logged in to %s %s%s at %s```",
			user, remoteHost, discord.hostname, terminal, service, eventDateTime,
		)
	case "close_session":
		payload.Content = fmt.Sprintf(
			"```- '%s'%s logged out from %s %s%s at %s```",
			user, remoteHost, discord.hostname, terminal, service, eventDateTime,
		)
	default:
		return nil, fmt.Errorf("unrecognised event type: %s", event.Type)
	}

	return json.Marshal(payload)
}
