#!/bin/bash

echo "Building dex-discord-interface..."
mkdir -p ./bin
GOOS=linux GOARCH=amd64 go build -o ./bin/dex-discord-interface main.go
echo "Build complete: ./bin/dex-discord-interface"
