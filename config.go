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
	General      GeneralConfig      `toml:"general"`
	Notification NotificationConfig `toml:"notification"`
	Allowlist    AllowlistConfig    `toml:"allowlist"`
}

type GeneralConfig struct {
	Timezone string `toml:"timezone"`
	Hostname string `toml:"hostname"`
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

	if cfg.General.Timezone == "" {
		cfg.General.Timezone = "Etc/UTC"
	}

	if !slices.Contains([]string{"discord"}, cfg.Notification.Service) {
		return nil, fmt.Errorf("unsupported notification service: %s", cfg.Notification.Service)
	}

	return &cfg, nil
}
