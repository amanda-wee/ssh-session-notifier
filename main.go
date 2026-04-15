package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"time"

	"github.com/amanda-wee/ssh-session-notifier/database"
	"github.com/amanda-wee/ssh-session-notifier/notifier"
	"github.com/amanda-wee/ssh-session-notifier/session"
)

const (
	queueTimeout   = 30 * time.Second
	sendTimeout    = 10 * time.Minute
	requestTimeout = 10 * time.Second
)

func main() {
	logger, err := syslog.New(syslog.LOG_ERR|syslog.LOG_AUTH, "ssh-session-notifier")
	if err != nil {
		log.Fatal(err.Error())
	}
	defer logger.Close()

	if !(len(os.Args) == 2 && (os.Args[1] == "queue" || os.Args[1] == "send")) {
		printHelpText()
		os.Exit(2)
	}

	cfg, err := newConfigFromFile(defaultConfigFilePath)
	if err != nil {
		logger.Crit(err.Error())
		os.Exit(1)
	}

	db, err := database.NewHandle(database.DefaultDataSourceName)
	if err != nil {
		logger.Crit(err.Error())
		os.Exit(1)
	}
	defer db.Close()

	ctx := context.Background()

	if err = db.PingContext(ctx); err != nil {
		logger.Crit(err.Error())
		os.Exit(1)
	}

	if err = database.Init(ctx, db); err != nil {
		logger.Crit(err.Error())
		os.Exit(1)
	}

	switch os.Args[1] {
	case "queue":
		queueCtx, cancel := context.WithTimeout(ctx, queueTimeout)
		defer cancel()
		if err = queue(queueCtx, cfg, db); err != nil {
			logger.Crit(err.Error())
			os.Exit(1)
		}
	case "send":
		sendCtx, cancel := context.WithTimeout(ctx, sendTimeout)
		defer cancel()
		if err = send(sendCtx, cfg, db); err != nil {
			logger.Crit(err.Error())
			os.Exit(1)
		}
	}
}

func printHelpText() {
	helpText := `Usage: ssh-session-notifier queue|send

    queue: use as PAM hook to queue SSH session events
    send: send queued SSH session events as notifications`
	fmt.Println(helpText)
}

func queue(ctx context.Context, cfg *Config, db *sql.DB) error {
	loc, err := time.LoadLocation(cfg.Host.Timezone)
	if err != nil {
		return err
	}

	event := session.NewEventFromEnv(loc, cfg.Allowlist.IPs)
	if event != nil {
		if err = event.Enqueue(ctx, db); err != nil {
			return err
		}
	}
	return nil
}

func send(ctx context.Context, cfg *Config, db *sql.DB) error {
	client := &http.Client{
		Timeout: requestTimeout,
	}

	var notificationService notifier.Notifier
	switch cfg.Notification.Service {
	case "discord":
		notificationService = notifier.NewDiscordNotifier(
			client, cfg.Host.Name, cfg.Notification.Discord.WebhookURL,
		)
	case "ntfy":
		notificationService = notifier.NewNtfyNotifier(
			client, cfg.Host.Name, cfg.Notification.Ntfy.TopicURL,
		)
	default:
		return fmt.Errorf("unsupported notification service: %s", cfg.Notification.Service)
	}
	return notifier.SendAll(ctx, notificationService, db)
}
