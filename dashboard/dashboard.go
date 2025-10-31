package dashboard

import (
	"github.com/bwmarrin/discordgo"
)

// Manager coordinates all dashboards
type Manager struct {
	session      *discordgo.Session
	logChannelID string

	System   *SystemDashboard
	Logs     *LogsDashboard
	Events   *EventsDashboard
	Messages *MessagesDashboard
	Voice    *VoiceDashboard
}

// NewManager creates a new dashboard manager
func NewManager(session *discordgo.Session, logChannelID string) *Manager {
	return &Manager{
		session:      session,
		logChannelID: logChannelID,
		System:       NewSystemDashboard(session, logChannelID),
		Logs:         NewLogsDashboard(session, logChannelID),
		Events:       NewEventsDashboard(session, logChannelID),
		Messages:     NewMessagesDashboard(session, logChannelID),
		Voice:        NewVoiceDashboard(session, logChannelID),
	}
}

// Init initializes all dashboards
func (m *Manager) Init() error {
	// Initialize in order: System -> Logs -> Events -> Messages -> Voice
	if err := m.System.Init(); err != nil {
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
	_ = m.System.Finalize()

	return nil
}
