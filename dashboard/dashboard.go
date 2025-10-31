package dashboard

import (
	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/bwmarrin/discordgo"
)

// Manager coordinates all dashboards
type Manager struct {
	session      *discordgo.Session
	logChannelID string
	serverID     string
	redisClient  *cache.RedisClient
	VoiceState   *VoiceState

	Server   *ServerDashboard
	Logs     *LogsDashboard
	Events   *EventsDashboard
	Messages *MessagesDashboard
	Voice    *VoiceDashboard
}

// NewManager creates a new dashboard manager
func NewManager(session *discordgo.Session, logChannelID, serverID string, redisClient *cache.RedisClient) *Manager {
	voiceState := NewVoiceState()
	return &Manager{
		session:      session,
		logChannelID: logChannelID,
		serverID:     serverID,
		redisClient:  redisClient,
		VoiceState:   voiceState,
		Server:       NewServerDashboard(session, logChannelID, serverID),
		Logs:         NewLogsDashboard(session, logChannelID, redisClient),
		Events:       NewEventsDashboard(session, logChannelID, redisClient),
		Messages:     NewMessagesDashboard(session, logChannelID, redisClient),
		Voice:        NewVoiceDashboard(session, logChannelID, voiceState),
	}
}

// Init initializes all dashboards
func (m *Manager) Init() error {
	// Initialize in order: Server -> Logs -> Events -> Messages -> Voice
	if err := m.Server.Init(); err != nil {
		return err
	}
	if err := m.Logs.Init(); err != nil {
		return err
	}
	if err := m.Events.Init(); err != nil {
		return err
	}
	if err := m.Messages.Init(); err != nil {
		return err
	}
	if err := m.Voice.Init(); err != nil {
		return err
	}

	return nil
}

// Shutdown finalizes all dashboards
func (m *Manager) Shutdown() error {
	// Finalize in reverse order
	_ = m.Voice.Finalize()
	_ = m.Messages.Finalize()
	_ = m.Events.Finalize()
	_ = m.Logs.Finalize()
	_ = m.Server.Finalize()

	return nil
}
