#!/bin/sh

# Convenience script for creating the directories and files, and copying the
# ssh-session-notifier binary from the current directory to an appropriate location.

# Create directories
mkdir -p /etc/ssh-session-notifier
mkdir -p /var/lib/ssh-session-notifier
touch /etc/ssh-session-notifier/config.toml
touch /var/lib/ssh-session-notifier/session_events.db

# Set ownership and permissions
chown -R root:root /etc/ssh-session-notifier
chmod 700 /etc/ssh-session-notifier
chmod 600 /etc/ssh-session-notifier/config.toml

chown -R root:root /var/lib/ssh-session-notifier
chmod 700 /var/lib/ssh-session-notifier
chmod 600 /var/lib/ssh-session-notifier/session_events.db

# Install binary
cp ./ssh-session-notifier /usr/sbin/
chown root:root /usr/sbin/ssh-session-notifier
chmod 755 /usr/sbin/ssh-session-notifier
