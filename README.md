# Dexter Discord Service

**A Discord bot integration service for the Dexter ecosystem**

Dexter Discord Service connects the Dexter platform to Discord, enabling rich bot interactions, command processing, and real-time event capture. The service runs as a persistent Discord bot client that listens for messages, commands, and events, then integrates them with the broader Dexter ecosystem through the event service and other backend components.

Beyond simple message handling, the service captures comprehensive Discord events including voice state changes, message snapshots, and user interactions. It provides both an HTTP status endpoint for monitoring and a Discord command interface for users, making it a critical bridge between Discord communities and Dexter's AI and automation capabilities.

**Platform Support:** Linux (systemd-based distributions)

## Standard Service Interface

All Dexter services implement a universal interface for version information and health monitoring.

### Version Command

Query the service version using the `version` argument:

```bash
dex-discord-service version
```

**Example Output:**
```
2.5.3.main.cd85a7a.2025-11-28-00-03-42.linux-amd64.itosxfdg
```

Version format: `major.minor.patch.branch.commit.buildDate.arch.buildHash`

### Service Endpoint

When running in server mode, the service exposes a `/service` endpoint on port `8300` that returns detailed information:

```bash
curl http://localhost:8300/service
```

**Example Response:**
```json
{
  "version": {
    "str": "2.5.3.main.cd85a7a.2025-11-28-00-03-42.linux-amd64.itosxfdg",
    "obj": {
      "major": "2",
      "minor": "5",
      "patch": "3",
      "branch": "main",
      "commit": "cd85a7a",
      "build_date": "2025-11-28-00-03-42",
      "arch": "linux-amd64",
      "build_hash": "itosxfdg"
    }
  },
  "health": {
    "status": "OK",
    "uptime": "2h15m30s",
    "message": "Service is running normally"
  },
  "metrics": {
    "messages_processed": 5678,
    "events_processed": 234,
    "snapshots_captured": 89,
    "voice_connections": 12,
    "goroutines": 15,
    "memory_alloc_mb": 24.8
  }
}
```

For simple version string only:
```bash
curl http://localhost:8300/service?format=version
```

## Discord Service Specifics

### Discord Bot Integration

The Discord service connects to Discord using a bot token configured in `~/Dexter/config/discord.json`:

```json
{
  "token": "your-bot-token-here",
  "client_id": "your-client-id"
}
```

### Event Capture

The service automatically captures and processes Discord events:

- **Message Events**: New messages, edits, deletions
- **Voice Events**: State changes, connections, disconnections
- **Snapshots**: Periodic channel message snapshots for context preservation
- **User Interactions**: Reactions, mentions, commands

All captured events are forwarded to the event service for processing by registered handlers.

### Discord Commands

Users interact with the bot through Discord slash commands (configured per-server):

- Custom commands are defined in the codebase
- Commands integrate with Dexter's backend services
- Responses can be ephemeral or public

### Server Mode

Running without arguments starts both the Discord bot client and HTTP server on port `8300`:

```bash
dex-discord-service
```

The service runs as a systemd user service, maintaining a persistent connection to Discord and continuously monitoring for events.
