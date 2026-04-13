#!/bin/sh

# Convenience script for creating the supporting system user, directories, and
# files, and copying the ssh-session-notifier binary from the current directory
# to an appropriate location.

# Create system user and group
id ssh-notifier > /dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin ssh-notifier

# Create directories
mkdir -p /etc/ssh-session-notifier
mkdir -p /var/lib/ssh-session-notifier
touch /etc/ssh-session-notifier/config.toml
touch /var/lib/ssh-session-notifier/session_events.db

# Set ownership and permissions
chown -R root:ssh-notifier /etc/ssh-session-notifier
chmod 750 /etc/ssh-session-notifier
chmod 640 /etc/ssh-session-notifier/config.toml

chown -R root:ssh-notifier /var/lib/ssh-session-notifier
chmod 750 /var/lib/ssh-session-notifier
chmod 660 /var/lib/ssh-session-notifier/session_events.db

# Install binary
cp ./ssh-session-notifier /usr/local/sbin/
chown root:root /usr/local/sbin/ssh-session-notifier
chmod 755 /usr/local/sbin/ssh-session-notifier
