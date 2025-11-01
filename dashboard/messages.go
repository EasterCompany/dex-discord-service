package dashboard

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/bwmarrin/discordgo"
)

const maxMessages = 100
const maxMessagesDisplay = 5

// MessagesDashboard shows recent Discord messages
type MessagesDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
	redisClient  *cache.RedisClient
}

// NewMessagesDashboard creates a new messages dashboard
func NewMessagesDashboard(session *discordgo.Session, logChannelID string, redisClient *cache.RedisClient) *MessagesDashboard {
	return &MessagesDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 5 * time.Second, // Faster updates for messages
		},
		redisClient: redisClient,
	}
}

// Init creates the messages dashboard message and loads initial data from Redis
func (d *MessagesDashboard) Init() error {
	log.Println("[DASHBOARD_INIT] Creating Messages dashboard...")

	// Format with existing messages from Redis
	content, err := d.formatMessages()
	if err != nil {
		log.Printf("Error formatting initial messages: %v", err)
		content = "**Messages Dashboard**\n\n_Error loading messages_"
	}

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create messages dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Messages dashboard created: %s\n", msg.ID)

	return nil
}

// AddMessage adds a new message to Redis and triggers a dashboard update.
func (d *MessagesDashboard) AddMessage(message string) {
	ctx := context.Background()
	err := d.redisClient.AddToList(ctx, cache.MessagesKey, message, maxMessages)
	if err != nil {
		log.Printf("Error adding message to Redis: %v", err)
		return
	}

	// Trigger a throttled update.
	if err := d.Update(); err != nil {
		log.Printf("Error updating messages dashboard: %v", err)
	}
}

// Update refreshes the messages dashboard (throttled)
func (d *MessagesDashboard) Update() error {
	content, err := d.formatMessages()
	if err != nil {
		return fmt.Errorf("failed to format messages for update: %w", err)
	}
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *MessagesDashboard) ForceUpdate() error {
	content, err := d.formatMessages()
	if err != nil {
		return fmt.Errorf("failed to format messages for force update: %w", err)
	}
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *MessagesDashboard) Finalize() error {
	return nil
}

// formatMessages generates the display content for the dashboard from Redis.
func (d *MessagesDashboard) formatMessages() (string, error) {
	messages, err := d.redisClient.GetListRange(context.Background(), cache.MessagesKey, 0, maxMessagesDisplay-1)
	if err != nil {
		return "", err
	}

	if len(messages) == 0 {
		return "**Messages Dashboard**\n\n_No messages yet_", nil
	}

	// Reverse messages to show newest at the bottom
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	var builder strings.Builder
	builder.WriteString("**Messages Dashboard**\n```\n")

	for _, msg := range messages {
		builder.WriteString(msg)
		builder.WriteString("\n")
	}

	builder.WriteString("```")
	return builder.String(), nil
}
