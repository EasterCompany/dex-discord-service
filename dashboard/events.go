package dashboard

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/cache"
	"github.com/bwmarrin/discordgo"
)

const maxEvents = 100
const maxEventsDisplay = 5

// EventsDashboard shows recent Discord events
type EventsDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
	redisClient  *cache.RedisClient
}

// NewEventsDashboard creates a new events dashboard
func NewEventsDashboard(session *discordgo.Session, logChannelID string, redisClient *cache.RedisClient) *EventsDashboard {
	return &EventsDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 5 * time.Second, // Faster updates for events
		},
		redisClient: redisClient,
	}
}

// Init creates the events dashboard message and loads initial data from Redis
func (d *EventsDashboard) Init() error {
	log.Println("[DASHBOARD_INIT] Creating Events dashboard...")

	content, err := d.formatEvents()
	if err != nil {
		log.Printf("Error formatting initial events: %v", err)
		content = "**Events Dashboard**\n\n_Error loading events_"
	}

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

// AddEvent adds a new event to Redis, publishes it, and triggers a dashboard update.
func (d *EventsDashboard) AddEvent(event string) {
	ctx := context.Background()

	// Add to list for dashboard display
	err := d.redisClient.AddToList(ctx, cache.EventsKey, event, maxEvents)
	if err != nil {
		log.Printf("Error adding event to Redis list: %v", err)
	}

	// Publish to pub/sub for other services
	err = d.redisClient.PublishEvent(ctx, cache.EventStreamChannel, event)
	if err != nil {
		log.Printf("Error publishing event to Redis: %v", err)
	}

	if err := d.Update(); err != nil {
		log.Printf("Error updating events dashboard: %v", err)
	}
}

// Update refreshes the events dashboard (throttled)
func (d *EventsDashboard) Update() error {
	content, err := d.formatEvents()
	if err != nil {
		return fmt.Errorf("failed to format events for update: %w", err)
	}
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *EventsDashboard) ForceUpdate() error {
	content, err := d.formatEvents()
	if err != nil {
		return fmt.Errorf("failed to format events for force update: %w", err)
	}
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *EventsDashboard) Finalize() error {
	return nil
}

// formatEvents generates the display content for the dashboard from Redis.
func (d *EventsDashboard) formatEvents() (string, error) {
	events, err := d.redisClient.GetListRange(context.Background(), cache.EventsKey, 0, maxEventsDisplay-1)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "**Events Dashboard**\n\n_No events yet_", nil
	}

	// Reverse events to show newest at the bottom
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	var builder strings.Builder
	builder.WriteString("**Events Dashboard**\n```\n")

	for _, event := range events {
		builder.WriteString(event)
		builder.WriteString("\n")
	}

	builder.WriteString("```")
	return builder.String(), nil
}
