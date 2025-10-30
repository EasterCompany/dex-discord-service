package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// OllamaStreamResponse is the structure of a single chunk in the Ollama stream.
type OllamaStreamResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message"`
	Done      bool      `json:"done"`
}

// processStream handles the streaming of the LLM response to a Discord message.
func (c *Client) processStream(ctx context.Context, s *discordgo.Session, triggeringMessage *discordgo.Message, body io.ReadCloser) (*discordgo.Message, error) {
	reader := bufio.NewReader(body)
	var fullContent strings.Builder
	var responseMessage *discordgo.Message
	var err error
	var line []byte

	// Read the first chunk to create the initial message
	firstChunk := true
streamLoop:
	for {
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		default:
			line, err = reader.ReadBytes('\n')
			if err == io.EOF {
				break streamLoop
			}
			if err != nil {
				return nil, fmt.Errorf("error reading stream: %w", err)
			}

			var streamResp OllamaStreamResponse
			if err := json.Unmarshal(line, &streamResp); err != nil {
				continue
			}

			fullContent.WriteString(streamResp.Message.Content)

			if firstChunk && fullContent.Len() > 0 {
				responseMessage, err = s.ChannelMessageSend(triggeringMessage.ChannelID, fullContent.String())
				if err != nil {
					return nil, fmt.Errorf("failed to send initial message: %w", err)
				}
				firstChunk = false
			} else if !firstChunk {
				if responseMessage.Content != fullContent.String() {
					_, _ = s.ChannelMessageEdit(responseMessage.ChannelID, responseMessage.ID, fullContent.String())
					responseMessage.Content = fullContent.String()
				}
			}

			if streamResp.Done {
				break streamLoop
			}
		}
	}

	// Final update to ensure the message is complete
	if responseMessage != nil && responseMessage.Content != fullContent.String() {
		_, _ = s.ChannelMessageEdit(responseMessage.ChannelID, responseMessage.ID, fullContent.String())
	}

	return responseMessage, nil
}
