#!/bin/bash

# Exit immediately if a command exits with a non-zero status.
set -e

# Define the output directory for the compiled binary
BIN_DIR="./bin"
TOOL_NAME="make-models"

# Create the bin directory if it doesn't exist
mkdir -p "$BIN_DIR"

# Build the Go tool
echo "Building $TOOL_NAME..."
go build -o "$BIN_DIR/$TOOL_NAME" "./cmd/$TOOL_NAME/main.go"

# Run the tool
echo "Running $TOOL_NAME..."
"$BIN_DIR/$TOOL_NAME"
