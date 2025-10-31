# TASKS.md

Task list for completing dex-discord-interface as a nimble Discord microservice.

## ✅ Completed

- [x] Clean slate architecture on `fresh-start` branch
- [x] Config loading from `~/Dexter/config/discord-interface.json`
- [x] Discord connection with full intents
- [x] Dashboard system with 5 panels (Server, Logs, Events, Messages, Voice)
- [x] Throttling system (cache + API layers)
- [x] Log channel cleanup on boot
- [x] Server dashboard with guild info
- [x] Build scripts using `./bin/` directory
- [x] Systemd service integration
- [x] Proper shutdown lifecycle

## 🚧 In Progress

### Dashboard Content Population

- [ ] **Logs Dashboard** - Track and display recent logs/errors
  - [ ] Add in-memory log buffer (ring buffer, last 20 entries)
  - [ ] Capture log.Printf/log.Println calls
  - [ ] Format as scrolling list with timestamps
  - [ ] Color-code by severity (info/warn/error)

- [ ] **Events Dashboard** - Show Discord gateway events
  - [ ] Track user joins/leaves
  - [ ] Track role changes
  - [ ] Track channel creates/deletes
  - [ ] Show last 10-15 events with timestamps
  - [ ] Format: `[HH:MM:SS] EventType: details`

- [ ] **Messages Dashboard** - Display message activity
  - [ ] Track messages per channel
  - [ ] Show command usage statistics
  - [ ] Display bot response count
  - [ ] Show last 5-10 recent messages
  - [ ] Format: `[HH:MM:SS] #channel: username: preview...`

- [ ] **Voice Dashboard** - Real-time voice status
  - [ ] Show connection state (idle/connecting/connected)
  - [ ] List connected users
  - [ ] Display SSRC mappings
  - [ ] Show transcription activity (if available)
  - [ ] Connection duration

## 🎯 High Priority

### Redis/Valkey Integration

- [ ] **Cache Package** - `cache/redis.go`
  - [ ] Connect to Redis using config
  - [ ] Store dashboard message IDs (persist across restarts)
  - [ ] Pub/sub for event streaming
  - [ ] Key-value for state storage
  - [ ] Health check functionality

- [ ] **Dashboard Persistence**
  - [ ] Save message IDs to Redis on dashboard creation
  - [ ] Load message IDs from Redis on boot (if exist)
  - [ ] Update existing messages instead of creating new ones
  - [ ] Graceful fallback if messages deleted

### Event Handlers

- [ ] **Message Handler** - `handlers/message.go`
  - [ ] Capture all messages
  - [ ] Update Messages dashboard
  - [ ] Publish to Redis for other services
  - [ ] Track command invocations

- [ ] **Voice Handler** - `handlers/voice.go`
  - [ ] Handle voice state updates
  - [ ] Track user joins/leaves voice
  - [ ] Update Voice dashboard
  - [ ] Manage voice connections

- [ ] **Guild Handler** - `handlers/guild.go`
  - [ ] Member join/leave events
  - [ ] Role updates
  - [ ] Channel changes
  - [ ] Update Events dashboard
  - [ ] Update Server dashboard when needed

### Service Interfaces

- [ ] **Define Interfaces** - `services/interfaces.go`
  ```go
  type STTService interface {
      Transcribe(audio []byte) (string, error)
  }

  type LLMService interface {
      Process(context, message string) (string, error)
  }

  type TTSService interface {
      Synthesize(text string) ([]byte, error)
  }
  ```

- [ ] **Stub Implementations** - `services/stubs.go`
  - [ ] Mock STT (returns "[transcription]")
  - [ ] Mock LLM (returns "mock response")
  - [ ] Mock TTS (returns empty audio)
  - [ ] Log all calls for visibility

## 🔧 Medium Priority

### Voice Connection

- [ ] **Voice State Management** - `voice/state.go`
  - [ ] Track active voice connection
  - [ ] Store SSRC → user mappings
  - [ ] Handle connection lifecycle
  - [ ] Integrate with Voice dashboard

- [ ] **Audio Pipeline** (Stub)
  - [ ] Receive Discord audio packets
  - [ ] Buffer audio per user
  - [ ] Send to STT service (when available)
  - [ ] Display transcriptions in Voice dashboard

### Command System

- [ ] **Command Handler** - `handlers/commands.go`
  - [ ] Detect commands (prefix: `!` or `dex`)
  - [ ] Route to appropriate handler
  - [ ] Commands: `!join`, `!leave`, `!status`, `!help`
  - [ ] Update Messages dashboard with command usage

- [ ] **Command Implementations**
  - [ ] `!join` - Join user's voice channel
  - [ ] `!leave` - Leave voice channel
  - [ ] `!status` - Force update all dashboards
  - [ ] `!help` - Show available commands

### Monitoring & Health

- [ ] **Health Endpoint** (Optional)
  - [ ] Simple HTTP server (`:8080/health`)
  - [ ] Return JSON with service status
  - [ ] Enable external monitoring

- [ ] **Metrics Collection** (Optional)
  - [ ] Track API calls to Discord
  - [ ] Track message count
  - [ ] Track dashboard updates
  - [ ] Expose via `/metrics` endpoint

## 📊 Low Priority / Nice-to-Have

### Enhanced Dashboards

- [ ] **Server Dashboard Enhancements**
  - [ ] Show server boost level
  - [ ] Display server features (community, verified, etc.)
  - [ ] Show Discord connection quality/latency
  - [ ] Track API rate limit headroom

- [ ] **Logs Dashboard Enhancements**
  - [ ] Severity filtering (show errors only, etc.)
  - [ ] Search/filter functionality (via commands)
  - [ ] Log rotation to file
  - [ ] Integration with external logging service

- [ ] **Voice Dashboard Enhancements**
  - [ ] Audio quality metrics
  - [ ] Packet loss statistics
  - [ ] User speaking time tracking
  - [ ] Transcription accuracy confidence scores

### Developer Experience

- [ ] **Hot Reload** (Development)
  - [ ] Watch for code changes
  - [ ] Auto-rebuild and restart
  - [ ] Keep dashboards persistent across restarts

- [ ] **Mock Discord Mode**
  - [ ] Run without real Discord connection
  - [ ] Simulate events for testing
  - [ ] Test dashboard rendering

- [ ] **Dashboard Preview**
  - [ ] Generate markdown preview of dashboards
  - [ ] View in terminal or browser
  - [ ] Test formatting without Discord

### Documentation

- [ ] **README.md**
  - [ ] Quick start guide
  - [ ] Architecture diagram
  - [ ] Configuration examples
  - [ ] Deployment guide

- [ ] **API Documentation**
  - [ ] Document Redis event format
  - [ ] Document service interfaces
  - [ ] Integration examples

- [ ] **Troubleshooting Guide**
  - [ ] Common issues and fixes
  - [ ] Debug mode instructions
  - [ ] Performance tuning

## 🚀 Integration Goals

### Phase 1: Monitoring (Current)
- ✅ Dashboard displays current Discord state
- ✅ Clean UI for debugging
- ⏳ Event tracking
- ⏳ Log aggregation

### Phase 2: Data Capture
- ⏳ Write all events to Redis
- ⏳ Persist voice activity
- ⏳ Store message history
- ⏳ Expose data for other services

### Phase 3: Service Integration
- ⏳ Connect to dex-stt for transcription
- ⏳ Connect to dex-llm for responses
- ⏳ Connect to dex-tts for voice output
- ⏳ Full voice conversation loop

### Phase 4: Production Ready
- ⏳ Error recovery and retry logic
- ⏳ Graceful degradation (service failures)
- ⏳ Comprehensive logging
- ⏳ Performance optimization
- ⏳ Security hardening

## 🎯 MVP Definition

**Minimum Viable Product** - What makes this service useful?

Must Have:
- ✅ Connect to Discord
- ✅ Display 5 dashboards
- ⏳ Persist dashboard state in Redis
- ⏳ Basic event tracking (messages, voice, guild events)
- ⏳ Command system (!join, !leave, !status)
- ⏳ Service interface stubs (STT, LLM, TTS)

Should Have:
- Voice connection handling
- Audio capture and forwarding
- Message interception for commands
- Real-time dashboard updates

Could Have:
- Metrics and monitoring
- Advanced logging
- Multiple command prefixes
- Hot reload for development

## 📝 Notes

### Architecture Principles
1. **Keep it thin** - This is an interface, not the brain
2. **Event-driven** - React to Discord, publish to Redis
3. **Fail gracefully** - Service outages shouldn't crash the interface
4. **Observable** - Dashboards show everything happening
5. **Testable** - Mock services for isolated testing

### Performance Targets
- Dashboard update latency: < 5 seconds (via throttling)
- Voice audio latency: < 500ms (when implemented)
- Event processing: < 100ms
- Memory usage: < 100MB
- CPU usage: < 5% idle, < 20% active

### Integration Strategy
Start with stubs, integrate services one at a time:
1. Redis (data layer) - FIRST
2. STT (voice transcription)
3. LLM (conversation)
4. TTS (voice synthesis)

Each integration should be tested in isolation before combining.
