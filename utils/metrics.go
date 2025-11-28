package utils

import "sync/atomic"

// Metrics holds counters for service operations
var (
	messagesReceived  int64
	messagesSent      int64
	eventsSent        int64
	discordReconnects int64
)

// IncrementMessagesReceived atomically increments the messages received counter
func IncrementMessagesReceived() {
	atomic.AddInt64(&messagesReceived, 1)
}

// IncrementMessagesSent atomically increments the messages sent counter
func IncrementMessagesSent() {
	atomic.AddInt64(&messagesSent, 1)
}

// IncrementEventsSent atomically increments the events sent counter
func IncrementEventsSent() {
	atomic.AddInt64(&eventsSent, 1)
}

// IncrementReconnects atomically increments the reconnection counter
func IncrementReconnects() {
	atomic.AddInt64(&discordReconnects, 1)
}

// GetMetrics returns the current metrics as a map
func GetMetrics() map[string]interface{} {
	return map[string]interface{}{
		"messages_received":  atomic.LoadInt64(&messagesReceived),
		"messages_sent":      atomic.LoadInt64(&messagesSent),
		"events_sent":        atomic.LoadInt64(&eventsSent),
		"discord_reconnects": atomic.LoadInt64(&discordReconnects),
	}
}
