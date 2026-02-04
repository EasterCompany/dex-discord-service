package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// FetchMissedMessages checks for and processes messages that occurred while the bot was offline.
// New Strategy: "Recent Active Channels"
// 1. Fetch the last 1000 messaging events from the Event Service.
// 2. Identify which channels were active (Unique Channel IDs).
// 3. Determine the latest timestamp SEEN for each of those channels.
// 4. Fetch messages from Discord for only those channels, AFTER that timestamp.
func FetchMissedMessages(dg *discordgo.Session, eventServiceURL string, serverID string) {
	log.Println("Starting catch-up routine (Strategy: Recent Active Context)...")

	// 1. Get recent active channels and their watermarks
	activeChannels, err := getRecentActiveChannels(eventServiceURL, 1000)
	if err != nil {
		log.Printf("Catch-up: Failed to fetch recent events: %v. Aborting.", err)
		return
	}

	if len(activeChannels) == 0 {
		log.Println("Catch-up: No recent activity found in event stream. Nothing to catch up on.")
		return
	}

	log.Printf("Catch-up: Found %d active channels in recent context.", len(activeChannels))

	// 2. Pre-fetch known message IDs to prevent duplicates (global check for safety)
	existingIDs, _ := fetchRecentEventMessageIDs(eventServiceURL, 10000)

	// 3. Iterate over active channels
	for channelID, lastSeenTime := range activeChannels {
		log.Printf("Checking channel %s (Last seen: %s)", channelID, lastSeenTime.Format(time.RFC3339))

		// Check if the channel is accessible (could be deleted, permission lost, or DM)
		// We try to fetch messages directly. If it fails, we assume we can't access it.
		snowflake := timeToSnowflake(lastSeenTime.Add(1 * time.Millisecond))

		messages, err := dg.ChannelMessages(channelID, 100, "", snowflake, "")
		if err != nil {
			// Quietly skip if the channel was deleted (very common with threads)
			if strings.Contains(err.Error(), "404 Not Found") || strings.Contains(err.Error(), "Unknown Channel") {
				continue
			}
			log.Printf("Skipping channel %s: %v", channelID, err)
			continue
		}

		if len(messages) == 0 {
			continue
		}

		// Sort messages chronologically (oldest first)
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].ID < messages[j].ID
		})

		count := 0
		channelName := "unknown" // We'll try to fetch this from the first message or cache

		for _, m := range messages {
			// Skip bot's own messages
			if m.Author.ID == dg.State.User.ID {
				continue
			}

			// Skip if already processed
			if existingIDs[m.ID] {
				continue
			}

			// Double check timestamp
			if !m.Timestamp.After(lastSeenTime) {
				continue
			}

			// Capture channel name for event data
			if channelName == "unknown" {
				ch, err := dg.Channel(m.ChannelID)
				if err == nil {
					channelName = ch.Name
					if ch.Type == discordgo.ChannelTypeDM {
						channelName = "DM-" + m.Author.Username
					}
				}
			}

			// Process Message
			if err := processMissedMessage(dg, eventServiceURL, serverID, m, channelName); err != nil {
				log.Printf("Failed to process missed message %s: %v", m.ID, err)
			} else {
				count++
			}
		}

		if count > 0 {
			log.Printf("Backfilled %d messages for channel %s (%s)", count, channelName, channelID)
		}
	}
	log.Println("Catch-up routine complete.")
}

func processMissedMessage(dg *discordgo.Session, eventServiceURL string, serverID string, m *discordgo.Message, channelName string) error {
	content := m.Content

	// Resolve Embeds
	if content == "" && len(m.Embeds) > 0 {
		var parts []string
		for _, embed := range m.Embeds {
			if embed.Title != "" {
				parts = append(parts, embed.Title)
			}
			if embed.Description != "" {
				parts = append(parts, embed.Description)
			}
			for _, field := range embed.Fields {
				parts = append(parts, fmt.Sprintf("%s: %s", field.Name, field.Value))
			}
		}
		content = strings.Join(parts, "\n")
	}

	// Resolve Mentions
	for _, user := range m.Mentions {
		content = strings.ReplaceAll(content, fmt.Sprintf("<@%s>", user.ID), fmt.Sprintf("@%s", user.Username))
		content = strings.ReplaceAll(content, fmt.Sprintf("<@!%s>", user.ID), fmt.Sprintf("@%s", user.Username))
	}

	var attachments []Attachment
	for _, a := range m.Attachments {
		attachments = append(attachments, Attachment{
			ID:          a.ID,
			URL:         a.URL,
			ProxyURL:    a.ProxyURL,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			Height:      a.Height,
			Width:       a.Width,
		})
	}

	eventType := EventTypeMessagingUserSentMessage
	if m.WebhookID != "" {
		eventType = EventTypeMessagingWebhookMessage
	}

	event := UserSentMessageEvent{
		GenericMessagingEvent: GenericMessagingEvent{
			Type:        eventType,
			Source:      "discord",
			UserID:      m.Author.ID,
			UserName:    m.Author.Username,
			ChannelID:   m.ChannelID,
			ChannelName: channelName,
			ServerID:    serverID,
			Timestamp:   m.Timestamp,
		},
		MessageID:    m.ID,
		Content:      content,
		MentionedBot: false,
		Attachments:  attachments,
	}

	for _, user := range m.Mentions {
		if user.ID == dg.State.User.ID {
			event.MentionedBot = true
			break
		}
	}

	return SendEventData(eventServiceURL, event)
}

// getRecentActiveChannels fetches the last N messaging events and returns a map of ChannelID -> LatestTimestamp.
func getRecentActiveChannels(serviceURL string, limit int) (map[string]time.Time, error) {
	// We want events that represent user activity: messaging.user.sent_message
	url := fmt.Sprintf("%s/events?max_length=%d&format=json&event.type=messaging.user.sent_message", serviceURL, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var result struct {
		Events []struct {
			Event     json.RawMessage `json:"event"`
			Timestamp int64           `json:"timestamp"`
		} `json:"events"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	activeMap := make(map[string]time.Time)

	// Default safety floor: 1 hour ago
	// If a channel is found but has weird data, we rely on this? No, we rely on the event timestamp.

	for _, e := range result.Events {
		var ed map[string]interface{}
		if err := json.Unmarshal(e.Event, &ed); err != nil {
			continue
		}

		channelID, _ := ed["channel_id"].(string)
		if channelID == "" {
			continue
		}

		// Use the event's timestamp
		eventTime := time.Unix(e.Timestamp, 0)

		// We want the LATEST timestamp seen for this channel
		if current, exists := activeMap[channelID]; !exists || eventTime.After(current) {
			activeMap[channelID] = eventTime
		}
	}

	return activeMap, nil
}

// fetchRecentEventMessageIDs retrieves the last N message IDs that have already been emitted as events.
func fetchRecentEventMessageIDs(serviceURL string, limit int) (map[string]bool, error) {
	url := fmt.Sprintf("%s/events?max_length=%d&format=json&event.type=messaging.user.sent_message", serviceURL, limit)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var result struct {
		Events []struct {
			Event json.RawMessage `json:"event"`
		} `json:"events"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	idMap := make(map[string]bool)
	for _, e := range result.Events {
		var ed map[string]interface{}
		if err := json.Unmarshal(e.Event, &ed); err == nil {
			if mid, ok := ed["message_id"].(string); ok {
				idMap[mid] = true
			}
		}
	}

	return idMap, nil
}

// timeToSnowflake converts a time.Time to a Discord Snowflake ID
func timeToSnowflake(t time.Time) string {
	const discordEpoch = 1420070400000
	timestamp := t.UnixNano() / int64(time.Millisecond)
	snowflake := (timestamp - discordEpoch) << 22
	return fmt.Sprintf("%d", snowflake)
}

// SendEventData is a helper to send event data (exported for reuse if needed)
func SendEventData(serviceURL string, eventData interface{}) error {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	request := map[string]interface{}{"service": "dex-discord-service", "event": json.RawMessage(eventJSON)}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := http.Post(serviceURL+"/events", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("api returned status %d", resp.StatusCode)
	}
	return nil
}
