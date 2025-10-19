#!/bin/bash

echo "Running linting and formatting checks..."
./scripts/lint.sh
if [ $? -ne 0 ]; then
    echo "Linting and formatting checks failed. Exiting make process."
    exit 1
fi

echo "Verifying the application..."
./scripts/verify.sh

echo "Building the application..."
./scripts/build.sh

echo "Installing the application..."
sudo ./scripts/install.sh

echo "Cleaning up..."
rm ./dex-discord-interface

echo "Done."
