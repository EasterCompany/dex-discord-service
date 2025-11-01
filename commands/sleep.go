package commands

import "log"

// handleSleep handles the /sleep command
func (h *Handler) handleSleep(channelID string) {
	log.Println("[COMMAND] Entering sleep mode")
	h.sendResponse(channelID, "😴 Entering sleep mode...")

	// Create a task for sleep mode
	taskID := h.createTask("sleep_mode", map[string]interface{}{
		"actions": []string{
			"disconnect_voice",
			"pause_dashboard_updates",
			"set_status_sleeping",
		},
	})

	h.sendResponse(channelID, "✅ Sleep mode task created\n"+
		"The bot will:\n"+
		"• Disconnect from any voice channels\n"+
		"• Pause dashboard updates\n"+
		"• Set Discord status to sleeping\n"+
		"\nTask ID: `"+taskID+"`")
}
