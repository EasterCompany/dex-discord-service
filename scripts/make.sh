#!/bin/bash

echo "Verifying the application..."
./scripts/verify.sh

echo "Building the application..."
./scripts/build.sh

echo "Installing the application..."
sudo ./scripts/install.sh

echo "Cleaning up..."
rm ./dex-discord-interface

echo "Done."
