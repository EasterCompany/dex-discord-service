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

const maxLogsDisplay = 5 // Max number of log entries to display on dashboard

// LogsDashboard shows recent logs and errors
type LogsDashboard struct {
	session      *discordgo.Session
	logChannelID string
	cache        *MessageCache
	redisClient  *cache.RedisClient
}

// NewLogsDashboard creates a new logs dashboard
func NewLogsDashboard(session *discordgo.Session, logChannelID string, redisClient *cache.RedisClient) *LogsDashboard {
	return &LogsDashboard{
		session:      session,
		logChannelID: logChannelID,
		cache: &MessageCache{
			ThrottleDuration: 10 * time.Second, // Logs update every 10 seconds
		},
		redisClient: redisClient,
	}
}

// Init creates the logs dashboard message and loads initial data from Redis
func (d *LogsDashboard) Init() error {
	log.Println("[DASHBOARD_INIT] Creating Logs dashboard...")

	content, err := d.formatLogs()
	if err != nil {
		log.Printf("Error formatting initial logs: %v", err)
		content = "**Logs Dashboard**\n\n_Error loading logs_"
	}

	msg, err := d.session.ChannelMessageSend(d.logChannelID, content)
	if err != nil {
		return fmt.Errorf("failed to create logs dashboard: %w", err)
	}

	d.cache.MessageID = msg.ID
	d.cache.Content = content
	d.cache.LastUpdate = time.Now()
	d.cache.LastAPIUpdate = time.Now()

	log.Printf("[DASHBOARD_INIT] Logs dashboard created: %s\n", msg.ID)

	return nil
}

// Update refreshes the logs dashboard (throttled)
func (d *LogsDashboard) Update() error {
	content, err := d.formatLogs()
	if err != nil {
		return fmt.Errorf("failed to format logs for update: %w", err)
	}
	return UpdateThrottled(d.cache, d.session, d.logChannelID, content)
}

// ForceUpdate bypasses throttle
func (d *LogsDashboard) ForceUpdate() error {
	content, err := d.formatLogs()
	if err != nil {
		return fmt.Errorf("failed to format logs for force update: %w", err)
	}
	return ForceUpdateNow(d.cache, d.session, d.logChannelID, content)
}

// Finalize performs final update
func (d *LogsDashboard) Finalize() error {
	return nil
}

// AddLog adds a new log entry to Redis and triggers a dashboard update.
func (d *LogsDashboard) AddLog(logEntry string) {
	ctx := context.Background()

	// Add to list for dashboard display
	err := d.redisClient.AddToList(ctx, cache.LogsKey, logEntry, maxLogsDisplay)
	if err != nil {
		log.Printf("Error adding log to Redis list: %v", err)
	}

	if err := d.Update(); err != nil {
		log.Printf("Error updating logs dashboard: %v", err)
	}
}

// formatLogs generates the display content for the dashboard from Redis.
func (d *LogsDashboard) formatLogs() (string, error) {
	logs, err := d.redisClient.GetListRange(context.Background(), cache.LogsKey, 0, maxLogsDisplay-1)
	if err != nil {
		return "", err
	}

	if len(logs) == 0 {
		return "**Logs Dashboard**\n\n_No logs yet_", nil
	}

	// Reverse logs to show newest at the bottom
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}

	var builder strings.Builder
	builder.WriteString("**Logs Dashboard**\n```\n")

	for _, logEntry := range logs {
		builder.WriteString(logEntry)
		builder.WriteString("\n")
	}

	builder.WriteString("```")
	return builder.String(), nil
}
