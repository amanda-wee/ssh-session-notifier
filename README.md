ssh-session-notifier
====================
[![Tests](https://github.com/amanda-wee/ssh-session-notifier/actions/workflows/test.yml/badge.svg)](https://github.com/amanda-wee/ssh-session-notifier/actions/workflows/test.yml)

Receive notifications when SSH sessions open or close so you can act on an unexpected login.

Notifications do not block or delay logging in and an allowlist keeps expected logins from becoming noise. Supports Discord and ntfy notifications, with more to come. Written in Go as a single binary.

Overview
--------

`ssh-session-notifier` consists of two subcommands:

`queue` is a PAM hook. It runs at session open and close, reads the PAM environment variables, and writes an event record to a queue in a SQLite database. It is invoked by PAM and runs as root.

`send` reads pending events from the queue and sends them as webhook notifications. It runs as an unprivileged system user on whatever schedule you configure using cron, a systemd timer, or some other scheduler.

Installation
------------

First, download the `ssh-session-notifier` binary and run the sample `install.sh` with root privileges (or you can use it as a guide for an Ansible playbook).

Then, edit the configuration file `/etc/ssh-session-notifier/config.toml`. Refer to the configuration section for an example.

Next, install the PAM hook by editing `/etc/pam.d/sshd` to append:

    session    optional     pam_exec.so /usr/sbin/ssh-session-notifier queue

Then configure a scheduler (cron, systemd timer, etc.) to periodically run `/usr/sbin/ssh-session-notifier send` as frequently as you like (say, every minute). Done!

Configuration
-------------

The `config.toml` configuration file uses TOML 1.1 syntax. Here is an example:

    [host]
    timezone = "Pacific/Auckland"
    name = "example.com"

    [notification]
    service = "discord"

    [notification.discord]
    webhook_url = "https://discord.com/api/webhooks/xyz"

    [allowlist]
    ips = ["10.0.0.2", "10.0.0.3"]

### Configuration Variables

| Section              | Variable    | Default    | Description
|----------------------|-------------|------------|------------
| host                 | timezone    | `Etc/UTC`    | TZ identifier for the host's timezone
| host                 | name        | (required) | Hostname or a recognisable label for the host
| notification         | service     | (required) | `discord` or `ntfy`
| notification.discord | webhook_url | (required) | URL of the Discord webhook
| notification.ntfy    | topic_url   | (required) | URL of the ntfy topic
| notification.ntfy    | token       | (optional) | ntfy access token
| allowlist            | ips         | (optional) | Array of IP addresses that are allowed to log in without triggering notifications. CIDR notation is not supported yet.
