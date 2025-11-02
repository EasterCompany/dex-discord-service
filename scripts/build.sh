#!/bin/bash

echo "Building dex-discord-service..."
mkdir -p ~/Dexter/bin
GOOS=linux GOARCH=amd64 go build -o ~/Dexter/bin/dex-discord-service main.go
echo "Build complete: ~/Dexter/bin/dex-discord-service"
