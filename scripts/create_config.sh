#!/bin/bash

DEXTER_DIR="$HOME/Dexter/config"

mkdir -p "$DEXTER_DIR"

# Main config that points to the others
echo '{
  "discord_config": "discord.json",
  "cache_config": "cache.json"
}' >"$DEXTER_DIR/config.json"

# Discord specific config
echo '{
  "token": "",
  "home_server_id": "",
  "log_channel_id": "",
  "transcription_channel_id": "",
  "audio_ttl_days": 7
}' >"$DEXTER_DIR/discord.json"

# Cache specific config
echo '{
  "local": {
    "addr": "localhost:6379",
    "username": "",
    "password": "",
    "db": 0
  },
  "cloud": {
    "addr": "",
    "username": "",
    "password": "",
    "db": 0
  }
}' >"$DEXTER_DIR/cache.json"

echo "Boilerplate config files created in $DEXTER_DIR"
