#!/bin/bash

DEXTER_DIR="$HOME/Dexter/config"

mkdir -p "$DEXTER_DIR"

# Discord service config
echo '{
  "discord_token": "",
  "server_id": "",
  "log_channel_id": "",
  "redis_addr": "localhost:6379",
  "redis_password": "",
  "redis_db": 0,
  "status_port": 8200,
  "services": {
    "llm-service": "http://127.0.0.1:8100/status",
    "stt-service": "http://127.0.0.1:8101/status",
    "tts-service": "http://127.0.0.1:8102/status",
    "http-service": "http://127.0.0.1:8103/status"
  },
  "command_permissions": {
    "default_level": 0,
    "allowed_roles": [],
    "user_whitelist": []
  }
}' >"$DEXTER_DIR/discord-service.json"

echo "Config file created: $DEXTER_DIR/discord-service.json"
echo "Please edit the file and add your Discord token, server ID, and log channel ID"
