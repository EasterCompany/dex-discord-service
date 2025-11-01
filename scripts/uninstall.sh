#!/bin/bash

set -e

SERVICE_NAME="dex-discord-service"
SERVICE_FILE="$HOME/.config/systemd/user/$SERVICE_NAME.service"
EXECUTABLE_PATH="$HOME/.local/bin/$SERVICE_NAME"

echo "Starting user-level uninstallation..."

# --- Stop and disable service ---
echo "Stopping and disabling the systemd user service..."
systemctl --user stop "$SERVICE_NAME.service" || true
systemctl --user disable "$SERVICE_NAME.service" || true

# --- Remove files ---
echo "Removing files..."
rm -f "$SERVICE_FILE"
rm -f "$EXECUTABLE_PATH"

# --- Systemd ---
echo "Reloading systemd user daemon..."
systemctl --user daemon-reload

echo "✅ Uninstallation complete."