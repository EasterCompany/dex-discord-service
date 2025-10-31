// Package reporting handles the generation of status reports.
package reporting

import (
	"fmt"

	logger "github.com/EasterCompany/dex-discord-interface/log"
)

// BootMessage handles the startup message.
type BootMessage struct {
	Logger    logger.Logger
	MessageID string
}

// NewBootMessage creates a new BootMessage.
func NewBootMessage(logger logger.Logger) *BootMessage {
	return &BootMessage{Logger: logger}
}

// PostInitialMessage posts the initial startup message.
func (b *BootMessage) PostInitialMessage() {
	bootMessage, err := b.Logger.PostInitialMessage("Dexter is starting up...")
	if err != nil {
		b.Logger.Error("Failed to post initial boot message", err)
		return
	}
	if bootMessage != nil {
		b.MessageID = bootMessage.ID
	}
}

// Update posts an update to the startup message.
func (b *BootMessage) Update(status string) {
	if b.MessageID != "" {
		content := fmt.Sprintf("Dexter is starting up...\n%s", status)
		b.Logger.UpdateInitialMessage(b.MessageID, content)
	}
}
