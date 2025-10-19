#!/bin/bash

set -e

# Run golangci-lint to format the code
echo "Running golangci-lint fmt..."
golangci-lint fmt .

# Run golangci-lint to check for issues
echo "Running golangci-lint..."
golangci-lint run

echo "Linting and formatting checks passed."
