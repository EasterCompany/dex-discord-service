package llm

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type LLMResponse struct {
	XMLName xml.Name `xml:"response"`
	Think   string   `xml:"think"`
	Say     string   `xml:"say"`
	React   []string `xml:"react"`
}

type streamState int

const (
	stateSearching streamState = iota
	stateThinking
	stateStreamingSay
	stateFinished
)

func (c *Client) processStream(s *discordgo.Session, triggeringMessage *discordgo.Message, body io.ReadCloser) error {
	reader := bufio.NewReader(body)
	var fullContent strings.Builder
	var responseMessage *discordgo.Message
	var lastEdit time.Time
	currentState := stateSearching
	const editInterval = 1 * time.Second

	for currentState != stateFinished {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			if responseMessage != nil {
				_, _ = s.ChannelMessageEdit(triggeringMessage.ChannelID, responseMessage.ID, "An error occurred while generating the response.")
			}
			return fmt.Errorf("error reading stream: %w", err)
		}

		var streamResp OllamaStreamResponse
		if err := json.Unmarshal(line, &streamResp); err != nil {
			continue
		}

		fullContent.WriteString(streamResp.Message.Content)
		currentBuffer := fullContent.String()

		switch currentState {
		case stateSearching:
			if strings.Contains(currentBuffer, "<think>") {
				msg, err := s.ChannelMessageSend(triggeringMessage.ChannelID, "<a:loading:1429533488687747182> Thinking...")
				if err != nil {
					return fmt.Errorf("failed to send 'thinking' message: %w", err)
				}
				responseMessage = msg
				currentState = stateThinking
			}
		case stateThinking:
			if sayIndex := strings.Index(currentBuffer, "<say>"); sayIndex != -1 {
				// Check if <think> is open and unclosed
				if thinkIndex := strings.Index(currentBuffer, "<think>"); thinkIndex != -1 {
					if !strings.Contains(currentBuffer[:sayIndex], "</think>") {
						currentBuffer = currentBuffer[:sayIndex] + "</think>" + currentBuffer[sayIndex:]
						sayIndex += len("</think>")
						fullContent.Reset()
						fullContent.WriteString(currentBuffer)
					}
				}

				initialContent := currentBuffer[sayIndex+len("<say>"):]
				if initialContent != "" {
					_, err := s.ChannelMessageEdit(triggeringMessage.ChannelID, responseMessage.ID, initialContent)
					if err != nil {
						return fmt.Errorf("failed to edit message for streaming: %w", err)
					}
				}
				lastEdit = time.Now()
				currentState = stateStreamingSay
			} else if reactIndex := strings.Index(currentBuffer, "<react>"); reactIndex != -1 {
				// Check if <think> is open and unclosed
				if thinkIndex := strings.Index(currentBuffer, "<think>"); thinkIndex != -1 {
					if !strings.Contains(currentBuffer[:reactIndex], "</think>") {
						currentBuffer = currentBuffer[:reactIndex] + "</think>" + currentBuffer[reactIndex:]
						fullContent.Reset()
						fullContent.WriteString(currentBuffer)
					}
				}
			}
		case stateStreamingSay:
			if time.Since(lastEdit) > editInterval {
				sayStartIndex := strings.Index(currentBuffer, "<say>")
				if sayStartIndex == -1 {
					continue
				}
				sayContent := currentBuffer[sayStartIndex+len("<say>"):]

				if sayEndIndex := strings.Index(sayContent, "</say>"); sayEndIndex != -1 {
					sayContent = sayContent[:sayEndIndex]
				}

				if responseMessage.Content != sayContent {
					_, err := s.ChannelMessageEdit(triggeringMessage.ChannelID, responseMessage.ID, sayContent)
					if err != nil {
						fmt.Printf("Failed to edit streaming message: %v\n", err)
					}
					responseMessage.Content = sayContent
				}
				lastEdit = time.Now()
			}
		}

		if streamResp.Done {
			currentState = stateFinished
		}
	}

	rawResponse := fullContent.String()
	// Check for open <say> tag and close it if necessary
	if strings.Contains(rawResponse, "<say>") && !strings.Contains(rawResponse, "</say>") {
		rawResponse += "</say>"
	}

	// Ensure the response is wrapped in <response> tags
	if !strings.HasPrefix(rawResponse, "<response>") {
		rawResponse = "<response>" + rawResponse
	}
	if !strings.HasSuffix(rawResponse, "</response>") {
		rawResponse += "</response>"
	}

	fmt.Printf("Raw LLM Response: %s\n", rawResponse)

	cleanResponse := strings.TrimSpace(rawResponse)
	if strings.HasPrefix(cleanResponse, "```xml") {
		cleanResponse = strings.TrimPrefix(cleanResponse, "```xml")
		cleanResponse = strings.TrimSuffix(cleanResponse, "```")
		cleanResponse = strings.TrimSpace(cleanResponse)
	}

	fmt.Printf("Cleaned LLM Response: %s\n", cleanResponse)

	var response LLMResponse
	thinkRegex := regexp.MustCompile(`(?s)<think>(.*?)<\/think>`)
	sayRegex := regexp.MustCompile(`(?s)<say>(.*?)<\/say>`)
	reactRegex := regexp.MustCompile(`(?s)<react>(.*?)<\/react>`)

	thinkMatches := thinkRegex.FindStringSubmatch(cleanResponse)
	sayMatches := sayRegex.FindStringSubmatch(cleanResponse)
	reactMatches := reactRegex.FindAllStringSubmatch(cleanResponse, -1)

	if thinkMatches != nil {
		response.Think = thinkMatches[1]
	}

	if sayMatches != nil {
		response.Say = sayMatches[1]
	}

	for _, match := range reactMatches {
		response.React = append(response.React, match[1])
	}

	if response.Say == "" && len(response.React) == 0 {
		fmt.Printf("Final XML parsing failed. Raw response was: %s\n", rawResponse)
		if responseMessage != nil {
			_, _ = s.ChannelMessageEdit(triggeringMessage.ChannelID, responseMessage.ID, "Sorry, I had a brain fart and couldn't figure out how to respond.")
		}
		return nil
	}

	if response.Say != "" && responseMessage != nil {
		if responseMessage.Content != response.Say {
			_, _ = s.ChannelMessageEdit(triggeringMessage.ChannelID, responseMessage.ID, response.Say)
		}
	} else if response.Say == "" && responseMessage != nil {
		_ = s.ChannelMessageDelete(triggeringMessage.ChannelID, responseMessage.ID)
	}

	for _, emoji := range response.React {
		err := s.MessageReactionAdd(triggeringMessage.ChannelID, triggeringMessage.ID, emoji)
		if err != nil {
			fmt.Printf("Failed to add reaction '%s': %v\n", emoji, err)
		}
	}

	return nil
}
