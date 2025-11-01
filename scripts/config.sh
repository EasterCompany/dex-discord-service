#!/bin/bash

DEXTER_DIR="$HOME/Dexter/config"

mkdir -p "$DEXTER_DIR"

# Discord interface config
echo '{
  "discord_token": "",
  "server_id": "",
  "log_channel_id": "",
  "default_channel_id": "",
  "redis_addr": "localhost:6379",
  "redis_password": "",
  "redis_db": 0
}' >"$DEXTER_DIR/discord-interface.json"

echo "Config file created: $DEXTER_DIR/discord-interface.json"
echo "Please edit the file and add your Discord token, server ID, log channel ID, and default voice channel ID"
