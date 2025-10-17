#!/bin/bash

# ==============================================================================
# Dexter Discord Interface Universal Installer & Systemd Setup
#
# This script automates the installation of the Dexter Discord Interface service.
# ==============================================================================

# --- Configuration ---
SRC_PARENT_DIR="$HOME/.dexter"
SRC_DIR="$SRC_PARENT_DIR/dex-discord-interface"
DATA_DIR="$HOME/Dexter/Discord/Users/Messages"
GIT_REPO_URL_SSH="git@github.com:EasterCompany/dex-discord-interface.git"

# --- Script Setup ---
set -e

# --- Helper Functions ---
log() {
  echo "[INFO] $1"
}

error() {
  echo "[ERROR] $1" >&2
  exit 1
}

# --- OS Detection and Dependency Installation ---
install_dependencies() {
  log "Detecting operating system..."
  if ! command -v go &>/dev/null; then
    log "Go is not installed. Installing Go..."
    if command -v yay &>/dev/null; then
      log "Arch-based system detected. Using yay."
      sudo yay -Syu --noconfirm go
    elif command -v apt &>/dev/null; then
      log "Debian/Ubuntu system detected. Using apt."
      sudo apt update
      sudo apt install -y golang-go
    else
      error "Unsupported package manager. Please install Go manually."
    fi
  else
    log "Go is already installed."
  fi
}

# --- Service Management ---
stop_and_disable_service() {
  log "Stopping and disabling any existing Dexter Discord Interface service..."
  if systemctl list-units --type=service | grep -q 'dexter-discord-interface.service'; then
    sudo systemctl stop dexter-discord-interface.service || true
    sudo systemctl disable dexter-discord-interface.service || true
  fi
}

# --- Project Cleanup and Setup ---
setup_project() {
  log "Ensuring parent source directory exists at $SRC_PARENT_DIR..."
  mkdir -p "$SRC_PARENT_DIR"

  log "Setting up source code directory at $SRC_DIR..."
  if [ -d "$SRC_DIR" ]; then
    log "Removing existing dex-discord-interface source directory."
    rm -rf "$SRC_DIR"
  fi

  log "Creating user data directory at $DATA_DIR..."
  mkdir -p "$DATA_DIR"

  log "Cloning repository via SSH into $SRC_DIR..."
  git clone "$GIT_REPO_URL_SSH" "$SRC_DIR"

  log "Building Go application..."
  cd "$SRC_DIR"
  go build -o dexter-discord-interface main.go
  cd -

  log "Initial setup complete."
}

# --- Systemd Service Creation ---
create_systemd_service() {
  log "Creating systemd service file..."
  cat <<EOF | sudo tee /etc/systemd/system/dexter-discord-interface.service
[Unit]
Description=Dexter Discord Interface
After=network.target

[Service]
User=$USER
Group=$(id -gn $USER)
WorkingDirectory=$SRC_DIR
ExecStart=$SRC_DIR/dexter-discord-interface
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

  log "Reloading systemd daemon..."
  sudo systemctl daemon-reload
}

# --- Final Service Start ---
start_service() {
  log "Enabling and starting dexter-discord-interface service..."
  sudo systemctl enable --now dexter-discord-interface.service
  sudo systemctl status dexter-discord-interface.service
}

# --- Main Execution ---
main() {
  log "Starting Dexter Discord Interface installation..."
  install_dependencies
  stop_and_disable_service
  setup_project
  create_systemd_service
  start_service
  log "Installation complete!"
  log "View logs with: journalctl -u dexter-discord-interface -f"
}

main
