package endpoints

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/EasterCompany/dex-discord-service/utils"
	"github.com/bwmarrin/discordgo"
)

var discordSession *discordgo.Session
var eventServiceURL string

// SetDiscordSession sets the Discord session for the post endpoint
func SetDiscordSession(session *discordgo.Session) {
	discordSession = session
}

// SetEventServiceURL sets the URL for the event service
func SetEventServiceURL(url string) {
	eventServiceURL = url
}

// PostRequest represents the structure of a post request
type PostRequest struct {
	ServerID  string                 `json:"server_id"`  // Discord Guild/Server ID
	ChannelID string                 `json:"channel_id"` // Discord Channel ID
	Content   string                 `json:"content"`    // Text message content (optional if image provided)
	ImageURL  string                 `json:"image_url"`  // URL to image to send (optional)
	Metadata  map[string]interface{} `json:"metadata"`   // Optional metadata (e.g., debug info)
}

// PostHandler handles POST requests to send messages to Discord
func PostHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if Discord session is available
	if discordSession == nil {
		log.Printf("POST ERROR: Discord session not initialized")
		http.Error(w, "Discord service not ready", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	var req PostRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("POST ERROR: Failed to read request body: %v", err)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("Error closing request body: %v", cerr)
		}
	}()

	if err := json.Unmarshal(body, &req); err != nil {
		log.Printf("POST ERROR: Failed to parse JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.ChannelID == "" {
		log.Printf("POST ERROR: Missing channel_id")
		http.Error(w, "channel_id is required", http.StatusBadRequest)
		return
	}

	if req.Content == "" && req.ImageURL == "" {
		log.Printf("POST ERROR: Missing content or image_url")
		http.Error(w, "Either content or image_url is required", http.StatusBadRequest)
		return
	}

	// Send message to Discord
	var message *discordgo.Message
	if req.ImageURL != "" {
		// Fetch the image
		resp, err := http.Get(req.ImageURL)
		if err != nil {
			log.Printf("POST ERROR: Failed to fetch image from URL: %v", err)
			http.Error(w, "Failed to fetch image", http.StatusBadRequest)
			return
		}
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Printf("Error closing image response body: %v", cerr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			log.Printf("POST ERROR: Image URL returned non-OK status: %d", resp.StatusCode)
			http.Error(w, "Failed to fetch image: non-OK status", http.StatusBadRequest)
			return
		}

		// Determine filename from content type
		filename := "image.png"
		contentType := resp.Header.Get("Content-Type")
		switch contentType {
		case "image/jpeg":
			filename = "image.jpg"
		case "image/png":
			filename = "image.png"
		case "image/gif":
			filename = "image.gif"
		case "image/webp":
			filename = "image.webp"
		}

		// Send message with image using ChannelFileSendWithMessage
		message, err = discordSession.ChannelFileSendWithMessage(req.ChannelID, req.Content, filename, resp.Body)
		if err != nil {
			log.Printf("POST ERROR: Failed to send message with image to Discord: %v", err)
			http.Error(w, "Failed to send message to Discord", http.StatusInternalServerError)
			return
		}
	} else {
		// Send text-only message
		message, err = discordSession.ChannelMessageSend(req.ChannelID, req.Content)
		if err != nil {
			log.Printf("POST ERROR: Failed to send message to Discord: %v", err)
			http.Error(w, "Failed to send message to Discord", http.StatusInternalServerError)
			return
		}
	}

	log.Printf("POST SUCCESS: Message sent to channel %s: %s", req.ChannelID, message.ID)
	utils.IncrementMessagesSent()

	// Emit messaging.bot.sent_message event
	go func() {
		if eventServiceURL == "" {
			return
		}

		// Fetch channel details for names
		channel, err := discordSession.Channel(req.ChannelID)
		channelName := "unknown-channel"
		guildID := ""
		if err == nil {
			channelName = channel.Name
			if channel.Type == discordgo.ChannelTypeDM {
				channelName = "DM"
			}
			guildID = channel.GuildID
		}

		// Fetch guild details
		serverName := "DM"
		if guildID != "" {
			guild, err := discordSession.Guild(guildID)
			if err == nil {
				serverName = guild.Name
			}
		}

		// Get Bot User Info
		botUser := discordSession.State.User
		botID := "unknown"
		botName := "Dexter"
		if botUser != nil {
			botID = botUser.ID
			botName = botUser.Username
		}

		eventData := map[string]interface{}{
			"type":         "messaging.bot.sent_message",
			"source":       "discord",
			"user_id":      botID,
			"user_name":    botName,
			"channel_id":   req.ChannelID,
			"channel_name": channelName,
			"server_id":    guildID,
			"server_name":  serverName,
			"message_id":   message.ID,
			"content":      req.Content,
			"timestamp":    time.Now().Format(time.RFC3339),
		}

		// Merge metadata into eventData (specifically look for response_model and response_raw)
		if req.Metadata != nil {
			if val, ok := req.Metadata["response_model"]; ok {
				eventData["response_model"] = val
			}
			if val, ok := req.Metadata["response_raw"]; ok {
				eventData["response_raw"] = val
			}
		}

		eventJSON, _ := json.Marshal(eventData)
		request := map[string]interface{}{
			"service": "dex-discord-service",
			"event":   json.RawMessage(eventJSON),
		}

		body, _ := json.Marshal(request)
		resp, err := http.Post(eventServiceURL+"/events", "application/json", bytes.NewBuffer(body))
		if err != nil {
			log.Printf("Warning: Failed to emit event: %v", err)
			return
		}
		defer func() {
			if err := resp.Body.Close(); err != nil {
				log.Printf("Error closing event service response body: %v", err)
			}
		}()
	}()

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	response := map[string]interface{}{
		"success":    true,
		"message_id": message.ID,
		"channel_id": req.ChannelID,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("POST ERROR: Failed to encode response: %v", err)
	}
}
