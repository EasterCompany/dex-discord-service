package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/bwmarrin/discordgo"
)

type Client struct {
	httpClient   *http.Client
	OllamaURL    string
	Model        string
	SystemPrompt string
	Cache        cache.Cache
}

func NewClient(persona *interfaces.Persona, cache cache.Cache) (*Client, error) {
	systemPrompt, err := createSystemMessage(persona)
	if err != nil {
		return nil, fmt.Errorf("failed to create system message: %w", err)
	}

	return &Client{
		httpClient:   &http.Client{},
		OllamaURL:    "http://localhost:11434/api/chat",
		Model:        "dolphin3:latest",
		SystemPrompt: systemPrompt,
		Cache:        cache,
	}, nil
}

func createSystemMessage(persona *interfaces.Persona) (string, error) {
	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	tmpl, err := template.New("systemMessage").Funcs(funcMap).Parse(systemMessageTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse system message template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, persona); err != nil {
		return "", fmt.Errorf("failed to execute system message template: %w", err)
	}

	return buf.String(), nil
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type OllamaResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   Message   `json:"message"`
	Done      bool      `json:"done"`
}

type EngagementCheckContext struct {
	Guild   *discordgo.Guild
	Channel *discordgo.Channel
	Message *discordgo.Message
	History []*discordgo.Message
}

func (c *Client) ShouldEngage(s *discordgo.Session, m *discordgo.MessageCreate, history []*discordgo.Message) (bool, error) {
	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		return false, fmt.Errorf("could not get channel: %w", err)
	}

	var guild *discordgo.Guild
	if m.GuildID != "" {
		guild, err = s.State.Guild(m.GuildID)
		if err != nil {
			return false, fmt.Errorf("could not get guild: %w", err)
		}
	} else {
		guild = &discordgo.Guild{Name: "Direct Message"}
	}

	engagementCtx := EngagementCheckContext{
		Guild:   guild,
		Channel: channel,
		Message: m.Message,
		History: history,
	}

	var promptBuf bytes.Buffer
	tmpl, err := template.New("engagementCheck").Parse(engagementCheckTemplate)
	if err != nil {
		return false, fmt.Errorf("failed to parse engagement check template: %w", err)
	}
	if err := tmpl.Execute(&promptBuf, &engagementCtx); err != nil {
		return false, fmt.Errorf("failed to execute engagement check template: %w", err)
	}

	request := OllamaRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "user", Content: promptBuf.String()},
		},
		Stream: false,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return false, fmt.Errorf("failed to marshal engagement request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", c.OllamaURL, bytes.NewBuffer(payload))
	if err != nil {
		return false, fmt.Errorf("failed to create engagement request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to send engagement request to ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("ollama returned non-200 status for engagement check: %s", resp.Status)
	}

	var ollamaResp OllamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return false, fmt.Errorf("failed to decode engagement response: %w", err)
	}

	return strings.TrimSpace(ollamaResp.Message.Content) == "ENGAGE", nil
}

type ContextBlock struct {
	Timestamp    string
	Guild        *discordgo.Guild
	Channel      *discordgo.Channel
	Message      *discordgo.Message
	OnlineUsers  []string
	OfflineUsers []string
}

func (c *Client) GenerateContextBlock(s *discordgo.Session, m *discordgo.MessageCreate) (string, error) {
	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		return "", fmt.Errorf("could not get channel: %w", err)
	}

	var guild *discordgo.Guild
	var onlineUsers, offlineUsers []string

	if m.GuildID != "" {
		guild, err = s.State.Guild(m.GuildID)
		if err != nil {
			return "", fmt.Errorf("could not get guild: %w", err)
		}
		for _, pres := range guild.Presences {
			user, err := s.User(pres.User.ID)
			if err != nil {
				continue
			}
			if pres.Status == discordgo.StatusOnline || pres.Status == discordgo.StatusDoNotDisturb || pres.Status == discordgo.StatusIdle {
				onlineUsers = append(onlineUsers, user.Username)
			} else {
				offlineUsers = append(offlineUsers, user.Username)
			}
		}
	} else {
		guild = &discordgo.Guild{Name: "Direct Message", ID: "N/A"}
		onlineUsers = append(onlineUsers, m.Author.Username)
	}

	contextBlock := ContextBlock{
		Timestamp:    time.Now().Format(time.RFC1123),
		Guild:        guild,
		Channel:      channel,
		Message:      m.Message,
		OnlineUsers:  onlineUsers,
		OfflineUsers: offlineUsers,
	}

	funcMap := template.FuncMap{"join": strings.Join}
	tmpl, err := template.New("contextBlock").Funcs(funcMap).Parse(contextBlockTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse context block template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, contextBlock); err != nil {
		return "", fmt.Errorf("failed to execute context block template: %w", err)
	}

	return buf.String(), nil
}

func (c *Client) CreateOllamaPayload(messages []*discordgo.Message, contextBlock string) ([]byte, error) {
	ollamaMessages := []Message{
		{
			Role:    "system",
			Content: c.SystemPrompt,
		},
		{
			Role:    "system",
			Content: contextBlock,
		},
	}

	for _, msg := range messages {
		role := "user"
		if msg.Author.Bot {
			role = "assistant"
		}
		ollamaMessages = append(ollamaMessages, Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	request := OllamaRequest{
		Model:    c.Model,
		Messages: ollamaMessages,
		Stream:   true,
	}

	return json.Marshal(request)
}

func (c *Client) StreamChatCompletion(s *discordgo.Session, triggeringMessage *discordgo.Message, messages []*discordgo.Message, contextBlock string) error {
	payload, err := c.CreateOllamaPayload(messages, contextBlock)
	if err != nil {
		return fmt.Errorf("failed to create ollama payload: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", c.OllamaURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to ollama: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	return c.processStream(s, triggeringMessage, resp.Body)
}
