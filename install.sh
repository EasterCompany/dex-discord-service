#!/bin/bash

# Stop and disable the old service
sudo systemctl stop dex-discord-interface.service
sudo systemctl disable dex-discord-interface.service

# Remove old files
sudo rm /usr/local/bin/dex-discord-interface
sudo rm /etc/systemd/system/dex-discord-interface.service

# Create Dexter config directory
sudo mkdir -p /root/Dexter

# Copy config file
sudo cp config.json /root/Dexter/config.json

# Create gcloud config directory
sudo mkdir -p /root/gcloud

# Copy credentials file
sudo cp credentials.json /root/gcloud/credentials.json

# Build the Go project
GOOS=linux GOARCH=amd64 go build -o dex-discord-interface main.go

# Move the binary to /usr/local/bin
sudo mv dex-discord-interface /usr/local/bin/

# Create the systemd service file
sudo tee /etc/systemd/system/dex-discord-interface.service > /dev/null <<EOL
[Unit]
Description=Dex Discord Interface
After=network.target

[Service]
User=root
Group=root
WorkingDirectory=/usr/local/bin
ExecStart=/usr/local/bin/dex-discord-interface
Restart=always
Environment="GOOGLE_APPLICATION_CREDENTIALS=/root/gcloud/credentials.json"

[Install]
WantedBy=multi-user.target
EOL

# Reload systemd, enable and start the service
sudo systemctl daemon-reload
sudo systemctl enable dex-discord-interface.service
sudo systemctl start dex-discord-interface.service

# Health check
if systemctl is-active --quiet dex-discord-interface.service; then
  echo "Dex Discord Interface is running."
else
  echo "Error: Dex Discord Interface failed to start."
  sudo journalctl -u dex-discord-interface.service
  exit 1
fi