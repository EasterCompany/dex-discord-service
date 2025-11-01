#!/bin/bash

set -e

SERVICE_NAME="dex-discord-service"
SERVICE_FILE="$HOME/.config/systemd/user/$SERVICE_NAME.service"
INSTALL_DIR="$HOME/.local/bin"
EXECUTABLE_NAME="dex-discord-service"
EXECUTABLE_PATH="$INSTALL_DIR/$EXECUTABLE_NAME"

echo "Starting user-level installation..."

# --- Stop existing service ---
echo "Stopping any running user service..."
systemctl --user stop "$SERVICE_NAME.service" || true

# --- Create directories ---
echo "Creating installation directories..."
mkdir -p "$INSTALL_DIR"
mkdir -p "$HOME/.config/systemd/user"

# --- Executable ---
echo "Installing executable to $INSTALL_DIR..."
if [ ! -f ./bin/$EXECUTABLE_NAME ]; then
  echo "Error: executable not found. Please run the build script first."
  exit 1
fi
cp "./bin/$EXECUTABLE_NAME" "$EXECUTABLE_PATH"
chmod 755 "$EXECUTABLE_PATH"

# --- Config Check ---
echo "Checking for configuration file..."
CONFIG_PATH="$HOME/Dexter/config/discord-service.json"
if [ ! -f "$CONFIG_PATH" ]; then
    echo "Warning: Configuration file not found at $CONFIG_PATH"
    echo "The service may not start without it. Please run './scripts/config.sh'."
fi

echo "Creating systemd user service file at $SERVICE_FILE..."
cat <<EOT >"$SERVICE_FILE"
[Unit]
Description=Dex Discord Service (User Service)
After=network.target

[Service]
ExecStart=$EXECUTABLE_PATH
WorkingDirectory=%h
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
EOT

# --- Systemd ---
echo "Reloading systemd user daemon, enabling and starting the service..."
systemctl --user daemon-reload
systemctl --user enable "$SERVICE_NAME.service"
systemctl --user start "$SERVICE_NAME.service"

# --- Health Check ---
echo "Verifying service status..."
if systemctl --user is-active --quiet "$SERVICE_NAME.service"; then
  echo "✅ Dex Discord Service is running successfully as a user service."
else
  echo "❌ Error: Dex Discord Service failed to start."
  echo "Run './scripts/logs.sh' to view the logs."
  exit 1
fi