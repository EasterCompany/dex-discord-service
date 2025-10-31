package dashboard

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

const maxEvents = 100

// EventsDashboard shows recent Discord events
type EventsDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
	recentEvents []string // In-memory store for recent events
}

// NewEventsDashboard creates a new events dashboard
func NewEventsDashboard(session *discordgo.Session, logChannelID string) *EventsDashboard {
	return &EventsDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 30 * time.Second,
		},
		recentEvents: make([]string, 0, maxEvents),
	}
}

// Init creates the events dashboard message
func (d *EventsDashboard) Init() error {
	content := "**Events Dashboard**\n\n_No events yet_"

	log.Println("[DASHBOARD_INIT] Creating Events dashboard...")

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create events dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Events dashboard created: %s\n", msg.ID)

	return nil
}

// AddEvent adds a new event to the dashboard's log and triggers an update.
func (d *EventsDashboard) AddEvent(event string) {
	// TODO: Replace in-memory slice with Redis list (LPUSH/LTRIM)
	d.recentEvents = append(d.recentEvents, event)
	if len(d.recentEvents) > maxEvents {
		d.recentEvents = d.recentEvents[len(d.recentEvents)-maxEvents:]
	}

	if err := d.Update(); err != nil {
		log.Printf("Error updating events dashboard: %v", err)
	}
}

// Update refreshes the events dashboard (throttled)
func (d *EventsDashboard) Update() error {
	content := d.formatEvents()
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *EventsDashboard) ForceUpdate() error {
	content := d.formatEvents()
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *EventsDashboard) Finalize() error {
	return nil
}

// formatEvents generates the display content for the dashboard.
func (d *EventsDashboard) formatEvents() string {
	if len(d.recentEvents) == 0 {
		return "**Events Dashboard**\n\n_No events yet_"
	}

	var builder strings.Builder
	builder.WriteString("**Events Dashboard**\n```\n")

	start := 0
	if len(d.recentEvents) > 15 {
		start = len(d.recentEvents) - 15
	}

	for _, event := range d.recentEvents[start:] {
		builder.WriteString(event)
		builder.WriteString("\n")
	}

	builder.WriteString("```")
	return builder.String()
}
