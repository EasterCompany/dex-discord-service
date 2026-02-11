package utils

import (
	"encoding/json"
)

// Message represents a single message in a chat history
type Message struct {
	Role       string      `json:"role"`
	Content    string      `json:"content"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	ToolCallID string      `json:"tool_call_id,omitempty"` // For tool responses
}

// ToolCall represents a request from the model to call a tool
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // always "function"
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // JSON string
	} `json:"function"`
}

// ModelRequest is the unified request structure for all Dexter model services
type ModelRequest struct {
	Model    string                 `json:"model"`
	Prompt   string                 `json:"prompt,omitempty"`
	Messages []Message              `json:"messages,omitempty"`
	Tools    []Tool                 `json:"tools,omitempty"`
	Stream   bool                   `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// Tool represents a tool available to the model
type Tool struct {
	Type     string `json:"type"` // always "function"
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters,omitempty"` // JSON Schema
	} `json:"function"`
}

// ModelResponse is the unified response structure
type ModelResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}
