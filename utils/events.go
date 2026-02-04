package utils

import "time"

// EventType is a custom type for our event types
type EventType string

// Constants for our event types
const (
	// Messaging Events
	EventTypeMessagingUserJoinedVoice     EventType = "messaging.user.joined_voice"
	EventTypeMessagingUserLeftVoice       EventType = "messaging.user.left_voice"
	EventTypeMessagingUserSentMessage     EventType = "messaging.user.sent_message"
	EventTypeMessagingBotStatusUpdate     EventType = "messaging.bot.status_update"
	EventTypeMessagingUserSpeakingStarted EventType = "messaging.user.speaking.started"
	EventTypeMessagingUserSpeakingStopped EventType = "messaging.user.speaking.stopped"
	EventTypeMessagingUserTranscribed     EventType = "messaging.user.transcribed"
	EventTypeMessagingUserJoinedServer    EventType = "messaging.user.joined_server"
	EventTypeMessagingBotVoiceResponse    EventType = "messaging.bot.voice_response"
	EventTypeMessagingWebhookMessage      EventType = "messaging.webhook.message"

	// System Events
	EventTypeSystemStatusChange EventType = "system.status.change"
)

// GenericMessagingEvent contains common fields for all messaging-related events
type GenericMessagingEvent struct {
	Type            EventType `json:"type"`
	Source          string    `json:"source"` // e.g., "discord", "slack"
	UserID          string    `json:"user_id"`
	UserName        string    `json:"user_name"`
	UserLevel       string    `json:"user_level"`
	ChannelID       string    `json:"channel_id"`
	ChannelName     string    `json:"channel_name"`
	ParentChannelID string    `json:"parent_channel_id,omitempty"`
	ServerID        string    `json:"server_id"`
	ServerName      string    `json:"server_name,omitempty"`
	Timestamp       time.Time `json:"timestamp"`
}

// Attachment represents a file attached to a message
type Attachment struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	ProxyURL    string `json:"proxy_url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
	Height      int    `json:"height,omitempty"`
	Width       int    `json:"width,omitempty"`
}

// UserSentMessageEvent is the payload for EventTypeMessagingUserSentMessage
type UserSentMessageEvent struct {
	GenericMessagingEvent
	MessageID    string       `json:"message_id"`
	Content      string       `json:"content"`
	MentionedBot bool         `json:"mentioned_bot"`
	Attachments  []Attachment `json:"attachments,omitempty"`
}

// UserVoiceStateChangeEvent is the payload for voice channel join/leave events
type UserVoiceStateChangeEvent struct {
	GenericMessagingEvent
}

// UserServerEvent is the payload for server-level user events
type UserServerEvent struct {
	GenericMessagingEvent
}

// BotStatusUpdateEvent is the payload for the bot's own status changes
type BotStatusUpdateEvent struct {
	Type      EventType `json:"type"`
	Source    string    `json:"source"`
	Status    string    `json:"status"`
	Details   string    `json:"details"`
	Timestamp time.Time `json:"timestamp"`
}

// UserSpeakingEvent is for when a user starts or stops speaking
type UserSpeakingEvent struct {
	GenericMessagingEvent
	SSRC uint32 `json:"ssrc"`
}

// UserTranscribedEvent is for when a user's speech is transcribed
type UserTranscribedEvent struct {
	GenericMessagingEvent
	Transcription string `json:"transcription"`
	Content       string `json:"content"`
}
