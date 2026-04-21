ssh-session-notifier
====================
[![Tests](https://github.com/amanda-wee/ssh-session-notifier/actions/workflows/test.yml/badge.svg)](https://github.com/amanda-wee/ssh-session-notifier/actions/workflows/test.yml)

Receive notifications when SSH sessions open or close so you can act on an unexpected login.

Notifications do not block or delay logging in and an allowlist keeps expected logins from becoming noise. Supports Discord and ntfy, with a design that makes it easy to add more notification services. Written in Go and compiled to a single binary; used by the author on Ubuntu (amd64) and Raspberry Pi OS (arm64 and armhf).

Overview
--------

`ssh-session-notifier` consists of two subcommands:

`queue` is a PAM hook. It runs at session open and close, reads the PAM environment variables, and writes an event record to a queue in a SQLite database.

`send` reads pending events from the queue and sends them as webhook notifications. It runs on whatever schedule you configure using cron, a systemd timer, or some other scheduler.

Installation
------------

First, download the `ssh-session-notifier` binary and run the sample `install.sh` with root privileges (or you can use it as a guide for an Ansible playbook).

Then, edit the configuration file `/etc/ssh-session-notifier/config.toml`. Refer to the configuration section for an example.

Next, install the PAM hook by editing `/etc/pam.d/sshd` to append:

    session    optional     pam_exec.so /usr/sbin/ssh-session-notifier queue

Then configure a scheduler (cron, systemd timer, etc.) to periodically run `/usr/sbin/ssh-session-notifier send` as frequently as you like (say, every minute). Done!

Configuration
-------------

The `config.toml` configuration file uses [TOML 1.1](https://toml.io/en/v1.1.0) syntax. Here is an example:

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

| Section              | Variable    | Default Value          | Description
|----------------------|-------------|------------------------|------------
| host                 | timezone    | `Etc/UTC`              | TZ identifier for the host's timezone
| host                 | name        | (required)             | Hostname or a recognisable label for the host
| notification         | service     | (required)             | `discord` or `ntfy`
| notification.discord | webhook_url | (required for Discord) | URL of the Discord webhook
| notification.ntfy    | topic_url   | (required for ntfy)    | URL of the ntfy topic
| notification.ntfy    | token       | (optional)             | ntfy access token
| allowlist            | ips         | (optional)             | Array of IP addresses that are allowed to log in without triggering notifications

Note that for the allowlist IP addresses, CIDR notation is not supported yet.

Why use ssh-session-notifier?
-----------------------------

### Isn't prevention better than notification of an intrusion?

Yes, but preventative measures can fail due to misconfiguration, leaked credentials, or even a bug. Being warned of a breach soon after it happens can help you to limit the damage and address the underlying issue.

### There are many simpler scripts that also work as PAM hooks to notify about suspicious logins. Why not use them instead?

These scripts typically send the notification synchronously, which makes sense because you want to be notified as soon as possible. The trouble is that these in-band notifications could encounter a notification service that is slow to respond or even down, blocking the login from completing until the service finally responds or the request times out. This can be worked around by sending the notification in a background process, but if the service is down for the duration of the process, the notification might never get sent, even when the service eventually comes back up.

ssh-session-notifier solves these problems by placing the notifications in a persistent queue that can then be processed out-of-band without affecting the login and without the risk that the notification might be lost.

These scripts also tend to be written for a particular notification service, whereas ssh-session-notifier is designed to work with a variety of different notification services through configuration rather than editing code.

### Why not design `ssh-session-notifier queue` to be invoked from `/etc/ssh/sshrc` instead?

* A user's `~/.ssh/rc` script can override `/etc/ssh/sshrc`, bypassing the invocation of `ssh-session-notifier queue`.
* ssh-session-notifier can be used to determine session length as it notifies on session close too, whereas `/etc/ssh/sshrc` can only inform you of the login event.
* As a PAM hook, ssh-session-notifier isn't strictly limited to notifying about SSH sessions. For example, you could configure `/etc/pam.d/login` such that you will be notified about local logins too.
