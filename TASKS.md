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
- [x] **Redis/Valkey Integration** - `cache/redis.go`
- [x] **Dashboard Persistence** - Messages and events stored in Redis
- [x] **Event Handlers** - `handlers/` for messages and generic events
- [x] **Service Interfaces** - `services/interfaces.go` and `services/stubs.go`

## 🚧 In Progress

### Dashboard Content Population

- [x] **Logs Dashboard** - Track and display recent logs/errors, stored in Redis
  - [x] Add in-memory log buffer (ring buffer, last 20 entries) - *Now Redis-backed*
  - [x] Capture log.Printf/log.Println calls - *Redirected to Redis*
  - [x] Format as scrolling list with timestamps
  - [ ] Color-code by severity (info/warn/error)

- [x] **Events Dashboard** - Shows Discord gateway events, stored in Redis
- [x] **Messages Dashboard** - Displays message activity, stored in Redis
- [x] **Voice Dashboard** - Real-time voice status
  - [x] List connected users and channels
  - [ ] Show connection state (idle/connecting/connected)
  - [ ] Display SSRC mappings
  - [ ] Show transcription activity (if available)
  - [ ] Connection duration

### Voice Connection

- [x] **Voice State Management** - `dashboard/voice_state.go`
  - [x] Track users in voice channels
  - [ ] Store SSRC → user mappings
  - [ ] Handle connection lifecycle

## 🎯 High Priority

### Command System

- [ ] **Command Handler** - `handlers/commands.go`
  - [ ] Detect commands (prefix: `!` or `dex`)
  - [ ] Route to appropriate handler
  - [ ] Commands: `!join`, `!leave`, `!status`, `!help`
  - [ ] Update Messages dashboard with command usage

### Service Integration

- [ ] **Implement Service Clients**
  - [ ] Connect to `dex-stt` for transcription
  - [ ] Connect to `dex-llm` for responses
  - [ ] Connect to `dex-tts` for voice output

## 🔧 Medium Priority

### Audio Pipeline

- [ ] **Audio Reception**
  - [ ] Receive Discord audio packets
  - [ ] Buffer audio per user
- [ ] **Audio Forwarding**
  - [ ] Send to STT service
  - [ ] Display transcriptions in Voice dashboard

### Monitoring & Health

- [ ] **Health Endpoint** (Optional)
  - [ ] Simple HTTP server (`:8080/health`)
  - [ ] Return JSON with service status
- [ ] **Metrics Collection** (Optional)
  - [ ] Track API calls to Discord, message count, dashboard updates
  - [ ] Expose via `/metrics` endpoint

## 📊 Low Priority / Nice-to-Have

### Enhanced Dashboards

- [ ] **Server Dashboard Enhancements**
  - [ ] Show server boost level, features, connection quality
  - [ ] Track API rate limit headroom
- [ ] **Logs Dashboard Enhancements**
  - [ ] Severity filtering, search functionality
  - [ ] Log rotation to file, integration with external logging service
- [ ] **Voice Dashboard Enhancements**
  - [ ] Audio quality metrics, packet loss stats
  - [ ] User speaking time tracking, transcription confidence scores

### Developer Experience

- [ ] **Hot Reload** (Development)
- [ ] **Mock Discord Mode**
- [ ] **Dashboard Preview**

### Documentation

- [ ] **README.md** - Quick start, architecture, deployment guide
- [ ] **API Documentation** - Redis format, service interfaces
- [ ] **Troubleshooting Guide**

## 🚀 Integration Goals

### Phase 1: Monitoring (Current)
- ✅ Dashboard displays current Discord state
- ✅ Clean UI for debugging
- ✅ Event tracking in Redis
- ✅ Log aggregation in Redis

### Phase 2: Data Capture
- ✅ Write all events to Redis
- ✅ Persist voice activity
- ✅ Store message history
- ✅ Expose data for other services via pub/sub

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