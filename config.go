package main

import (
	"bytes"
	"fmt"
	"os"
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
		if cfg.Notification.Discord.WebhookURL == "" {
			return nil, fmt.Errorf("webhook_url must be provided for Discord")
		}
	case "ntfy":
		if cfg.Notification.Ntfy.TopicURL == "" {
			return nil, fmt.Errorf("topic_url must be provided for ntfy")
		}
	default:
		return nil, fmt.Errorf("unsupported notification service: %s", cfg.Notification.Service)
	}

	return &cfg, nil
}
