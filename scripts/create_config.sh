#!/bin/bash

DEXTER_DIR="$HOME/Dexter/config"

mkdir -p "$DEXTER_DIR"

echo '{
  "token": "",
  "log_server_id": "",
  "log_channel_id": ""
}' > "$DEXTER_DIR/discord.json"

echo '{
  "addr": "localhost:6379"
}' > "$DEXTER_DIR/redis.json"

echo "Boilerplate config files created in $DEXTER_DIR"
