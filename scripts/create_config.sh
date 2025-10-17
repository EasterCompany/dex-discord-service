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

echo '{
  "type": "service_account",
  "project_id": "your-project-id",
  "private_key_id": "your-private-key-id",
  "private_key": "-----BEGIN PRIVATE KEY-----\\n...\\n-----END PRIVATE KEY-----\\n",
  "client_email": "your-client-email",
  "client_id": "your-client-id",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "your-client-x509-cert-url"
}' > "$DEXTER_DIR/gcloud.json"

echo "Boilerplate config files created in $DEXTER_DIR"
