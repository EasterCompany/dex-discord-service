package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/EasterCompany/dex-discord-service/endpoints"
	"github.com/EasterCompany/dex-discord-service/utils"
	"github.com/bwmarrin/discordgo"
)

var eventServiceURL string

// RunCoreLogic represents the persistent core functionality of the service.
// It connects to Discord and manages the session with automatic reconnection.
func RunCoreLogic(ctx context.Context, token, serviceURL string) error {
	eventServiceURL = serviceURL

	// Initialize Discord session
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("FATAL: Error creating Discord session: %v", err)
		utils.SetHealthStatus("ERROR", "Failed to create Discord session")
		return err
	}
	defer func() {
		if err := dg.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}()

	// Add handlers for Discord events
	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)
	dg.AddHandler(voiceStateUpdate)
	dg.AddHandler(guildMemberAdd)

	// Add disconnect handler for reconnection tracking
	dg.AddHandler(func(s *discordgo.Session, d *discordgo.Disconnect) {
		log.Printf("Discord disconnected, will attempt to reconnect...")
		utils.IncrementReconnects()
		utils.SetHealthStatus("RECONNECTING", "Discord connection lost, reconnecting...")
	})

	// Add resume handler
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		log.Printf("Discord connection resumed")
		utils.SetHealthStatus("OK", "Service is running and connected to Discord")
	})

	// Reconnection loop
	reconnectDelay := 5 * time.Second
	maxReconnectDelay := 5 * time.Minute

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			log.Println("Core Logic: Shutdown signal received, closing Discord connection...")
			utils.SetHealthStatus("SHUTTING_DOWN", "Service is shutting down")
			return nil
		default:
		}

		// Attempt to open connection
		if err := dg.Open(); err != nil {
			log.Printf("Error opening Discord connection: %v", err)
			utils.SetHealthStatus("ERROR", fmt.Sprintf("Failed to connect to Discord: %v", err))
			utils.IncrementReconnects()

			// Wait before retrying
			log.Printf("Retrying connection in %v...", reconnectDelay)
			select {
			case <-time.After(reconnectDelay):
				// Exponential backoff with max cap
				reconnectDelay *= 2
				if reconnectDelay > maxReconnectDelay {
					reconnectDelay = maxReconnectDelay
				}
				continue
			case <-ctx.Done():
				log.Println("Core Logic: Shutdown signal received during reconnection")
				utils.SetHealthStatus("SHUTTING_DOWN", "Service is shutting down")
				return nil
			}
		}

		// Announce connection to the event service
		log.Println("Core Logic: Announcing connection to event service...")
		if err := sendEvent("connected", "Discord service successfully connected and is online."); err != nil {
			log.Printf("Warning: Failed to announce connection to event service: %v", err)
			// Non-fatal, so we continue
		}

		// Mark service as healthy
		utils.SetHealthStatus("OK", "Service is running and connected to Discord")
		log.Println("Core Logic: Discord connection established, service is healthy.")

		// Set the Discord session for the post endpoint
		endpoints.SetDiscordSession(dg)

		// Wait for context cancellation or session close
		<-ctx.Done()
		log.Println("Core Logic: Shutdown signal received, closing Discord connection...")
		utils.SetHealthStatus("SHUTTING_DOWN", "Service is shutting down")
		return nil
	}
}

// ready is called when the bot is ready to start interacting with Discord.
func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as %s#%s", s.State.User.Username, s.State.User.Discriminator)
	if err := s.UpdateGameStatus(0, "Listening for events..."); err != nil {
		log.Printf("Error updating game status: %v", err)
	}
}

// messageCreate is called every time a new message is created on any channel.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	utils.IncrementMessagesReceived()
	channel, err := s.Channel(m.ChannelID)
	if err != nil {
		log.Printf("Error getting channel: %v", err)
		return
	}
	message := fmt.Sprintf("%s user posted in %s channel: %s", m.Author.Username, channel.Name, m.Content)
	if err := sendEvent("message_posted", message); err != nil {
		log.Printf("Error sending event: %v", err)
	}
}

// voiceStateUpdate is called every time a user joins or leaves a voice channel.
func voiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	user, err := s.User(v.UserID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		return
	}
	var message string
	if v.ChannelID != "" {
		channel, err := s.Channel(v.ChannelID)
		if err != nil {
			log.Printf("Error getting channel: %v", err)
			return
		}
		message = fmt.Sprintf("%s user joined %s voice channel", user.Username, channel.Name)
	} else {
		message = fmt.Sprintf("%s user disconnected from voice", user.Username)
	}

	if err := sendEvent("voice_state_update", message); err != nil {
		log.Printf("Error sending event: %v", err)
	}
}

// guildMemberAdd is called every time a new user joins a guild.
func guildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	guild, err := s.Guild(m.GuildID)
	if err != nil {
		log.Printf("Error getting guild: %v", err)
		return
	}
	message := fmt.Sprintf("%s user has joined the %s server", m.User.Username, guild.Name)
	if err := sendEvent("user_joined_server", message); err != nil {
		log.Printf("Error sending event: %v", err)
	}
}

// sendEvent sends an event to the event service with retry logic.
func sendEvent(eventType, message string) error {
	// Create the event data that will be stored
	eventData := map[string]interface{}{
		"type":    eventType,
		"message": message,
	}

	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event data: %w", err)
	}

	// Wrap in the CreateEventRequest structure
	request := map[string]interface{}{
		"service": "dex-discord-service",
		"event":   json.RawMessage(eventJSON),
	}

	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Retry logic with exponential backoff
	maxRetries := 3
	retryDelay := 1 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, err := http.Post(eventServiceURL+"/events", "application/json", bytes.NewBuffer(body))
		if err != nil {
			if attempt < maxRetries {
				log.Printf("Failed to send event (attempt %d/%d): %v. Retrying in %v...", attempt+1, maxRetries+1, err, retryDelay)
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			}
			return fmt.Errorf("failed to send event after %d attempts: %w", maxRetries+1, err)
		}
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Printf("Error closing response body: %v", cerr)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			if attempt < maxRetries {
				log.Printf("Event service returned non-OK status (attempt %d/%d): %s. Retrying in %v...", attempt+1, maxRetries+1, resp.Status, retryDelay)
				time.Sleep(retryDelay)
				retryDelay *= 2 // Exponential backoff
				continue
			}
			return fmt.Errorf("event service returned non-OK status after %d attempts: %s", maxRetries+1, resp.Status)
		}

		// Success
		utils.IncrementEventsSent()
		return nil
	}

	return fmt.Errorf("failed to send event after %d attempts", maxRetries+1)
}
