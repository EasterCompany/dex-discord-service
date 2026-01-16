# Dexter Discord Service

**The bridge between Discord and the Dexter ecosystem.**

The `dex-discord-service` is a comprehensive gateway that integrates Discord with the Dexter platform. It functions as both an event producer (listening to Discord) and an event consumer (acting on commands/messages), enabling two-way communication between users and the system.

## 🏗 Architecture

The service operates on a dual-channel model:

1.  **Gateway Connection (Ingress)**: Maintains a persistent WebSocket connection to the Discord Gateway.
    - **Monitors**: Message creation, voice state updates, member joins.
    - **Forwards**: Converts these activities into Dexter Events and sends them to the `dex-event-service` for asynchronous processing.
2.  **API Server (Egress)**: Exposes a REST API allowing other services to perform actions on Discord.
    - **Capabilities**: Sending text messages, uploading images, and serving audio assets.

## 🚀 Tech Stack

- **Language:** Go 1.24
- **Discord Lib:** `discordgo`
- **Audio:** `layeh.com/gopus` (Opus encoding)
- **Storage:** Redis (via `go-redis/v9`)
- **Communication:** HTTP (REST) & WebSocket (Gateway)

## 🔌 Ports & Networking

- **Port:** `8300` (Default)
- **Host:** `0.0.0.0` (Binds to all interfaces)

## 🛠 Prerequisites

- **Go 1.24+**
- **dex-cli** (Installed and configured)
- **dex-event-service** (Running, for event forwarding)
- **Discord Bot Token** (Configured in `options.json`)

## 📦 Getting Started

The recommended way to manage this service is via the `dex-cli`.

### 1. Configure

Ensure `~/Dexter/config/options.json` contains valid Discord credentials:

```json
{
  "discord": {
    "token": "YOUR_BOT_TOKEN",
    "server_id": "YOUR_GUILD_ID",
    "master_user": "ADMIN_USER_ID",
    "default_voice_channel": "CHANNEL_ID"
  }
}
```

### 2. Build

Build the service from source:

```bash
dex build discord
```

### 3. Run

Start the service:

```bash
dex start discord
```

### 4. Verify

Check connection status:

```bash
dex status discord
```

## 📡 API Documentation

The service exposes endpoints for internal communication and health monitoring.

### Base URL

`http://localhost:8300`

### Endpoints

#### 1. Service Health & Info

Get the current status, version, and Discord connection state.

- **GET** `/service`
- **Query Param:** `?format=version` (Optional, returns version string only)

```json
{
  "version": {
    "str": "2.5.4.main.xyz...",
    "obj": { ... }
  },
  "health": {
    "status": "OK",
    "message": "Service is running and connected to Discord"
  },
  "metrics": {
    "messages_received": 120,
    "events_sent": 120
  }
}
```

#### 2. Post Message

Send a message to a Discord channel. Supports text and/or images.

- **POST** `/post`
- **Headers:**
  - `Content-Type: application/json`
  - `X-Service-Name: <calling-service-id>` (Required for auth)

**Payload:**

```json
{
  "channel_id": "9876543210",
  "content": "Hello from Dexter!",
  "image_url": "https://example.com/image.png"
}
```

_Note: Either `content` or `image_url` (or both) is required._

#### 3. Audio Access

Public endpoint to retrieve recorded or processed audio files.

- **GET** `/audio/{filename}`

## ⚙️ Configuration

Configuration is managed centrally by `dex-cli` and stored in `~/Dexter/config/`.

- **`service-map.json`**: Defines the service's port (`8300`).
- **`options.json`**: Critical Discord settings (Token, Guild ID, etc.).

## 🔍 Troubleshooting

**"Discord token not found"**

- Verify that `options.json` exists in `~/Dexter/config/` and has a valid `discord.token` set.

**"Event service not found"**

- Ensure `dex-event-service` is defined in `service-map.json` and is currently running. The bot cannot forward events if the event bus is down.

**"Authentication failed" (HTTP 403)**

- When calling `/post`, ensure you provide a valid `X-Service-Name` header matching a registered service ID.
