#!/bin/bash

# This script builds and runs the configuration verification tool.

TOOL_DIR="./cmd/verify-config"
TOOL_NAME="verify-config"

echo "Building the config verifier..."
go build -o "$TOOL_NAME" "$TOOL_DIR"

if [ $? -ne 0 ]; then
  echo "Build failed. Please check for Go compilation errors."
  exit 1
fi

echo "Running the verifier..."
./"$TOOL_NAME"

# Store the exit code of the verifier
EXIT_CODE=$?

echo "Cleaning up..."
rm "$TOOL_NAME"

exit $EXIT_CODE
