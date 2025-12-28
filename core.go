package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-service/audio"
	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/EasterCompany/dex-discord-service/endpoints"
	"github.com/EasterCompany/dex-discord-service/utils"
	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

const MaxAttachmentSize = 10 * 1024 * 1024 // 10 MiB

var eventServiceURL string
var masterUserID string
var defaultVoiceChannelID string
var serverID string
var redisClient *redis.Client
var roleConfig config.DiscordRoleConfig
var voiceRecorder *audio.VoiceRecorder
var activeVoiceConnection *discordgo.VoiceConnection
var voiceConnectionMutex sync.Mutex

// RunCoreLogic manages the Discord session and its event handlers.
func RunCoreLogic(ctx context.Context, token, serviceURL, masterUser, defaultChannel, guildID string, roles config.DiscordRoleConfig, rc *redis.Client) error {
	eventServiceURL = serviceURL
	endpoints.SetEventServiceURL(serviceURL)
	masterUserID = masterUser
	defaultVoiceChannelID = defaultChannel
	serverID = guildID
	roleConfig = roles
	redisClient = rc

	var dg *discordgo.Session // Declare dg early so callbacks capture it
	var err error
	voiceRecorder, err = audio.NewVoiceRecorder(ctx,
		// OnStart callback
		func(userID, channelID string) {
			// user, _ := dg.User(userID)
			// channel, _ := dg.Channel(channelID)

			log.Printf("VAD: User %s started speaking.", userID)

			/*
				event := utils.UserSpeakingEvent{
					GenericMessagingEvent: utils.GenericMessagingEvent{
						Source:      "discord",
						UserID:      userID,
						UserName:    user.Username,
						ChannelID:   channelID,
						ChannelName: channel.Name,
						ServerID:    guildID,
						Timestamp:   time.Now(),
						Type:        utils.EventTypeMessagingUserSpeakingStarted,
					},
				}
				if err := sendEventData(event); err != nil {
					log.Printf("Error sending speaking started event: %v", err)
				}
			*/
		},
		// OnStop callback
		func(userID, channelID, redisKey string) {
			// user, _ := dg.User(userID)
			// channel, _ := dg.Channel(channelID)

			log.Printf("VAD: User %s stopped speaking.", userID)

			/*
				event := utils.UserSpeakingEvent{
					GenericMessagingEvent: utils.GenericMessagingEvent{
						Source:      "discord",
						UserID:      userID,
						UserName:    user.Username,
						ChannelID:   channelID,
						ChannelName: channel.Name,
						ServerID:    guildID,
						Timestamp:   time.Now(),
						Type:        utils.EventTypeMessagingUserSpeakingStopped,
					},
				}
				if err := sendEventData(event); err != nil {
					log.Printf("Error sending speaking stopped event: %v", err)
				}
			*/

			if redisKey != "" {
				go transcribeAudio(dg, userID, channelID, redisKey)
			}
		},
	)
	if err != nil {
		log.Printf("FATAL: Error creating voice recorder: %v", err)
		utils.SetHealthStatus("ERROR", "Failed to create voice recorder")
		return err
	}

	dg, err = discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("FATAL: Error creating Discord session: %v", err)
		return err
	}
	defer func() {
		if err := dg.Close(); err != nil {
			log.Printf("Error closing Discord session: %v", err)
		}
	}()

	dg.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuildMembers | discordgo.IntentsDirectMessages | discordgo.IntentsGuildPresences
	dg.ShouldReconnectOnError = true

	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)
	dg.AddHandler(voiceStateUpdate)
	dg.AddHandler(guildMemberAdd)
	dg.AddHandler(guildMemberUpdate)
	dg.AddHandler(func(s *discordgo.Session, d *discordgo.Disconnect) {
		log.Printf("Discord disconnected, will attempt to reconnect...")
		utils.IncrementReconnects()
		utils.SetHealthStatus("RECONNECTING", "Discord connection lost, reconnecting...")
	})
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Resumed) {
		log.Printf("Discord connection resumed")
		utils.SetHealthStatus("OK", "Service is running and connected to Discord")
	})

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		if err := dg.Open(); err != nil {
			log.Printf("Error opening Discord connection: %v", err)
			time.Sleep(15 * time.Second) // Wait before retrying
			continue
		}

		log.Println("Core Logic: Announcing connection to event service...")
		utils.ReportProcess(ctx, redisClient, "system-discord", "connected")
		connectionEvent := utils.BotStatusUpdateEvent{
			Type:      utils.EventTypeMessagingBotStatusUpdate,
			Source:    "discord",
			Status:    "connected",
			Details:   "Discord service successfully connected and is online.",
			Timestamp: time.Now(),
		}
		if err := sendEventData(connectionEvent); err != nil {
			log.Printf("Warning: Failed to announce connection: %v", err)
		}

		utils.SetHealthStatus("OK", "Service is running and connected to Discord")
		endpoints.SetDiscordSession(dg)

		// Join default channel if configured
		if defaultVoiceChannelID != "" && serverID != "" {
			log.Printf("Joining default voice channel...")
			vc, err := joinOrMoveToVoiceChannel(dg, serverID, defaultVoiceChannelID)
			if err != nil {
				log.Printf("Error joining default voice channel: %v", err)
			} else {
				endpoints.SetActiveVoiceConnection(vc)
				go playGreeting(dg, vc)
			}
		}

		// Start Voice Watchdog
		go voiceWatchdog(dg)

		<-ctx.Done()
		log.Println("Core Logic: Shutting down, clearing process info...")
		utils.ClearProcess(context.Background(), redisClient, "system-discord")
		return nil
	}
}

func voiceWatchdog(s *discordgo.Session) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		voiceConnectionMutex.Lock()
		vc := activeVoiceConnection
		voiceConnectionMutex.Unlock()

		if vc != nil {
			if !vc.Ready {
				log.Printf("Voice Watchdog: Connection detected as not ready. Attempting to recover...")

				// Attempt to re-join the current channel
				// We use the last known state from the session if possible, or the VC's data
				guildID := vc.GuildID
				channelID := vc.ChannelID

				if guildID != "" && channelID != "" {
					// Force a reconnect by joining again
					// We don't use joinOrMoveToVoiceChannel to avoid the mutex lock complexity here,
					// we just call discordgo directly which triggers the state update handler eventually.
					// Actually, calling ChannelVoiceJoin might be safest.
					// But we need to avoid deadlock if joinOrMoveToVoiceChannel locks mutex.
					// voiceWatchdog already unlocked.

					// Just trigger a rejoin
					log.Printf("Voice Watchdog: Reconnecting to %s...", channelID)
					_, err := s.ChannelVoiceJoin(guildID, channelID, false, false)
					if err != nil {
						log.Printf("Voice Watchdog: Reconnection failed: %v", err)
					}
				}
			}
		}
	}
}

func playGreeting(s *discordgo.Session, vc *discordgo.VoiceConnection) {
	// Simple TTS greeting
	text := "Dexter online. Systems functional."

	// Create a pipe to stream the TTS audio
	// Since we are inside the discord service, we can call the TTS service directly via HTTP
	// or invoke PlayAudioHandler logic. But PlayAudioHandler expects a request body.

	// Better: Call the TTS service to generate audio, and then stream that response to PlayAudioHandler?
	// No, we can call PlayAudioHandler logic internally or duplicate it.
	// But simplest is to call our own endpoint? No, we are in the same process.

	// Let's re-use PlayAudioHandler logic but adapted, or just call TTS service and stream to endpoints.PlayAudioHandler via a fake request?
	// Fake request is easiest to reuse the complex ffmpeg/opus logic.

	ttsURL := "http://127.0.0.1:8200/generate"
	reqBody := map[string]string{
		"text": text,
	}
	jsonBody, _ := json.Marshal(reqBody)

	resp, err := http.Post(ttsURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Printf("Failed to generate greeting TTS: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("TTS service returned error: %d", resp.StatusCode)
		return
	}

	// Now play it. We can create a fake request to our own PlayAudioHandler.
	// Or we can extract the streaming logic to a helper function.
	// Let's create a fake request.

	fakeReq, _ := http.NewRequest("POST", "/audio/play", resp.Body)
	fakeW := &fakeResponseWriter{}

	endpoints.PlayAudioHandler(fakeW, fakeReq)

	// Emit event
	event := utils.GenericMessagingEvent{
		Type:        utils.EventTypeMessagingBotVoiceResponse,
		Source:      "discord",
		UserID:      s.State.User.ID,
		UserName:    s.State.User.Username,
		UserLevel:   string(utils.GetUserLevel(s, redisClient, vc.GuildID, s.State.User.ID, masterUserID, roleConfig)),
		ChannelID:   vc.ChannelID,
		ChannelName: "Voice", // Could fetch real name
		ServerID:    vc.GuildID,
		Timestamp:   time.Now(),
	}

	// Custom payload for VoiceResponse
	voiceEvent := struct {
		utils.GenericMessagingEvent
		Content string `json:"content"`
	}{
		GenericMessagingEvent: event,
		Content:               text,
	}

	if err := sendEventData(voiceEvent); err != nil {
		log.Printf("Error sending greeting event: %v", err)
	}
}

// fakeResponseWriter to satisfy http.ResponseWriter interface
type fakeResponseWriter struct{}

func (f *fakeResponseWriter) Header() http.Header         { return http.Header{} }
func (f *fakeResponseWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeResponseWriter) WriteHeader(statusCode int)  {}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as %s#%s", s.State.User.Username, s.State.User.Discriminator)
	if err := s.UpdateGameStatus(0, "Listening for events..."); err != nil {
		log.Printf("Error updating game status: %v", err)
	}

	// Trigger catch-up logic for missed messages
	go func() {
		// Wait a brief moment to ensure connection stability
		time.Sleep(5 * time.Second)
		utils.FetchMissedMessages(s, eventServiceURL, serverID)
	}()
}

func joinOrMoveToVoiceChannel(s *discordgo.Session, guildID, channelID string) (*discordgo.VoiceConnection, error) {
	voiceConnectionMutex.Lock()
	defer voiceConnectionMutex.Unlock()

	// If we are already in the target channel with an active connection, do nothing.
	if activeVoiceConnection != nil && activeVoiceConnection.ChannelID == channelID {
		log.Printf("Already in voice channel %s, reusing connection.", channelID)
		return activeVoiceConnection, nil
	}

	// If we are moving channels, stop recordings first.
	if activeVoiceConnection != nil {
		log.Printf("Moving voice connection from %s to %s", activeVoiceConnection.ChannelID, channelID)
		voiceRecorder.StopAllRecordings()
	}

	// Join the new channel. This will return a new or existing connection object.
	// Set selfMute to false so the bot can speak.
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, false)
	if err != nil {
		return nil, fmt.Errorf("failed to join voice channel: %w", err)
	}

	// If the returned connection is a brand new object, we must set up its handlers.
	// This covers the initial join and any reconnection scenarios where a new connection is made.
	if vc != activeVoiceConnection {
		log.Println("New voice connection object detected. Setting up voice receivers...")
		setupVoiceReceivers(s, vc)
	}

	// Update the global active connection and the recorder's current channel.
	activeVoiceConnection = vc
	voiceRecorder.SetCurrentChannel(channelID)

	// Wait for connection to stabilize before enabling mixer
	time.Sleep(1 * time.Second)
	endpoints.SetActiveVoiceConnection(vc) // Register with endpoints for playback

	// Emit event after the mixer is ready
	log.Printf("Bot joined voice channel: %s", vc.ChannelID)
	channel, _ := s.Channel(vc.ChannelID)
	channelName := "unknown"
	if channel != nil {
		channelName = channel.Name
	}
	event := utils.UserVoiceStateChangeEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:        utils.EventType("messaging.bot.joined_voice"),
			Source:      "discord",
			UserID:      s.State.User.ID,
			UserName:    s.State.User.Username,
			UserLevel:   string(utils.GetUserLevel(s, redisClient, vc.GuildID, s.State.User.ID, masterUserID, roleConfig)),
			ChannelID:   vc.ChannelID,
			ChannelName: channelName,
			ServerID:    vc.GuildID,
			Timestamp:   time.Now(),
		},
	}
	if err := sendEventData(event); err != nil {
		log.Printf("Error sending bot voice join event: %v", err)
	}

	return vc, nil
}

func setupVoiceReceivers(s *discordgo.Session, vc *discordgo.VoiceConnection) {
	vc.AddHandler(func(vc *discordgo.VoiceConnection, vs *discordgo.VoiceSpeakingUpdate) {
		voiceRecorder.RegisterSSRC(uint32(vs.SSRC), vs.UserID, vc.ChannelID)
	})

	go func() {
		for p := range vc.OpusRecv {
			if p != nil {
				if err := voiceRecorder.ProcessVoicePacket(p.SSRC, p); err != nil {
					log.Printf("Error processing voice packet: %v", err)
				}
			}
		}
	}()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	utils.IncrementMessagesReceived()

	channelName := "DM"
	serverID := ""
	if m.GuildID != "" {
		channel, err := s.Channel(m.ChannelID)
		if err == nil {
			channelName = channel.Name
		}
		serverID = m.GuildID
	}

	// Pre-process content to replace mentions with display names
	content := m.Content

	// If content is empty (common with webhooks/embeds), try to build it from embeds
	if content == "" && len(m.Embeds) > 0 {
		var parts []string
		for _, embed := range m.Embeds {
			if embed.Title != "" {
				parts = append(parts, embed.Title)
			}
			if embed.Description != "" {
				parts = append(parts, embed.Description)
			}
			for _, field := range embed.Fields {
				parts = append(parts, fmt.Sprintf("%s: %s", field.Name, field.Value))
			}
		}
		content = strings.Join(parts, "\n")
	}

	for _, user := range m.Mentions {
		displayName := utils.GetUserDisplayName(s, redisClient, m.GuildID, user.ID)
		// Replace <@USER_ID> and <@!USER_ID> with @DisplayName
		content = strings.ReplaceAll(content, fmt.Sprintf("<@%s>", user.ID), fmt.Sprintf("@%s", displayName))
		content = strings.ReplaceAll(content, fmt.Sprintf("<@!%s>", user.ID), fmt.Sprintf("@%s", displayName))
	}

	var attachments []utils.Attachment
	for _, a := range m.Attachments {
		if a.Size > MaxAttachmentSize {
			log.Printf("Attachment '%s' skipped: size %d exceeds limit %d", a.Filename, a.Size, MaxAttachmentSize)
			continue
		}
		attachments = append(attachments, utils.Attachment{
			ID:          a.ID,
			URL:         a.URL,
			ProxyURL:    a.ProxyURL,
			Filename:    a.Filename,
			ContentType: a.ContentType,
			Size:        a.Size,
			Height:      a.Height,
			Width:       a.Width,
		})
	}

	// Determine username (handle webhooks specifically)
	var userName string
	var eventType utils.EventType
	if m.WebhookID != "" {
		userName = m.Author.Username
		eventType = utils.EventTypeMessagingWebhookMessage
	} else {
		userName = utils.GetUserDisplayName(s, redisClient, m.GuildID, m.Author.ID)
		eventType = utils.EventTypeMessagingUserSentMessage
	}

	event := utils.UserSentMessageEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:        eventType,
			Source:      "discord",
			UserID:      m.Author.ID,
			UserName:    userName,
			UserLevel:   string(utils.GetUserLevel(s, redisClient, m.GuildID, m.Author.ID, masterUserID, roleConfig)),
			ChannelID:   m.ChannelID,
			ChannelName: channelName,
			ServerID:    serverID,
			Timestamp:   m.Timestamp,
		},
		MessageID:    m.ID,
		Content:      content,
		MentionedBot: false,
		Attachments:  attachments,
	}

	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			event.MentionedBot = true
			break
		}
	}

	if err := sendEventData(event); err != nil {
		log.Printf("Error sending message event: %v", err)
	}
}

func voiceStateUpdate(s *discordgo.Session, v *discordgo.VoiceStateUpdate) {
	// Detect if bot joined a voice channel
	/*
		// Temporarily disabled: This event is now emitted by joinOrMoveToVoiceChannel
		// to ensure the audio mixer is ready before the greeting handler is triggered.
		if v.UserID == s.State.User.ID {
			if v.ChannelID != "" && (v.BeforeUpdate == nil || v.BeforeUpdate.ChannelID != v.ChannelID) {
				log.Printf("Bot joined voice channel: %s", v.ChannelID)
				channel, _ := s.Channel(v.ChannelID)
				channelName := "unknown"
				if channel != nil {
					channelName = channel.Name
				}
				event := utils.UserVoiceStateChangeEvent{
					GenericMessagingEvent: utils.GenericMessagingEvent{
						Type:        utils.EventType("messaging.bot.joined_voice"),
						Source:      "discord",
						UserID:      v.UserID,
						UserName:    s.State.User.Username,
						ChannelID:   v.ChannelID,
						ChannelName: channelName,
						ServerID:    v.GuildID,
						Timestamp:   time.Now(),
					},
				}
				if err := sendEventData(event); err != nil {
					log.Printf("Error sending bot voice join event: %v", err)
				}
			}
		}
	*/

	if v.UserID == masterUserID {
		if v.ChannelID != "" {
			if _, err := joinOrMoveToVoiceChannel(s, v.GuildID, v.ChannelID); err != nil {
				log.Printf("Error following master user to voice channel: %v", err)
			}
		} else if defaultVoiceChannelID != "" && serverID != "" {
			if _, err := joinOrMoveToVoiceChannel(s, serverID, defaultVoiceChannelID); err != nil {
				log.Printf("Error returning bot to default voice channel: %v", err)
			}
		}
	}

	var eventType utils.EventType
	channelID := v.ChannelID
	if v.ChannelID != "" && (v.BeforeUpdate == nil || v.BeforeUpdate.ChannelID != v.ChannelID) {
		eventType = utils.EventTypeMessagingUserJoinedVoice
	} else if v.ChannelID == "" && v.BeforeUpdate != nil {
		eventType = utils.EventTypeMessagingUserLeftVoice
		channelID = v.BeforeUpdate.ChannelID
	}

	// Do not emit user join/left events for the bot itself
	if v.UserID == s.State.User.ID {
		eventType = ""
	}

	if eventType != "" {
		channel, _ := s.Channel(channelID)
		event := utils.UserVoiceStateChangeEvent{
			GenericMessagingEvent: utils.GenericMessagingEvent{
				Type:        eventType,
				Source:      "discord",
				UserID:      v.UserID,
				UserName:    utils.GetUserDisplayName(s, redisClient, v.GuildID, v.UserID),
				UserLevel:   string(utils.GetUserLevel(s, redisClient, v.GuildID, v.UserID, masterUserID, roleConfig)),
				ChannelID:   channelID,
				ChannelName: channel.Name,
				ServerID:    v.GuildID,
				Timestamp:   time.Now(),
			},
		}
		if err := sendEventData(event); err != nil {
			log.Printf("Error sending voice state change event: %v", err)
		}
	}
}

func guildMemberAdd(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
	enforceRoles(s, m.GuildID, m.User.ID, m.Roles)

	guild, _ := s.Guild(m.GuildID)
	event := utils.UserServerEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:       utils.EventTypeMessagingUserJoinedServer,
			Source:     "discord",
			UserID:     m.User.ID,
			UserName:   utils.GetUserDisplayName(s, redisClient, m.GuildID, m.User.ID),
			UserLevel:  string(utils.GetUserLevel(s, redisClient, m.GuildID, m.User.ID, masterUserID, roleConfig)),
			ServerID:   m.GuildID,
			ServerName: guild.Name,
			Timestamp:  m.JoinedAt,
		},
	}
	if err := sendEventData(event); err != nil {
		log.Printf("Error sending guild member add event: %v", err)
	}
}

func sendEventData(eventData interface{}) error {
	eventJSON, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	request := map[string]interface{}{"service": "dex-discord-service", "event": json.RawMessage(eventJSON)}
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	for i := 0; i < 3; i++ {
		resp, err := http.Post(eventServiceURL+"/events", "application/json", bytes.NewBuffer(body))
		if err == nil {
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Printf("Error closing response body: %v", err)
				}
			}()
			if resp.StatusCode < 300 {
				utils.IncrementEventsSent()
				return nil
			}

			// Read response body for error details
			respBody, _ := io.ReadAll(resp.Body)
			log.Printf("Failed to send event (Attempt %d/3): Status %d, Body: %s", i+1, resp.StatusCode, string(respBody))
		} else {
			log.Printf("Failed to send event (Attempt %d/3): %v", i+1, err)
		}
		time.Sleep(time.Duration(i+1) * 2 * time.Second)
	}
	return fmt.Errorf("failed to send event after multiple attempts")
}

func transcribeAudio(s *discordgo.Session, userID, channelID, redisKey string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Printf("Error getting user home directory: %v", err)
		return
	}
	dexPath := filepath.Join(homeDir, "Dexter", "bin", "dex")

	cmd := exec.Command(dexPath, "whisper", "transcribe", "-k", redisKey)
	outputBytes, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			log.Printf("Error transcribing audio: %v, stderr: %s", err, string(exitErr.Stderr))
		} else {
			log.Printf("Error transcribing audio: %v", err)
		}
		return
	}

	// Structure to parse dex-cli JSON output
	var dexOutput struct {
		OriginalTranscription string `json:"original_transcription"`
		Error                 string `json:"error"`
	}

	transcription := ""

	// Attempt to parse JSON output
	if err := json.Unmarshal(outputBytes, &dexOutput); err == nil {
		if dexOutput.Error != "" {
			log.Printf("Error from dex-cli: %s", dexOutput.Error)
			return
		}
		transcription = dexOutput.OriginalTranscription
	} else {
		// Fallback for non-JSON output (or legacy behavior)
		transcription = strings.TrimSpace(string(outputBytes))
	}

	// IGNORE empty or whitespace-only transcriptions
	if strings.TrimSpace(transcription) == "" {
		log.Printf("Ignoring empty transcription from user %s in channel %s.", userID, channelID)
		return
	}

	channel, _ := s.Channel(channelID)
	userName := utils.GetUserDisplayName(s, redisClient, channel.GuildID, userID)

	log.Printf("user %s in channel %s said: %s", userName, channel.Name, transcription)

	event := utils.UserTranscribedEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:        utils.EventTypeMessagingUserTranscribed,
			Source:      "discord",
			UserID:      userID,
			UserName:    userName,
			UserLevel:   string(utils.GetUserLevel(s, redisClient, channel.GuildID, userID, masterUserID, roleConfig)),
			ChannelID:   channelID,
			ChannelName: channel.Name,
			ServerID:    channel.GuildID,
			Timestamp:   time.Now(),
		},
		Transcription: transcription,
	}
	if err := sendEventData(event); err != nil {
		log.Printf("Error sending transcription event: %v", err)
	}
}

func guildMemberUpdate(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	enforceRoles(s, m.GuildID, m.User.ID, m.Roles)
}

func enforceRoles(s *discordgo.Session, guildID, userID string, currentRoles []string) {
	// Map system roles from config
	// Priority: 3=Admin, 2=Moderator, 1=Contributor, 0=User
	systemRoles := make(map[string]int)
	if roleConfig.Admin != "" {
		systemRoles[roleConfig.Admin] = 3
	}
	if roleConfig.Moderator != "" {
		systemRoles[roleConfig.Moderator] = 2
	}
	if roleConfig.Contributor != "" {
		systemRoles[roleConfig.Contributor] = 1
	}
	if roleConfig.User != "" {
		systemRoles[roleConfig.User] = 0
	}

	// Find highest system role user currently has
	highestRoleID := ""
	highestLevel := -1
	hasAnySystemRole := false

	// Create map for fast lookup of current roles
	currentRoleMap := make(map[string]bool)
	for _, r := range currentRoles {
		currentRoleMap[r] = true
		if level, ok := systemRoles[r]; ok {
			hasAnySystemRole = true
			if level > highestLevel {
				highestLevel = level
				highestRoleID = r
			}
		}
	}

	// Determine target role
	targetRoleID := highestRoleID
	if !hasAnySystemRole {
		// Default to User if configured
		if roleConfig.User != "" {
			targetRoleID = roleConfig.User
		} else {
			return // No user role configured, nothing to enforce
		}
	}

	// Calculate changes
	var toAdd []string
	var toRemove []string

	// 1. Add target if missing
	if !currentRoleMap[targetRoleID] && targetRoleID != "" {
		toAdd = append(toAdd, targetRoleID)
	}

	// 2. Remove other system roles
	for rID := range systemRoles {
		if rID != targetRoleID && currentRoleMap[rID] {
			toRemove = append(toRemove, rID)
		}
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}

	log.Printf("Enforcing roles for user %s: Adding %v, Removing %v", userID, toAdd, toRemove)

	for _, rID := range toAdd {
		if err := s.GuildMemberRoleAdd(guildID, userID, rID); err != nil {
			log.Printf("Failed to add role %s: %v", rID, err)
		}
	}
	for _, rID := range toRemove {
		if err := s.GuildMemberRoleRemove(guildID, userID, rID); err != nil {
			log.Printf("Failed to remove role %s: %v", rID, err)
		}
	}
}
