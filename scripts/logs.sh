#!/bin/bash

N=${1:-100}

# Get the timestamp of the last service start
TIMESTAMP=$(systemctl show -p ActiveEnterTimestamp dex-discord-interface.service | cut -d'=' -f2)

# Show logs since the last service start
journalctl -u dex-discord-interface.service --since "$TIMESTAMP" -n "$N" --no-pager
