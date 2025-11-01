#!/bin/bash

echo "Building dex-discord-service..."
mkdir -p ./bin
GOOS=linux GOARCH=amd64 go build -o ./bin/dex-discord-service main.go
echo "Build complete: ./bin/dex-discord-service"
