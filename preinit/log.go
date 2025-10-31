// Package preinit provides a logger for the application before the Discord session is initialized.
package preinit

import (
	"log"
	"os"
)

// Logger is a simple logger that writes to stderr.
type Logger struct{}

// NewLogger creates a new preinit logger.
func NewLogger() *Logger {
	return &Logger{}
}

// Error logs an error to stderr.
func (l *Logger) Error(context string, err error) {
	log.Printf("[ERROR] %s: %v\n", context, err)
}

// Fatal logs an error to stderr and exits the program.
func (l *Logger) Fatal(context string, err error) {
	l.Error(context, err)
	os.Exit(1)
}
