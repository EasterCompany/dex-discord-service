package dashboard

import (
	"fmt"
	"log"
	"time"
)

// UpdateThrottled updates the dashboard with throttling
// - Always updates the cache
// - Only pushes to Discord API if throttle period has passed
func UpdateThrottled(cache *MessageCache, session Session, channelID string, newContent string) error {
	now := time.Now()

	// Always update the cache
	cache.Content = newContent
	cache.LastUpdate = now

	// Check if we should update via API (throttle check)
	if now.Sub(cache.LastAPIUpdate) < cache.ThrottleDuration {
		// Still within throttle period, skip API update
		return nil
	}

	// Update via Discord API
	if cache.MessageID == "" {
		return fmt.Errorf("message ID not set, cannot update")
	}

	log.Printf("[DASHBOARD_UPDATE] MessageID: %s | Pushing cached content to Discord\n", cache.MessageID)

	_, err := session.ChannelMessageEdit(channelID, cache.MessageID, cache.Content)
	if err != nil {
		return fmt.Errorf("failed to update dashboard: %w", err)
	}

	cache.LastAPIUpdate = now
	return nil
}

// ForceUpdateNow bypasses throttle and updates immediately
func ForceUpdateNow(cache *MessageCache, session Session, channelID string, newContent string) error {
	now := time.Now()

	cache.Content = newContent
	cache.LastUpdate = now

	if cache.MessageID == "" {
		return fmt.Errorf("message ID not set, cannot update")
	}

	log.Printf("[DASHBOARD_FORCE_UPDATE] MessageID: %s | Force updating\n", cache.MessageID)

	_, err := session.ChannelMessageEdit(channelID, cache.MessageID, cache.Content)
	if err != nil {
		return fmt.Errorf("failed to force update dashboard: %w", err)
	}

	cache.LastAPIUpdate = now
	return nil
}
