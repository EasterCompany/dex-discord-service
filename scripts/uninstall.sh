#!/bin/bash

echo "Stopping and disabling the systemd service..."
sudo systemctl stop dex-discord-interface.service
sudo systemctl disable dex-discord-interface.service

echo "Removing the systemd service file..."
sudo rm -f /etc/systemd/system/dex-discord-interface.service
sudo rm -f /etc/systemd/system/multi-user.target.wants/dex-discord-interface.service

echo "Removing the executable..."
sudo rm -f /usr/local/bin/dex-discord-interface

echo "Removing configuration files from root's home directory..."
sudo rm -f /root/Dexter/config/discord.json
sudo rm -f /root/Dexter/config/redis.json
sudo rm -f /root/gcloud/credentials.json

# Remove the directories if they are empty
sudo rmdir /root/Dexter/config 2>/dev/null
sudo rmdir /root/Dexter 2>/dev/null
sudo rmdir /root/gcloud 2>/dev/null

echo "Reloading systemd..."
sudo systemctl daemon-reload

echo "Uninstallation complete."