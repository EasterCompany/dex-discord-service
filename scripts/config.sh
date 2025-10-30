#!/bin/bash

DEXTER_DIR="$HOME/Dexter/config"

mkdir -p "$DEXTER_DIR"

# Main config that points to the others
echo '{
  "discord_config": "discord.json",
  "cache_config": "cache.json",
  "bot_config": "bot.json",
  "gcloud_config": "gcloud.json",
  "persona_config": "persona.json"
}' >"$DEXTER_DIR/config.json"

# Discord specific config
echo '{
  "token": "",
  "home_server_id": "",
  "log_channel_id": ""
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

# Bot specific config
echo '{
  "voice_timeout_seconds": 2,
  "audio_ttl_minutes": 10,
  "llm_server_url": "http://localhost:11434/api/chat",
  "engagement_model": "llama3",
  "conversational_model": "llama3"
}' >"$DEXTER_DIR/bot.json"

echo "Boilerplate config files created in $DEXTER_DIR"
