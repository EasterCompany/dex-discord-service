package cache

import (
	"context"

	"fmt"

	"log"

	"strings"

	"time"
)

const (
	LogsKey = "dexter:discord:logs"
	maxLogs = 100 // Max number of log entries to store in Redis
)

// LogWriter is an io.Writer that captures log output and sends it to Redis.
type LogWriter struct {
	redisClient *RedisClient
}

// NewLogWriter creates a new LogWriter.
func NewLogWriter(client *RedisClient) *LogWriter {
	return &LogWriter{
		redisClient: client,
	}
}

// Write implements the io.Writer interface.
func (lw *LogWriter) Write(p []byte) (n int, err error) {
	// The input from the log package includes a newline, which we trim.
	logEntry := strings.TrimRight(string(p), "\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = lw.redisClient.AddToList(ctx, LogsKey, logEntry, maxLogs)
	if err != nil {
		// Log to stderr if Redis fails, to avoid infinite loop
		_, _ = fmt.Fprintf(log.Writer(), "[ERROR] Failed to write log to Redis: %v\n", err)
	}

	// Also write to original stderr to keep console logs
	return fmt.Fprint(log.Writer(), string(p))
}
