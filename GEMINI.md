# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

**dex-discord-interface** is a lightweight Discord microservice that provides a visual dashboard interface for monitoring and controlling a Discord bot. It's part of the Dexter ecosystem - a collection of plug-and-play microservices for building AI interfaces.

**Core Philosophy:**
- This is an **interface**, not the brain
- Discord log channel = monitoring dashboard (5 persistent panels)
- All data flows through Redis/Valkey for inter-service communication
- Clean slate on every boot (delete old messages, create fresh dashboards)
- Single server connection only (no multi-guild support)

## Architecture

### Fresh Start (October 2025)
This project was completely rebuilt from scratch on the `fresh-start` branch with a clean, focused architecture.

**What This Service Does:**
- Connects to ONE Discord server
- Displays 5 persistent dashboard panels in a log channel
- Captures Discord events → writes to Redis
- Listens for commands from Redis → executes on Discord
- Provides real-time monitoring of Discord activity

**What This Service Does NOT Do:**
- LLM inference (handled by `dex-llm` service)
- Speech-to-text (handled by `dex-stt` service)
- Text-to-speech (handled by `dex-tts` service)
- Complex business logic (keep it thin!)

## Directory Structure

```
.
├── bin/                      # Compiled binaries (gitignored)
├── cache/                    # Redis/Valkey integration (TODO)
├── config/                   # Configuration loading
│   └── config.go            # Loads ~/Dexter/config/discord-interface.json
├── dashboard/               # Dashboard system (5 panels)
│   ├── dashboard.go        # Manager coordinating all panels
│   ├── server.go           # Server/guild info panel
│   ├── logs.go             # Recent logs/errors panel
│   ├── events.go           # Discord events panel
│   ├── messages.go         # Message activity panel
│   ├── voice.go            # Voice connection status panel
│   ├── types.go            # Common interfaces
│   ├── throttle.go         # Update throttling (cache vs API)
│   └── cleanup.go          # Log channel cleanup
├── services/                # Service interfaces (TODO)
│   ├── interfaces.go       # STT, LLM, TTS interfaces
│   └── stubs.go           # Mock implementations
├── scripts/                 # Build/deploy/management scripts
│   ├── build.sh           # Build to ./bin/
│   ├── config.sh          # Create config template
│   ├── install.sh         # Install as systemd service
│   ├── make.sh            # Lint + build + install
│   ├── logs.sh            # View systemd logs
│   └── ...
├── main.go                  # Entry point
└── go.mod                   # Dependencies
```

## Configuration

Single JSON file: `~/Dexter/config/discord-interface.json`

```json
{
  "discord_token": "Bot token here",
  "server_id": "Discord server ID",
  "log_channel_id": "Channel ID for dashboards",
  "redis_addr": "localhost:6379",
  "redis_password": "",
  "redis_db": 0
}
```

Create template: `./scripts/config.sh`

## Dashboard System

### 5 Persistent Panels

1. **Server Dashboard** (60s throttle)
   - Guild/server information
   - Member count, roles, channels
   - Bot uptime
   - Connection status

2. **Logs Dashboard** (30s throttle)
   - Recent errors and warnings
   - System log messages
   - Scrolling list (last N entries)

3. **Events Dashboard** (30s throttle)
   - Discord gateway events
   - User joins/leaves
   - Role changes, etc.

4. **Messages Dashboard** (30s throttle)
   - Recent message activity
   - Command usage
   - Bot responses

5. **Voice Dashboard** (30s throttle)
   - Voice connection status
   - Connected users (SSRCs)
   - Transcription activity
   - Audio processing stats

### Throttling System

**Two-tier updates:**
- **Cache layer:** Updates immediately on every event (in-memory, free)
- **Discord API layer:** Pushes to Discord based on throttle period (rate-limited)

```go
// Cache updates immediately
dashboard.cache.Content = newContent
dashboard.cache.LastUpdate = time.Now()

// API update only if throttle period passed
if time.Since(dashboard.cache.LastAPIUpdate) > throttleDuration {
    session.ChannelMessageEdit(channelID, messageID, cachedContent)
    dashboard.cache.LastAPIUpdate = time.Now()
}
```

**Special cases:**
- `Init()` - Bypasses throttle (creates message)
- `ForceUpdate()` - Bypasses throttle (immediate push)
- `Finalize()` - Bypasses throttle (shutdown message)

### Lifecycle

1. **Boot:**
   - Connect to Discord
   - Clean log channel (delete all messages)
   - Create 5 dashboard panels
   - Force update Server dashboard with real data

2. **Running:**
   - Event handlers update dashboard caches
   - Throttled API updates push to Discord
   - Clean slate dashboard showing current state

3. **Shutdown:**
   - Finalize all dashboards (show offline status)
   - Close Discord connection

## Common Commands

### Development
```bash
./scripts/config.sh          # Create config template
./scripts/build.sh           # Build to ./bin/
./scripts/lint.sh            # Run linter
go run main.go               # Run locally
```

### Deployment
```bash
./scripts/make.sh            # Lint + build + install (requires sudo)
./scripts/logs.sh            # View systemd logs
./scripts/restart.sh         # Restart service
./scripts/stop.sh            # Stop service
./scripts/start.sh           # Start service
./scripts/uninstall.sh       # Remove service
```

### Systemd Service
- Installed to: `/usr/local/bin/dex-discord-interface`
- Config path (root): `/root/Dexter/config/discord-interface.json`
- Service: `dex-discord-interface.service`

## Integration Points

### Redis/Valkey (TODO)
- **Dashboard message IDs:** Persist across restarts
- **Event stream:** Publish Discord events for other services
- **Command queue:** Receive commands from other services
- **State sharing:** Voice connections, user activity, etc.

### Other Dexter Services (TODO)
- **dex-stt:** Send audio → receive transcriptions
- **dex-llm:** Send messages → receive AI responses
- **dex-tts:** Send text → receive audio

Communication protocol TBD (Redis pub/sub? HTTP? gRPC?)

## Key Concepts

### Single Server Design
This interface connects to ONE Discord server only. Multi-guild support is explicitly out of scope.

### Dashboard as Frontend
The Discord log channel IS the user interface. Think of it as a web dashboard rendered in Discord messages.

### Thin Interface Layer
Keep business logic OUT of this service. It's a bridge, not a brain.

### Clean Slate Philosophy
Every boot = fresh start. No lingering state, no old messages. Current reality only.

## Common Modification Points

### Add New Dashboard Panel
1. Create `dashboard/new_panel.go` with Dashboard interface
2. Add to `Manager` struct in `dashboard/dashboard.go`
3. Initialize in `Init()` and finalize in `Shutdown()`
4. Set appropriate throttle duration

### Add Event Handler
1. Create handler in new file (e.g., `handlers/message.go`)
2. Register in `main.go` with `session.AddHandler()`
3. Update relevant dashboard caches in handler
4. Dashboard API updates happen automatically via throttle

### Change Throttle Duration
Edit `ThrottleDuration` in dashboard constructor:
```go
cache: &MessageCache{
    ThrottleDuration: 30 * time.Second, // Change this
}
```

### Add Service Integration
1. Define interface in `services/interfaces.go`
2. Create stub in `services/stubs.go`
3. Wire up in `main.go`
4. Replace stub with real implementation later

## Design Decisions

### Why Clean Slate Every Boot?
- Prevents message clutter
- Forces dashboards to show current state
- Avoids stale data confusion
- Makes debugging easier (fresh start = clean logs)

### Why Throttling Instead of Event-Driven?
- Discord API rate limits
- Reduces API calls by 95%+
- Still feels real-time (30-60s updates)
- Predictable API usage

### Why Single Server?
- Simplifies architecture
- Most bots only need one server
- Easier to monitor and debug
- Can run multiple instances if needed

### Why Separate Dashboard Files?
- Easier to maintain
- Clear separation of concerns
- Each dashboard can evolve independently
- Reduces merge conflicts

## Troubleshooting

### Dashboards not updating
- Check throttle duration (might be waiting)
- Call `ForceUpdate()` to bypass throttle
- Check Discord API rate limits
- Verify session is open

### "Connecting..." stays forever
- Dashboard `Init()` called but not `Update()` or `ForceUpdate()`
- Add `dashboardManager.SomePanel.ForceUpdate()` after init

### Config not found
- Check path: `~/Dexter/config/discord-interface.json`
- Run `./scripts/config.sh` to create template
- For systemd: `/root/Dexter/config/discord-interface.json`

### Build fails
- Run `go mod tidy` to fetch dependencies
- Check Go version (1.21+ recommended)
- Run `./scripts/lint.sh` to check for errors
