#!/bin/bash

echo "Running linting and formatting checks..."
./scripts/lint.sh
if [ $? -ne 0 ]; then
    echo "Linting and formatting checks failed. Exiting."
    exit 1
fi

echo "Building the application..."
./scripts/build.sh

echo "Installing the application..."
sudo ./scripts/install.sh

echo "Cleaning up..."
rm -rf ./bin

echo "Done."
