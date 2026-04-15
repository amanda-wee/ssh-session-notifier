package main

import (
	"bytes"
	"fmt"
	"os"
	"slices"

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
	Service    string `toml:"service"`
	WebhookURL string `toml:"webhook_url"`
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

	if !slices.Contains([]string{"discord"}, cfg.Notification.Service) {
		return nil, fmt.Errorf("unsupported notification service: %s", cfg.Notification.Service)
	}

	return &cfg, nil
}
