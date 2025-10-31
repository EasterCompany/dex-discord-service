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
./scripts/verify.sh
# Or directly:
go run cmd/verify-config/main.go
```

### Debug Cache
```bash
./scripts/debug-cache.sh
# Or directly:
go run cmd/debug-cache/main.go
```

### Make Models
```bash
./scripts/make-models.sh
# Or directly:
go run cmd/make-models/main.go
```

## Architecture

### Application Initialization Flow

1. **Main Entry** (`main.go`): Simple entry point that creates and runs the app
   - Calls `app.NewApp()` to create the application
   - Calls `application.Run()` to start the bot
   - Uses `preinit.NewLogger()` for early error logging if initialization fails

2. **Dependency Injection** (`di/di.go`): `NewContainer()` creates a DI container with all dependencies:
   - Loads all configs via `config.LoadAllConfigs()`
   - Creates Discord session via `session.NewSession()`
   - Initializes logger (posts to Discord channel)
   - Initializes local and cloud Redis caches
   - Initializes STT client (Google Cloud Speech-to-Text)
   - Initializes LLM client (Ollama) with persona config
   - Creates StateManager and UserManager
   - Returns a `Container` struct holding all initialized components

3. **App Bootstrapping** (`app/app.go`): `NewApp()` creates the app with DI container:
   - Calls `di.NewContainer()` to initialize all dependencies
   - Wraps container in an `App` struct

4. **Startup Sequence** (`app/app.go` `Run()`):
   - Creates event handlers: `Handler`, `VoiceHandler`, `CommandHandler`, `MessageHandler`
   - Registers Discord event handlers for `Ready`, `MessageCreate`, `ChannelCreate`
   - Opens Discord connection
   - Posts initial boot message (`reporting.NewBootMessage()`)
   - Performs cleanup via `startup.PerformCleanup()` (clears logs, old audio)
   - Loads guild states via `startup.LoadGuildStates()`
   - Caches all DM message history via `startup.CacheAllDMs()`
   - Posts detailed system status via `reporting.PostFinalStatus()`
   - Waits for SIGINT/SIGTERM to gracefully shutdown

### Core Components

#### Event System (`events/`)

The event system is split into four separate handler structs, each with clear responsibilities:

- **Handler** (`events/events.go`): Main event handler for Discord gateway events
  - Handles `Ready` event: Fetches and stores last 50 messages for all channels (guilds + DMs)
  - Handles `ChannelCreate` event: Updates state (handler intentionally empty, registration alone is sufficient)
  - `fetchAndStoreLast50Messages()`: Fetches messages and bulk inserts into cache

- **MessageHandler** (`events/message_handler.go`): Processes incoming messages and LLM integration
  - Filters out bot's own messages
  - Routes commands (prefixed with `!`) to CommandHandler
  - Caches all messages to Redis (including DM channel tracking)
  - `ProcessLLMResponse()`: Multi-stage LLM response pipeline
    1. User sends message → enters `StatePending`, starts typing indicator
    2. Fetches last 50 messages, extracts up to 5 since bot's last message for engagement check
    3. Calls `GetEngagementDecision()` with engagement model
    4. Based on decision:
       - `REPLY`: Stream conversational response with last 10 messages, enter `StateStreaming`
       - `REACT`: Add emoji reaction
       - `STOP`: Cancel current streaming response
       - `CONTINUE`: Resume interrupted response with saved context
       - `IGNORE`: Do nothing
    5. On interruption (new message while streaming), cancels and deletes partial response

- **CommandHandler** (`events/command_handler.go`): Routes and handles bot commands
  - Deletes command messages in guilds (not DMs)
  - Routes commands:
    - `!join` - Join user's current voice channel
    - `!leave` - Leave voice channel
    - `!clear_dex` - Delete all bot messages in current channel (last 100)
  - `handleUnknownCommand()`: Shows error message for invalid commands (auto-deletes after 10s)

- **VoiceHandler** (`events/voice_handler.go`): Complex voice connection and audio processing pipeline
  - **Connection Management**:
    - `JoinVoice()`: Joins voice channel with retry logic (max 3 attempts with exponential backoff)
    - State-aware join: Checks if already in correct channel, handles channel moves
    - `LeaveVoice()`: Disconnects with proper cleanup and finalization
    - `disconnectFromVoice()`: Polls for complete disconnection (5s timeout), updates connection message with stats
  - **Audio Processing**:
    - `handleVoice()`: Main event loop receiving Opus packets from Discord
    - `handleAudioPacket()`: Converts Opus packets to OGG format per SSRC (user)
    - Tracks unmapped SSRCs (users who joined before bot, no VoiceSpeakingUpdate received)
    - `checkStreamTimeouts()`: Finalizes streams after silence timeout
  - **Transcription**:
    - `finalizeStream()`: Saves audio to cache, calls transcription
    - `transcribeAndUpdate()`: Sends audio to STT, adds to in-memory transcription history
    - Transcriptions stored in `GuildState.TranscriptionHistory` map (keyed by channel ID)
    - Audio deleted immediately after transcription (success or failure)
  - **Status Reporting**:
    - Updates connection message every 5 seconds with live status
    - `formatConnectionMessage()`: Shows header, user list, recent transcriptions
    - `formatUserList()`: Lists all users in channel with speaking status, SSRCs, and mapped/unmapped indicators
    - `formatTranscriptionHistory()`: Shows last 10 transcriptions from in-memory history
  - **SSRC Mapping**:
    - `SpeakingUpdate()`: Maps SSRCs to users (called on speaking events and user joins)
    - Tracks all SSRCs regardless of speaking status to handle pre-existing users

- **StateManager** (`events/state.go`): Manages guild voice connection states
  - Thread-safe `sync.Map` for guild states
  - `GetOrStoreGuildState()`: Creates new `GuildState` with context for cancellation
  - `GetGuildState()`, `DeleteGuildState()`: State lifecycle management
  - Tracks connection stats (added messages count and size)

- **UserManager** (`events/usermanager.go`): Tracks per-user LLM interaction state
  - Three states: `StateIdle`, `StatePending`, `StateStreaming`
  - `TransitionToPending()`: Starts typing indicator
  - `TransitionToStreaming()`: Returns context for cancellation
  - `TransitionToIdle()`: Stops typing, cancels ongoing response, deletes partial message
  - `SaveInterruptedState()`, `ClearInterruptedState()`: Preserves context for `CONTINUE` decision

#### LLM Client (`llm/`)

The LLM client is split into three files for better organization:

- **Client** (`llm/client.go`): Ollama API client and core logic
  - `NewClient()`: Initializes with persona config, creates system prompt from template
  - Two models: `engagement_model` (fast decision-making) and `conversational_model` (full responses)
  - `GetEngagementDecision()`:
    - Analyzes recent messages with engagement model
    - Returns action (REPLY/REACT/IGNORE/STOP/CONTINUE) and optional argument (e.g., emoji)
    - Uses engagement prompt template with conversation context
  - `GenerateContextBlock()`: Creates context message with:
    - Current timestamp and timezone
    - Guild/channel information
    - Online/offline user lists (from voice states)
    - Used as first message in conversation history
  - `createOllamaPayload()`: Formats messages for Ollama API
    - System prompt + context block + conversation history
    - Maps Discord messages to Ollama format (user/assistant roles)

- **Streaming** (`llm/stream.go`): Handles Ollama streaming responses
  - `StreamChatCompletion()`: Main streaming function
    - POSTs to Ollama API with streaming enabled
    - Calls `processStream()` to handle response
    - Respects cancellation context from UserManager
  - `processStream()`: Processes newline-delimited JSON chunks
    - Creates initial Discord message on first chunk
    - Accumulates content and edits message incrementally
    - Cleans response (strips XML tags like `<think>`, `<say>`)
    - Returns final message for caching

- **Prompts** (`llm/prompts.go`): Prompt templates
  - `systemMessageTemplate`: System prompt with persona configuration
    - Uses Go `text/template` with custom `join` function
    - Includes identity, functions, personality traits, communication style
  - `engagementPromptTemplate`: Engagement decision prompt
    - Provides conversation context for decision-making
    - Returns JSON with action and optional argument

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
  - `GuildState` struct with multiple mutexes for thread-safety:
    - `StreamsMutex`: Protects `ActiveStreams` map (SSRC → UserStream)
    - `MetaMutex`: Protects metadata (SSRC mappings, transcription history, connection info)
  - `SSRCUserMap`: Maps SSRC (uint32) to Discord user ID
  - `UnmappedSSRCs`: Tracks SSRCs received without user mappings (pre-existing users)
  - `TranscriptionHistory`: In-memory transcription log per channel (map[channelID][]TranscriptionEntry)
    - Each entry contains: duration, username, transcription text, timestamp, isEvent flag
    - Capped at 100 entries per channel
  - Connection metadata: message IDs, channel ID, start time
  - Context and cancel function for graceful shutdown
  - `NewGuildState()`: Creates new state with context for cancellation

- **User Streams** (`guild/userstream.go`): Per-user audio stream
  - `UserStream` struct tracks individual user's audio:
    - Voice channel ID
    - OggWriter and Buffer for audio data
    - Last packet timestamp for timeout detection
    - User info and start time
    - Unique filename for cache storage
  - Buffers RTP packets and converts to OGG format
  - Finalized on silence timeout or disconnect

#### Additional Packages

- **Interfaces** (`interfaces/`): Interface definitions and shared types
  - `SpeechToText` interface: `Transcribe(audioData []byte) (string, error)`
  - `Persona` struct: Complete persona configuration structure
    - Identity: name, aliases, pronouns, origin story
    - Functions: interaction rules, problem-solving approach
    - Personality: core traits (passion, humor, sarcasm, empathy, etc.), communication style
    - Preferences: language, formatting options

- **Startup** (`startup/`): Startup operations
  - `PerformCleanup()`: Runs cleanup tasks in parallel
    - Clears log channel (except boot message)
    - Cleans old audio from cache
    - Returns cleanup report with statistics
  - `LoadGuildStates()`: Loads persisted guild states from cache
  - `CacheAllDMs()`: Bulk caches last 50 messages for all known DM channels

- **Reporting** (`reporting/`): Status reporting and boot messages
  - `NewBootMessage()`: Creates and posts initial boot message
  - `PostFinalStatus()`: Posts comprehensive system status
    - System info: CPU, RAM, GPU utilization and specs
    - Storage devices: disk usage per device/mount point
    - Service status: Discord, STT, LLM, local/cloud cache
    - Cache statistics and guild/channel counts
    - Cleanup results
  - Helper functions: `formatSystemStatus()`, `formatStorageStatus()`, `formatServiceStatus()`

- **Cleanup** (`cleanup/`): Cleanup operations
  - `ClearChannel()`: Deletes messages from log channel (preserves boot message)
  - Returns `Result` struct with operation name and count

- **Health** (`health/`): Health checking for external services
  - Checks connectivity to LLM server, STT client, caches

- **System** (`system/`): System information gathering
  - `GetSystemInfo()`: Returns `SysInfo` with CPU model, core count, thread count, speed, total memory
  - `GetCPUUsage()`: Current CPU utilization percentage
  - `GetMemoryUsage()`: Current memory utilization percentage
  - `GetGPUInfo()`: Nvidia GPU information (name, utilization, memory usage)
  - `GetStorageInfo()`: Storage device information (device name, mount point, total/used/available space)

- **Session** (`session/`): Discord session creation
  - `NewSession()`: Creates and configures Discord session
    - Sets intents (guilds, guild messages, guild voice states, DMs, message content)
    - Configures voice options (disable video, enable opus decode)
    - Initializes state tracking

- **Preinit** (`preinit/`): Pre-initialization logging
  - `NewLogger()`: Creates console-only logger for early initialization errors
  - Used before Discord connection is established

- **Constants** (`constants/`): Application constants
  - Boot message stage constants (`BootMessageInit`, `BootMessageCleanup`, `BootMessageGuildsLoaded`)

### Configuration

Configs live in `~/Dexter/config/` and are loaded/created by `config/config.go`:

- **Main Config** (`config.json`): Paths to other config files
- **Discord Config**: Token, home server ID, log channel ID
- **Cache Config**: Local and cloud Redis connection details
- **Bot Config**: Voice timeout, audio TTL, LLM server URL, engagement model name, conversational model name
- **GCloud Config**: Service account credentials for Speech-to-Text
- **Persona Config**: Complete persona definition (see `interfaces.Persona` struct) used in LLM system prompt
  - Loaded from JSON file, parsed into `Persona` struct
  - Used by `llm.NewClient()` to generate system prompt via template

### Key Dependencies

- **discordgo**: Discord API client and voice support
- **go-redis**: Redis client for caching
- **Google Cloud Speech**: Speech-to-Text API
- **pion/webrtc**: WebRTC for voice packet handling
- **gopsutil**: System resource monitoring

### Important Patterns

1. **Dependency Injection**: All dependencies initialized in `di.Container`, passed to handlers via constructors
2. **Dual Cache Strategy**: Local Redis for fast access, optional cloud Redis for persistence
3. **Message History**: Always maintains last 50 messages per channel in Redis
4. **Engagement Model**: Lightweight LLM call before every potential response to decide whether/how to engage
   - Uses last 5 messages since bot's last message for context
   - Full conversation (last 10 messages) only used if decision is REPLY
5. **User State Tracking**: Per-user state prevents simultaneous responses and enables interruption
6. **Voice Stream Lifecycle**: Each user gets separate audio stream, finalized on silence or disconnect
7. **In-Memory Transcription**: Transcriptions stored in `GuildState.TranscriptionHistory` (no dedicated channel)
   - Displayed in live-updating connection status message
   - Capped at 100 entries per channel
8. **SSRC Mapping**: VoiceSpeakingUpdate events map SSRCs to users
   - Handles pre-existing users (unmapped SSRCs tracked separately)
   - Critical for attributing audio packets to correct users
9. **Error Resilience**: Most errors logged but don't crash; continues operation
10. **Boot Reporting**: Posts detailed startup status to Discord log channel
    - Includes system stats (CPU, RAM, GPU, storage)
    - Service health checks
    - Cleanup results

### Common Modification Points

- **Add new command**: Add case to switch statement in `CommandHandler.RouteCommand()` in `events/command_handler.go`
  - Create new method on `CommandHandler` or `VoiceHandler` for command logic
- **Add new dependency**:
  - Add to `di.Container` struct in `di/di.go`
  - Initialize in `di.NewContainer()`
  - Pass to handlers via constructors in `app/app.go`
- **Change LLM behavior**:
  - Modify templates in `llm/prompts.go` (system prompt or engagement prompt)
  - Update persona config JSON file
  - Adjust decision parsing in `llm/client.go` `GetEngagementDecision()`
- **Modify voice processing**:
  - Connection/SSRC logic: `events/voice_handler.go`
  - Audio stream management: `guild/userstream.go`
  - Status message formatting: `VoiceHandler.format*()` methods
- **Adjust message handling**:
  - Message routing: `events/message_handler.go` `Handle()`
  - LLM processing pipeline: `MessageHandler.ProcessLLMResponse()`
- **Add startup task**: Add to `app/app.go` `Run()` between connection and final status
- **Modify boot message**:
  - Update reporting functions in `reporting/reporting.go`
  - Add constants to `constants/boot_messages.go`
- **Cache operations**:
  - Extend `cache.Cache` interface in `cache/cache.go`
  - Implement in concrete cache type
- **Add system info**: Extend functions in `system/system.go` and format in `reporting/reporting.go`
