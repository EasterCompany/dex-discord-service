#!/bin/bash

# This script is intended to be run with sudo.

echo "Stopping the service..."
systemctl stop dex-discord-interface.service

# --- Executable ---
echo "Installing executable to /usr/local/bin..."
if [ ! -f ./dex-discord-interface ]; then
  echo "Error: executable not found. Please run the build script first."
  exit 1
fi
cp ./dex-discord-interface /usr/local/bin/dex-discord-interface
chown root:root /usr/local/bin/dex-discord-interface
chmod 755 /usr/local/bin/dex-discord-interface

# --- Dexter Config ---
echo "Creating /root/Dexter/config directory..."
mkdir -p /root/Dexter/config

echo "Copying Dexter config files to /root/Dexter/config..."
SOURCE_USER_HOME=$(getent passwd "$SUDO_USER" | cut -d: -f6)

cp "$SOURCE_USER_HOME/Dexter/config/"*.json /root/Dexter/config/
cp "$SOURCE_USER_HOME/Dexter/config/"*.default.json /root/Dexter/config/

ls -l /root/Dexter/config

chown -R root:root /root/Dexter/config

echo "Configuration files and executable installed."

echo "Creating systemd service file..."
cat <<EOT >/etc/systemd/system/dex-discord-interface.service
[Unit]
Description=Dex Discord Interface
After=network.target

[Service]
User=root
Group=root
WorkingDirectory=/usr/local/bin
ExecStart=/usr/local/bin/dex-discord-interface
Restart=always
Environment="GOOGLE_APPLICATION_CREDENTIALS=/root/Dexter/config/gcloud.json"

[Install]
WantedBy=multi-user.target
EOT

# --- Systemd ---
echo "Reloading systemd, enabling and starting the service..."
systemctl daemon-reload
systemctl enable dex-discord-interface.service
systemctl start dex-discord-interface.service

# --- Health Check ---
if systemctl is-active --quiet dex-discord-interface.service; then
  echo "Dex Discord Interface is running."
else
  echo "Error: Dex Discord Interface failed to start."
  journalctl -u dex-discord-interface.service
  exit 1
fi
