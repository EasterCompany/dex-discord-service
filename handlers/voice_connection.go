package handlers

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/dashboard"
	"github.com/bwmarrin/discordgo"
)

// VoiceConnectionManager manages voice connections and tracks speaking states
type VoiceConnectionManager struct {
	session         *discordgo.Session
	voiceDashboard  *dashboard.VoiceDashboard
	eventsDashboard *dashboard.EventsDashboard
	logsDashboard   *dashboard.LogsDashboard

	// Track active voice connection
	connection   *discordgo.VoiceConnection
	connectionMu sync.RWMutex

	// Speaking state tracking
	speakingUsers map[string]*SpeakingState
	speakingMu    sync.RWMutex

	// Audio statistics
	stats   *AudioStats
	statsMu sync.RWMutex
}

// SpeakingState tracks speaking information for a user
type SpeakingState struct {
	UserID        string
	Speaking      bool
	LastSpoke     time.Time
	SpeakCount    int
	TotalDuration time.Duration
	StartTime     time.Time
}

// AudioStats tracks overall audio statistics
type AudioStats struct {
	TotalPackets     int64
	TotalBytes       int64
	DroppedPackets   int64
	ActiveSpeakers   int
	LastPacketTime   time.Time
	SessionStartTime time.Time
}

// NewVoiceConnectionManager creates a new voice connection manager
func NewVoiceConnectionManager(
	session *discordgo.Session,
	voiceDashboard *dashboard.VoiceDashboard,
	eventsDashboard *dashboard.EventsDashboard,
	logsDashboard *dashboard.LogsDashboard,
) *VoiceConnectionManager {
	return &VoiceConnectionManager{
		session:         session,
		voiceDashboard:  voiceDashboard,
		eventsDashboard: eventsDashboard,
		logsDashboard:   logsDashboard,
		speakingUsers:   make(map[string]*SpeakingState),
		stats: &AudioStats{
			SessionStartTime: time.Now(),
		},
	}
}

// JoinVoiceChannel joins a voice channel and sets up monitoring
func (m *VoiceConnectionManager) JoinVoiceChannel(guildID, channelID string) error {
	m.connectionMu.Lock()
	defer m.connectionMu.Unlock()

	// Leave existing connection if any
	if m.connection != nil {
		_ = m.connection.Disconnect()
	}

	// Join the voice channel
	vc, err := m.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		m.logError(fmt.Sprintf("Failed to join voice channel: %v", err))
		return err
	}

	m.connection = vc
	m.stats.SessionStartTime = time.Now()

	// Log join event
	channel, _ := m.session.Channel(channelID)
	channelName := channelID
	if channel != nil {
		channelName = channel.Name
	}

	// Update voice dashboard with connection status
	m.voiceDashboard.SetBotConnection(true, channelName)
	if err := m.voiceDashboard.ForceUpdate(); err != nil {
		log.Printf("Error updating voice dashboard: %v", err)
	}

	m.logEvent(fmt.Sprintf("Bot joined voice channel #%s", channelName))
	m.logInfo(fmt.Sprintf("Voice connection established to #%s", channelName))

	// Set up speaking state listener
	m.setupSpeakingListener(vc)

	// Set up voice packet receiver for detailed monitoring
	m.setupVoicePacketReceiver(vc)

	return nil
}

// LeaveVoiceChannel disconnects from the current voice channel
func (m *VoiceConnectionManager) LeaveVoiceChannel() error {
	m.connectionMu.Lock()
	defer m.connectionMu.Unlock()

	if m.connection == nil {
		return fmt.Errorf("not connected to a voice channel")
	}

	channelID := m.connection.ChannelID
	channel, _ := m.session.Channel(channelID)
	channelName := channelID
	if channel != nil {
		channelName = channel.Name
	}

	if err := m.connection.Disconnect(); err != nil {
		m.logError(fmt.Sprintf("Error disconnecting from voice: %v", err))
		return err
	}

	m.connection = nil

	// Update voice dashboard with disconnection status
	m.voiceDashboard.SetBotConnection(false, "")
	if err := m.voiceDashboard.ForceUpdate(); err != nil {
		log.Printf("Error updating voice dashboard: %v", err)
	}

	m.logEvent(fmt.Sprintf("Bot left voice channel #%s", channelName))
	m.logInfo(fmt.Sprintf("Voice connection closed to #%s", channelName))

	return nil
}

// setupSpeakingListener sets up the speaking state update handler
func (m *VoiceConnectionManager) setupSpeakingListener(vc *discordgo.VoiceConnection) {
	// Set up a goroutine to monitor speaking states
	// Note: discordgo doesn't expose a speaking map directly, so we track via voice packets
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			m.connectionMu.RLock()
			if m.connection == nil || m.connection != vc {
				m.connectionMu.RUnlock()
				return
			}
			m.connectionMu.RUnlock()

			// Check for users who stopped speaking (no packets in last 2 seconds)
			m.checkInactiveSpeakers()
		}
	}()
}

// setupVoicePacketReceiver sets up detailed voice packet monitoring
func (m *VoiceConnectionManager) setupVoicePacketReceiver(vc *discordgo.VoiceConnection) {
	// Monitor OpusRecv channel for incoming audio packets
	go func() {
		log.Println("[VOICE] Packet receiver started")

		// Batch update ticker to reduce dashboard API calls
		updateTicker := time.NewTicker(3 * time.Second)
		defer updateTicker.Stop()
		needsUpdate := false

		for {
			m.connectionMu.RLock()
			if m.connection == nil || m.connection != vc {
				m.connectionMu.RUnlock()
				log.Println("[VOICE] Packet receiver stopped")
				return
			}
			m.connectionMu.RUnlock()

			// Check if we have an OpusRecv channel
			if vc.OpusRecv == nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			select {
			case packet, ok := <-vc.OpusRecv:
				if !ok {
					return
				}
				m.handleVoicePacket(packet)
				needsUpdate = true
			case <-updateTicker.C:
				// Batched dashboard update
				if needsUpdate {
					m.updateVoiceDashboard()
					needsUpdate = false
				}
			case <-time.After(5 * time.Second):
				// Timeout to check if connection is still alive
				continue
			}
		}
	}()
}

// handleVoicePacket processes incoming voice packets and updates statistics
func (m *VoiceConnectionManager) handleVoicePacket(packet *discordgo.Packet) {
	if packet == nil {
		return
	}

	// Update stats atomically
	m.statsMu.Lock()
	m.stats.TotalPackets++
	m.stats.TotalBytes += int64(len(packet.Opus))
	m.stats.LastPacketTime = time.Now()
	m.statsMu.Unlock()

	// Note: Speaking detection is handled via VoiceStateUpdate events
	// Packet-level speaking detection would require SSRC->UserID mapping
}

// checkInactiveSpeakers checks for users who stopped speaking
func (m *VoiceConnectionManager) checkInactiveSpeakers() {
	m.speakingMu.Lock()
	defer m.speakingMu.Unlock()

	now := time.Now()
	for userID, state := range m.speakingUsers {
		// If user was speaking but hasn't spoken in 2 seconds, mark as stopped
		if state.Speaking && now.Sub(state.LastSpoke) > 2*time.Second {
			m.updateSpeakingStateInternal(userID, false)
		}
	}
}

// updateSpeakingStateInternal updates the speaking state (must be called with lock held)
func (m *VoiceConnectionManager) updateSpeakingStateInternal(userID string, speaking bool) {
	state, exists := m.speakingUsers[userID]
	if !exists {
		state = &SpeakingState{
			UserID: userID,
		}
		m.speakingUsers[userID] = state
	}

	// State transition: not speaking -> speaking
	if speaking && !state.Speaking {
		state.Speaking = true
		state.StartTime = time.Now()
		state.LastSpoke = time.Now()
		state.SpeakCount++

		// Log speaking started
		user, err := m.session.User(userID)
		username := userID
		if err == nil && user != nil {
			username = user.Username
		}
		m.logEvent(fmt.Sprintf("@%s started speaking", username))

		// Update dashboard
		m.voiceDashboard.SetUserSpeaking(userID, true)
		m.statsMu.Lock()
		m.stats.ActiveSpeakers++
		m.statsMu.Unlock()
	} else if speaking && state.Speaking {
		// Update last spoke time to track continued speaking
		state.LastSpoke = time.Now()
	}

	// State transition: speaking -> not speaking
	if !speaking && state.Speaking {
		state.Speaking = false
		state.LastSpoke = time.Now()
		duration := time.Since(state.StartTime)
		state.TotalDuration += duration

		// Log speaking stopped
		user, err := m.session.User(userID)
		username := userID
		if err == nil && user != nil {
			username = user.Username
		}
		m.logEvent(fmt.Sprintf("@%s stopped speaking (duration: %s)", username, duration.Round(time.Millisecond)))

		// Update dashboard
		m.voiceDashboard.SetUserSpeaking(userID, false)
		m.statsMu.Lock()
		m.stats.ActiveSpeakers--
		if m.stats.ActiveSpeakers < 0 {
			m.stats.ActiveSpeakers = 0
		}
		m.statsMu.Unlock()
	}

	// Update voice dashboard
	if err := m.voiceDashboard.Update(); err != nil {
		log.Printf("Error updating voice dashboard: %v", err)
	}
}

// updateVoiceDashboard updates the voice dashboard with current statistics
func (m *VoiceConnectionManager) updateVoiceDashboard() {
	m.statsMu.RLock()
	stats := *m.stats
	m.statsMu.RUnlock()

	// Update dashboard with current stats
	m.voiceDashboard.UpdateAudioStats(
		stats.TotalPackets,
		stats.TotalBytes,
		stats.ActiveSpeakers,
		stats.LastPacketTime,
	)

	// This will be called frequently, so rely on dashboard's internal throttling
	if err := m.voiceDashboard.Update(); err != nil {
		log.Printf("Error updating voice dashboard: %v", err)
	}
}

// GetStats returns current audio statistics
func (m *VoiceConnectionManager) GetStats() AudioStats {
	m.statsMu.RLock()
	defer m.statsMu.RUnlock()
	return *m.stats
}

// GetSpeakingUsers returns a copy of current speaking states
func (m *VoiceConnectionManager) GetSpeakingUsers() map[string]SpeakingState {
	m.speakingMu.RLock()
	defer m.speakingMu.RUnlock()

	result := make(map[string]SpeakingState)
	for userID, state := range m.speakingUsers {
		result[userID] = *state
	}
	return result
}

// Helper methods for logging
func (m *VoiceConnectionManager) logEvent(message string) {
	timestamp := time.Now().Format("15:04:05")
	m.eventsDashboard.AddEvent(fmt.Sprintf("[%s] %s", timestamp, message))
}

func (m *VoiceConnectionManager) logInfo(message string) {
	timestamp := time.Now().Format("15:04:05")
	m.logsDashboard.AddLog(fmt.Sprintf("[%s] INFO: %s", timestamp, message))
}

func (m *VoiceConnectionManager) logError(message string) {
	timestamp := time.Now().Format("15:04:05")
	m.logsDashboard.AddLog(fmt.Sprintf("[%s] ERROR: %s", timestamp, message))
}
