#!/bin/bash

set -e

echo "Building debug-cache tool..."
go build -o bin/debug-cache cmd/debug-cache/main.go

echo "Running debug-cache tool..."
./bin/debug-cache
