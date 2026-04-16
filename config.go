package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const defaultConfigFilePath = "/etc/ssh-session-notifier/config.toml"

type Config struct {
	Host         HostConfig         `toml:"host"`
	Notification NotificationConfig `toml:"notification"`
	Allowlist    AllowlistConfig    `toml:"allowlist"`
}

type HostConfig struct {
	Timezone string `toml:"timezone"`
	Name     string `toml:"name"`
}

type NotificationConfig struct {
	Service string        `toml:"service"`
	Discord DiscordConfig `toml:"discord"`
	Ntfy    NtfyConfig    `toml:"ntfy"`
}

type DiscordConfig struct {
	WebhookURL string `toml:"webhook_url"`
}

type NtfyConfig struct {
	TopicURL string `toml:"topic_url"`
}

type AllowlistConfig struct {
	IPs []string `toml:"ips"`
}

func newConfigFromFile(configFilePath string) (*Config, error) {
	content, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	_, err = toml.NewDecoder(bytes.NewReader(content)).Decode(&cfg)
	if err != nil {
		return nil, err
	}

	if cfg.Host.Timezone == "" {
		cfg.Host.Timezone = "Etc/UTC"
	}
	_, err = time.LoadLocation(cfg.Host.Timezone)
	if err != nil {
		return nil, err
	}

	switch cfg.Notification.Service {
	case "discord":
		if !strings.HasPrefix(cfg.Notification.Discord.WebhookURL, "https://discord.com/api/webhooks/") {
			return nil, fmt.Errorf("valid webhook_url must be provided for Discord")
		}
	case "ntfy":
		topicURL := cfg.Notification.Ntfy.TopicURL
		if !strings.HasPrefix(topicURL, "https://") && !strings.HasPrefix(topicURL, "http://") {
			return nil, fmt.Errorf("valid topic_url must be provided for ntfy")
		}
	default:
		return nil, fmt.Errorf("unsupported notification service: %s", cfg.Notification.Service)
	}

	return &cfg, nil
}
