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
func FetchMissedMessages(dg *discordgo.Session, eventServiceURL string, serverID string) {
	log.Println("Starting catch-up routine...")

	// 1. Determine the "last seen" timestamp.
	// We MUST find a reliable timestamp from the event service.
	// If we cannot find one, we assume no backfill is safe/needed to avoid duplication.
	latestEventTime, err := fetchLatestEventTimestamp(eventServiceURL)
	if err != nil || latestEventTime.IsZero() {
		log.Printf("Catch-up: Could not fetch last event timestamp (err: %v). Aborting backfill to prevent duplication.", err)
		return
	}

	lastSeenTime := latestEventTime
	log.Printf("Last known event timestamp: %s", lastSeenTime.Format(time.RFC3339))

	// 2. Get all text channels in the guild.
	channels, err := dg.GuildChannels(serverID)
	if err != nil {
		log.Printf("Error fetching guild channels: %v", err)
		return
	}

	for _, channel := range channels {
		// Only check text channels
		if channel.Type != discordgo.ChannelTypeGuildText {
			continue
		}

		// 3. Fetch messages since lastSeenTime
		// Discord API 'After' takes a message ID, not a timestamp.
		// We use a snowflake from lastSeenTime + 1ms to avoid re-fetching the exact last seen message.
		snowflake := timeToSnowflake(lastSeenTime.Add(1 * time.Millisecond))

		// Pre-fetch last 10000 message IDs from event service to prevent duplicate events
		// This deep search is critical to ensure we don't re-process messages we've already handled.
		existingIDs, _ := fetchRecentEventMessageIDs(eventServiceURL, 10000)

		messages, err := dg.ChannelMessages(channel.ID, 100, "", snowflake, "")
		if err != nil {
			log.Printf("Error fetching messages for channel %s: %v", channel.Name, err)
			continue
		}

		// Messages are returned newest first. We want to process them chronologically (oldest first).
		// Sort messages by timestamp/ID
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].ID < messages[j].ID // ID is chronologically sortable
		})

		count := 0
		for _, m := range messages {
			// Skip bot's own messages
			if m.Author.ID == dg.State.User.ID {
				continue
			}

			// Skip if we already have an event for this message ID
			if existingIDs[m.ID] {
				continue
			}

			msgTime := m.Timestamp

			// Double check it's actually after our last seen time (redundant with snowflake but safe)
			if !msgTime.After(lastSeenTime) {
				continue
			}

			// Construct and Emit Event
			// We need to replicate the logic from messageCreate in core.go
			// Ideally refactor messageCreate to use a shared helper, but for now we duplicate carefully.

			content := m.Content

			// If content is empty (common with webhooks/embeds), try to build it from embeds
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

			// Handle Mentions
			for _, user := range m.Mentions {
				// We don't have easy access to Redis here for nickname caching without passing it down.
				// For catch-up, we might skip the redis lookup or pass nil/implement fallback.
				// Let's rely on user.Username for now.
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
					UserName:    m.Author.Username, // Simplified vs core.go which uses display name
					ChannelID:   m.ChannelID,
					ChannelName: channel.Name,
					ServerID:    serverID,
					Timestamp:   msgTime,
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

			if err := SendEventData(eventServiceURL, event); err != nil {
				log.Printf("Error sending missed message event: %v", err)
			} else {
				count++
			}
		}
		if count > 0 {
			log.Printf("Backfilled %d messages for channel %s", count, channel.Name)
		}
	}
	log.Println("Catch-up routine complete.")
}

// fetchRecentEventMessageIDs retrieves the last N message IDs that have already been emitted as events.
func fetchRecentEventMessageIDs(serviceURL string, limit int) (map[string]bool, error) {
	url := fmt.Sprintf("%s/events?limit=%d&format=json&event.type=messaging.user.sent_message", serviceURL, limit)

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

// fetchLatestEventTimestamp queries the event service for the latest messaging event
func fetchLatestEventTimestamp(serviceURL string) (time.Time, error) {
	// Query parameters to get 1 latest event of type user sent message
	url := fmt.Sprintf("%s/events?limit=1&format=json&event.type=messaging.user.sent_message", serviceURL)

	resp, err := http.Get(url)
	if err != nil {
		return time.Time{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, fmt.Errorf("bad status: %d", resp.StatusCode)
	}

	var result struct {
		Events []struct {
			Timestamp int64 `json:"timestamp"`
		} `json:"events"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return time.Time{}, err
	}

	if len(result.Events) > 0 {
		return time.Unix(result.Events[0].Timestamp, 0), nil
	}

	return time.Time{}, fmt.Errorf("no events found")
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
