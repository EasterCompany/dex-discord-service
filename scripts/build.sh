#!/bin/bash

echo "Building dex-discord-interface..."
GOOS=linux GOARCH=amd64 go build -o dex-discord-interface main.go
echo "Build complete."