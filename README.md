# Dexter Discord Service

**A Discord bot integration service for the Dexter ecosystem**

Dexter Discord Service provides seamless integration between Discord and the Dexter ecosystem. It monitors Discord events (messages, voice state changes, member joins) and forwards them to the event service for asynchronous processing. The service maintains a persistent WebSocket connection to Discord and automatically handles reconnection to ensure reliable event delivery.

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
2.5.4.main.6a7fd26.2025-11-28-09-27-15.linux-amd64.xyz12345
```

Version format: `major.minor.patch.branch.commit.buildDate.arch.buildHash`

### Service Endpoint

When running in server mode, the service exposes a `/service` endpoint on its configured port that returns detailed information:

```bash
curl http://localhost:8101/service
```

**Example Response:**
```json
{
  "version": {
    "str": "2.5.4.main.6a7fd26.2025-11-28-09-27-15.linux-amd64.xyz12345",
    "obj": {
      "major": "2",
      "minor": "5",
      "patch": "4",
      "branch": "main",
      "commit": "6a7fd26",
      "build_date": "2025-11-28-09-27-15",
      "arch": "linux-amd64",
      "build_hash": "xyz12345"
    }
  },
  "health": {
    "status": "OK",
    "uptime": "1h23m45s",
    "message": "Service is running and connected to Discord"
  },
  "metrics": {
    "messages_received": 1234,
    "events_sent": 4567,
    "discord_reconnects": 2
  }
}
```

For simple version string only:
```bash
curl http://localhost:8101/service?format=version
```

## Discord Service Specifics

### Monitored Events

The service monitors the following Discord events and forwards them to the event service:

- **Message Creation**: User messages posted in channels
- **Voice State Updates**: Users joining or leaving voice channels
- **Guild Member Add**: New users joining the Discord server
- **Service Connection**: Bot connection and reconnection events

### Event Format

Events are sent to the event service as JSON via HTTP POST:

```json
{
  "source": "dex-discord-service",
  "type": "message_posted",
  "message": "username user posted in channel-name channel: Hello world!"
}
```

### Server Mode

Running without arguments starts the HTTP server and Discord bot:

```bash
dex-discord-service
```

The service runs as a systemd user service and maintains a persistent connection to Discord.

## API Endpoints

### POST /post

Send messages or images to Discord channels. This endpoint is protected by service authentication middleware.

**Authentication**: Requires `X-Service-Name` header with a valid service name from the service map (localhost requests bypass authentication).

**Request Body**:
```json
{
  "server_id": "1234567890",      // Optional: Discord Guild/Server ID
  "channel_id": "9876543210",     // Required: Discord Channel ID
  "content": "Hello, World!",     // Optional: Text message (required if no image_url)
  "image_url": "https://..."      // Optional: URL to image to send (required if no content)
}
```

**Response** (Success - 200 OK):
```json
{
  "success": true,
  "message_id": "1234567890123456789",
  "channel_id": "9876543210"
}
```

**Example - Send Text Message**:
```bash
curl -X POST http://localhost:8101/post \
  -H "Content-Type: application/json" \
  -H "X-Service-Name: dex-event-service" \
  -d '{
    "channel_id": "9876543210",
    "content": "Hello from the API!"
  }'
```

**Example - Send Image**:
```bash
curl -X POST http://localhost:8101/post \
  -H "Content-Type: application/json" \
  -H "X-Service-Name: dex-event-service" \
  -d '{
    "channel_id": "9876543210",
    "content": "Check out this image:",
    "image_url": "https://example.com/image.png"
  }'
```

**Error Responses**:
- `400 Bad Request`: Invalid JSON, missing required fields, or failed to fetch image
- `403 Forbidden`: Authentication failed (invalid service name or IP)
- `405 Method Not Allowed`: Non-POST request
- `503 Service Unavailable`: Discord connection not established
