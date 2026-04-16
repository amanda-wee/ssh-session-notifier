package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNewConfigFromFile(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		missingFile   bool
		wantErr       bool
		wantErrSubstr string
		validate      func(*testing.T, *Config)
	}{
		{
			name: "full valid config",
			content: `
[host]
timezone = "Pacific/Auckland"
name = "test-server"

[notification]
service = "discord"

[notification.discord]
webhook_url = "https://discord.com/api/webhooks/xyz"

[allowlist]
ips = ["192.168.1.1", "10.0.0.1"]
`,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Host.Timezone != "Pacific/Auckland" {
					t.Errorf("Timezone: got %q, want %q", cfg.Host.Timezone, "Pacific/Auckland")
				}
				if cfg.Host.Name != "test-server" {
					t.Errorf("Hostname: got %q, want %q", cfg.Host.Name, "test-server")
				}
				if cfg.Notification.Service != "discord" {
					t.Errorf("Service: got %q, want %q", cfg.Notification.Service, "discord")
				}
				if cfg.Notification.Discord.WebhookURL != "https://discord.com/api/webhooks/xyz" {
					t.Errorf("WebhookURL: got %q, want %q", cfg.Notification.Discord.WebhookURL, "https://discord.com/api/webhooks/xyz")
				}
				wantIPs := []string{"192.168.1.1", "10.0.0.1"}
				if !slices.Equal(cfg.Allowlist.IPs, wantIPs) {
					t.Errorf("Allowlist.IPs: got %v, want %v", cfg.Allowlist.IPs, wantIPs)
				}
			},
		},
		{
			name: "timezone defaults to Etc/UTC when omitted",
			content: `
[host]
name = "test-server"

[notification]
service = "discord"

[notification.discord]
webhook_url = "https://discord.com/api/webhooks/xyz"
`,
			validate: func(t *testing.T, cfg *Config) {
				if cfg.Host.Timezone != "Etc/UTC" {
					t.Errorf("Timezone: got %q, want %q", cfg.Host.Timezone, "Etc/UTC")
				}
			},
		},
		{
			name: "invalid timezone returns error",
			content: `
[host]
timezone = "Bogus/Timezone"
name = "test-server"

[notification]
service = "discord"

[notification.discord]
webhook_url = "https://discord.com/api/webhooks/xyz"
`,
			wantErr:       true,
			wantErrSubstr: "unknown time zone Bogus/Timezone",
		},
		{
			name: "missing host name returns error",
			content: `
[host]
timezone = "Etc/UTC"

[notification]
service = "discord"

[notification.discord]
webhook_url = "https://discord.com/api/webhooks/xyz"
`,
			wantErr:       true,
			wantErrSubstr: "missing host name",
		},
		{
			name: "unsupported notification service returns error",
			content: `
[host]
name = "test-server"

[notification]
service = "bogus"

[notification.bogus]
webhook_url = "https://hooks.bogus.com/xyz"
`,
			wantErr:       true,
			wantErrSubstr: "unsupported notification service: bogus",
		},
		{
			name: "discord notification service without webhook_url returns error",
			content: `
[host]
name = "test-server"

[notification]
service = "discord"
`,
			wantErr:       true,
			wantErrSubstr: "valid webhook_url must be provided for Discord",
		},
		{
			name: "discord notification service with bogus webhook_url returns error",
			content: `
[host]
name = "test-server"

[notification]
service = "discord"

[notification.discord]
webhook_url = "bogus"
`,
			wantErr:       true,
			wantErrSubstr: "valid webhook_url must be provided for Discord",
		},
		{
			name: "ntfy notification service without topic_url returns error",
			content: `
[host]
name = "test-server"

[notification]
service = "ntfy"
`,
			wantErr:       true,
			wantErrSubstr: "valid topic_url must be provided for ntfy",
		},
		{
			name: "ntfy notification service with bogus topic_url returns error",
			content: `
[host]
name = "test-server"

[notification]
service = "ntfy"

[notification.ntfy]
topic_url = "bogus"
`,
			wantErr:       true,
			wantErrSubstr: "valid topic_url must be provided for ntfy",
		},
		{
			name:        "missing file returns error",
			missingFile: true,
			wantErr:     true,
		},
		{
			name:    "invalid TOML returns error",
			content: `not valid toml ][`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configPath string
			if tt.missingFile {
				configPath = filepath.Join(t.TempDir(), "nonexistent.toml")
			} else {
				configPath = filepath.Join(t.TempDir(), "config.toml")
				if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
					t.Fatalf("could not write config file: %v", err)
				}
			}

			cfg, err := newConfigFromFile(configPath)

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
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.validate != nil {
				tt.validate(t, cfg)
			}
		})
	}
}
