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
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/bwmarrin/discordgo"
)

type Client struct {
	httpClient          *http.Client
	LLMServerURL        string
	EngagementModel     string
	ConversationalModel string
	SystemPrompt        string
	Cache               cache.Cache
}

func NewClient(persona *interfaces.Persona, botConfig *config.BotConfig, cache cache.Cache) (*Client, error) {
	systemPrompt, err := createSystemMessage(persona)
	if err != nil {
		return nil, fmt.Errorf("failed to create system message: %w", err)
	}

	return &Client{
		httpClient:          &http.Client{},
		LLMServerURL:        botConfig.LLMServerURL,
		EngagementModel:     botConfig.EngagementModel,
		ConversationalModel: botConfig.ConversationalModel,
		SystemPrompt:        systemPrompt,
		Cache:               cache,
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

type LLMServerRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type LLMServerResponse struct {
	Model     string    `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	Message   struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

type EngagementDecision struct {
	Action   string `json:"action"`
	Argument string `json:"argument"`
}

type EngagementCheckContext struct {
	Guild   *discordgo.Guild
	Channel *discordgo.Channel
	Message *discordgo.Message
	History []*discordgo.Message
}

func (c *Client) getEngagementContext(s *discordgo.Session, m *discordgo.MessageCreate) (*discordgo.Guild, *discordgo.Channel, error) {
	channel, err := s.State.Channel(m.ChannelID)
	if err != nil {
		channel, err = s.Channel(m.ChannelID)
		if err != nil {
			return nil, nil, fmt.Errorf("could not get channel: %w", err)
		}
	}

	var guild *discordgo.Guild
	if m.GuildID != "" {
		guild, err = s.State.Guild(m.GuildID)
		if err != nil {
			return nil, nil, fmt.Errorf("could not get guild: %w", err)
		}
	} else {
		guild = &discordgo.Guild{Name: "Direct Message"}
	}

	return guild, channel, nil
}

func (c *Client) buildEngagementPrompt(guild *discordgo.Guild, channel *discordgo.Channel, m *discordgo.MessageCreate, history []*discordgo.Message) (string, error) {
	engagementCtx := EngagementCheckContext{
		Guild:   guild,
		Channel: channel,
		Message: m.Message,
		History: history,
	}

	var promptBuf bytes.Buffer
	tmpl, err := template.New("engagementCheck").Parse(engagementCheckTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse engagement check template: %w", err)
	}
	if err := tmpl.Execute(&promptBuf, &engagementCtx); err != nil {
		return "", fmt.Errorf("failed to execute engagement check template: %w", err)
	}

	return promptBuf.String(), nil
}

func (c *Client) sendEngagementRequest(prompt string) (*LLMServerResponse, error) {
	request := LLMServerRequest{
		Model: c.EngagementModel,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal engagement request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", c.LLMServerURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create engagement request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send engagement request to LLM server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("LLM server returned non-200 status for engagement check: %s", resp.Status)
	}

	var llmResp LLMServerResponse
	if err := json.NewDecoder(resp.Body).Decode(&llmResp); err != nil {
		return nil, fmt.Errorf("failed to decode engagement response: %w", err)
	}

	return &llmResp, nil
}

func (c *Client) parseEngagementResponse(llmResp *LLMServerResponse) (string, string) {
	var decision EngagementDecision
	if err := json.Unmarshal([]byte(llmResp.Message.Content), &decision); err != nil {
		return "IGNORE", ""
	}

	switch decision.Action {
	case "REACT":
		return "REACT", decision.Argument
	case "REPLY":
		return "REPLY", ""
	case "STOP":
		return "STOP", ""
	case "CONTINUE":
		return "CONTINUE", ""
	case "IGNORE":
		return "IGNORE", ""
	}

	return "IGNORE", ""
}

func (c *Client) GetEngagementDecision(s *discordgo.Session, m *discordgo.MessageCreate, history []*discordgo.Message) (string, string, error) {
	guild, channel, err := c.getEngagementContext(s, m)
	if err != nil {
		return "IGNORE", "", err
	}

	prompt, err := c.buildEngagementPrompt(guild, channel, m, history)
	if err != nil {
		return "IGNORE", "", err
	}

	llmResp, err := c.sendEngagementRequest(prompt)
	if err != nil {
		return "IGNORE", "", err
	}

	decision, arg := c.parseEngagementResponse(llmResp)
	return decision, arg, nil
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
		channel, err = s.Channel(m.ChannelID)
		if err != nil {
			return "", fmt.Errorf("could not get channel: %w", err)
		}
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
	llmMessages := []Message{
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
		llmMessages = append(llmMessages, Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	request := LLMServerRequest{
		Model:    c.ConversationalModel,
		Messages: llmMessages,
		Stream:   true,
	}

	return json.Marshal(request)
}

func (c *Client) StreamChatCompletion(ctx context.Context, s *discordgo.Session, triggeringMessage *discordgo.Message, messages []*discordgo.Message, contextBlock string) (*discordgo.Message, error) {
	payload, err := c.CreateOllamaPayload(messages, contextBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM server payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.LLMServerURL, bytes.NewBuffer(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to LLM server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LLM server returned non-200 status: %s, body: %s", resp.Status, string(body))
	}

	return c.processStream(ctx, s, triggeringMessage, resp.Body)
}
