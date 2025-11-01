package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Task represents a task in the system
type Task struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Status    string                 `json:"status"` // pending, running, completed, failed
	CreatedAt time.Time              `json:"created_at"`
	Data      map[string]interface{} `json:"data"`
}

// createTask creates a new task in Redis
func (h *Handler) createTask(taskType string, data map[string]interface{}) string {
	taskID := uuid.New().String()[:8] // Short UUID

	task := Task{
		ID:        taskID,
		Type:      taskType,
		Status:    "pending",
		CreatedAt: time.Now(),
		Data:      data,
	}

	taskJSON, err := json.Marshal(task)
	if err != nil {
		log.Printf("[TASK] Failed to marshal task: %v", err)
		return ""
	}

	// Store task in Redis with key: dex-discord-service:task:<id>
	key := fmt.Sprintf("dex-discord-service:task:%s", taskID)
	ctx := context.Background()

	if err := h.redisClient.Set(ctx, key, taskJSON, 24*time.Hour); err != nil {
		log.Printf("[TASK] Failed to store task in Redis: %v", err)
		return ""
	}

	// Add task ID to task queue
	queueKey := "dex-discord-service:task:queue"
	if err := h.redisClient.RPush(ctx, queueKey, taskID); err != nil {
		log.Printf("[TASK] Failed to add task to queue: %v", err)
		return ""
	}

	log.Printf("[TASK] Created task %s (type: %s)", taskID, taskType)
	return taskID
}

// handleTasks displays all tasks in the queue
func (h *Handler) handleTasks(channelID string) {
	ctx := context.Background()
	queueKey := "dex-discord-service:task:queue"

	// Get all task IDs from queue
	taskIDs, err := h.redisClient.GetListRange(ctx, queueKey, 0, -1)
	if err != nil {
		h.sendResponse(channelID, fmt.Sprintf("❌ Failed to fetch tasks: %v", err))
		return
	}

	if len(taskIDs) == 0 {
		h.sendResponse(channelID, "✅ Task queue is empty")
		return
	}

	var report strings.Builder
	report.WriteString(fmt.Sprintf("**Task Queue** (%d tasks)\n```\n", len(taskIDs)))

	for i, taskID := range taskIDs {
		taskKey := fmt.Sprintf("dex-discord-service:task:%s", taskID)
		taskJSON, err := h.redisClient.Get(ctx, taskKey).Result()
		if err != nil {
			report.WriteString(fmt.Sprintf("%d. [ERROR] Task %s not found\n", i+1, taskID))
			continue
		}

		var task Task
		if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
			report.WriteString(fmt.Sprintf("%d. [ERROR] Task %s corrupted\n", i+1, taskID))
			continue
		}

		statusIcon := "⏸️"
		switch task.Status {
		case "running":
			statusIcon = "▶️"
		case "completed":
			statusIcon = "✅"
		case "failed":
			statusIcon = "❌"
		}

		report.WriteString(fmt.Sprintf("%d. %s %s (%s) - %s\n",
			i+1, statusIcon, task.ID, task.Type, task.Status))
	}

	report.WriteString("```")
	h.sendResponse(channelID, report.String())
}
