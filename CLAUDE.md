# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Dexter is a Discord bot that provides voice transcription and AI conversational capabilities. It connects to Discord voice channels, transcribes speech using Google Cloud Speech-to-Text, and responds to messages using an LLM (via Ollama).

## Common Commands

### Build
```bash
./scripts/build.sh
# Or directly:
GOOS=linux GOARCH=amd64 go build -o dex-discord-interface main.go
```

### Run Tests
```bash
./scripts/test.sh
# Or directly:
go test ./...
```

### Lint and Format
```bash
./scripts/lint.sh
# Runs golangci-lint fmt and golangci-lint run
```

### Verify Configuration
```bash
go run cmd/verify-config/main.go
```

### Debug Cache
```bash
./scripts/debug-cache.sh
# Or directly:
go run cmd/debug-cache/main.go
```

## Architecture

### Application Initialization Flow

1. **Config Loading** (`config/config.go`): Loads multiple JSON config files from `~/Dexter/config/`:
   - `config.json` - Main config that references other config files
   - Discord credentials, cache connections (local/cloud Redis), bot settings, GCloud credentials, and persona configuration
   - If configs don't exist, creates defaults from `.default.json` files

2. **App Bootstrapping** (`app/app.go`): `NewApp()` initializes all components in order:
   - Discord session
   - Logger (posts to Discord channel)
   - Local and cloud Redis caches
   - STT client (Google Cloud Speech-to-Text)
   - LLM client (Ollama)
   - StateManager and UserManager

3. **Startup Sequence** (`app/app.go` `Run()`):
   - Opens Discord connection and registers event handlers
   - Performs cleanup of old audio and messages
   - Loads guild states from cache
   - Caches all DM message history
   - Posts system status report to Discord log channel

### Core Components

#### Event System (`events/`)

- **Handler** (`events/events.go`): Central event dispatcher
  - Handles `Ready`, `MessageCreate`, `ChannelCreate` events
  - Routes commands (prefixed with `!`)
  - Manages message caching to Redis
  - Triggers LLM processing pipeline

- **StateManager** (`events/state.go`): Manages guild voice connection states
  - Thread-safe `sync.Map` for guild states
  - Tracks active audio streams, connection info, message counts

- **UserManager** (`events/usermanager.go`): Tracks per-user interaction state
  - Three states: `StateIdle`, `StatePending`, `StateStreaming`
  - Manages interruption/cancellation of responses
  - Preserves interrupted conversation context for `CONTINUE` decision

- **LLM Processing** (`events/llm.go`): Multi-stage response pipeline
  1. User sends message → enters `StatePending`, starts typing indicator
  2. Fetches last 50 messages, extracts up to 5 since bot's last message
  3. Calls `GetEngagementDecision()` with engagement model
  4. Based on decision:
     - `REPLY`: Stream conversational response, enter `StateStreaming`
     - `REACT`: Add emoji reaction
     - `STOP`: Cancel current streaming response
     - `CONTINUE`: Resume interrupted response with saved context
     - `IGNORE`: Do nothing
  5. On interruption (new message while streaming), cancels and deletes partial response

- **Commands** (`events/commands.go`): Handles `!` prefixed commands
  - `!join` - Join user's current voice channel
  - `!leave` - Leave voice channel
  - `!clear_dex` - Delete all bot messages in current channel

- **Voice Handling** (`events/voice.go`): Complex voice pipeline
  - Manages WebRTC audio streams per SSRC (user)
  - Converts Opus packets to OGG format
  - Sends to Google STT for transcription
  - Posts transcriptions to Discord channel
  - Handles silence detection and stream finalization

#### LLM Client (`llm/`)

- **Client** (`llm/client.go`): Ollama API client
  - Two models: `engagement_model` (fast decision-making) and `conversational_model` (full responses)
  - `GetEngagementDecision()`: Analyzes recent messages, returns REPLY/REACT/IGNORE/STOP/CONTINUE
  - `GenerateContextBlock()`: Creates system message with timestamp, guild/channel info, online/offline users
  - `CreateOllamaPayload()`: Formats messages for Ollama (user messages + bot as assistant)
  - `StreamChatCompletion()`: Streams response, creating/editing Discord message in real-time

- **Streaming** (`llm/stream.go`): Handles Ollama streaming JSON responses
  - Reads newline-delimited JSON chunks
  - Creates initial Discord message on first chunk
  - Edits message as content accumulates
  - Respects cancellation context

- **Prompts** (`llm/prompts.go`): Contains system message and engagement check templates
  - Uses Go `text/template` with persona configuration
  - Engagement template provides conversation context for decision-making

#### Cache Layer (`cache/`)

- Redis-backed storage with prefixed keys (`dex-discord-interface:`)
- **Message caching**: Stores last 50 messages per channel as JSON in Redis lists
  - Keys: `messages:guild:{guildID}:channel:{channelID}` or `messages:dm:{channelID}`
- **Guild state**: Persists voice connection state
- **DM channels**: Set of all DM channel IDs for bulk caching
- **Audio**: Temporary storage for voice recordings with TTL
- **Cleanup**: Pattern-based deletion with memory usage tracking

#### State Management

- **Guild States** (`guild/guild.go`): Per-guild voice session state
  - Active streams (SSRC → UserStream mapping)
  - Connection metadata (channel ID, message ID, start time)
  - Voice timeout configuration

- **User Streams** (`guild/userstream.go`): Per-user audio stream
  - Buffers RTP packets
  - Converts to OGG format
  - Manages silence detection timers

### Configuration

Configs live in `~/Dexter/config/` and are loaded/created by `config/config.go`:

- **Main Config** (`config.json`): Paths to other config files
- **Discord Config**: Token, home server ID, log channel ID, transcription channel ID
- **Cache Config**: Local and cloud Redis connection details
- **Bot Config**: Voice timeout, audio TTL, LLM server URL, model names
- **GCloud Config**: Service account credentials for Speech-to-Text
- **Persona Config**: Name, role, personality traits, communication style, etc. (used in LLM system prompt)

### Key Dependencies

- **discordgo**: Discord API client and voice support
- **go-redis**: Redis client for caching
- **Google Cloud Speech**: Speech-to-Text API
- **pion/webrtc**: WebRTC for voice packet handling
- **gopsutil**: System resource monitoring

### Important Patterns

1. **Dual Cache Strategy**: Local Redis for fast access, optional cloud Redis for persistence
2. **Message History**: Always maintains last 50 messages per channel in Redis
3. **Engagement Model**: Lightweight LLM call before every potential response to decide whether/how to engage
4. **User State Tracking**: Per-user state prevents simultaneous responses and enables interruption
5. **Voice Stream Lifecycle**: Each user gets separate audio stream, finalized on silence or disconnect
6. **Error Resilience**: Most errors logged but don't crash; continues operation
7. **Boot Reporting**: Posts detailed startup status to Discord log channel

### Common Modification Points

- **Add new command**: Register in `commandHandlers` map in `events/commands.go`
- **Change LLM behavior**: Modify templates in `llm/prompts.go` or persona config
- **Adjust engagement logic**: Update engagement model or decision parsing in `llm/client.go`
- **Modify voice processing**: See `events/voice.go` and `guild/userstream.go`
- **Cache operations**: Extend `cache.Cache` interface in `cache/cache.go`
