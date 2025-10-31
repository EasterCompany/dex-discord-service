#!/bin/bash

echo "Stopping and disabling the systemd service..."
sudo systemctl stop dex-discord-interface.service
sudo systemctl disable dex-discord-interface.service

echo "Removing the systemd service file..."
sudo rm -f /etc/systemd/system/dex-discord-interface.service

echo "Removing the executable..."
sudo rm -f /usr/local/bin/dex-discord-interface

echo "Removing configuration files..."
sudo rm -rf /root/Dexter

echo "Reloading systemd..."
sudo systemctl daemon-reload

echo "Uninstallation complete."
