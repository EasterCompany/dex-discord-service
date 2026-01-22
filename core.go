package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
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
var ttsServiceURL string
var sttServiceURL string
var defaultVoiceChannelID string
var serverID string
var redisClient *redis.Client
var roleConfig config.DiscordRoleConfig
var voiceRecorder *audio.VoiceRecorder
var activeVoiceConnection *discordgo.VoiceConnection
var voiceConnectionMutex sync.Mutex

// RunCoreLogic manages the Discord session and its event handlers.
func RunCoreLogic(ctx context.Context, token, serviceURL, ttsURL, sttURL, defaultChannel, guildID string, roles config.DiscordRoleConfig, rc *redis.Client, port int) error {
	eventServiceURL = serviceURL
	ttsServiceURL = ttsURL
	sttServiceURL = sttURL
	endpoints.SetEventServiceURL(serviceURL)
	utils.SetEventServiceURL(serviceURL)
	defaultVoiceChannelID = defaultChannel
	serverID = guildID
	roleConfig = roles
	redisClient = rc

	var dg *discordgo.Session // Declare dg early so callbacks capture it
	var err error
	voiceRecorder, err = audio.NewVoiceRecorder(ctx,
		// OnStart callback
		func(userID, channelID string) {
			// Barge-In: Stop Dexter speaking if a user starts talking
			// We assume self-echo is handled by OpusRecv filtering or SSRC logic, but we double check ID.
			if dg != nil && dg.State != nil && dg.State.User != nil && userID == dg.State.User.ID {
				return
			}

			log.Printf("VAD: User %s started speaking. Triggering Barge-In Interrupt.", userID)
			if mixer := audio.GetGlobalMixer(); mixer != nil {
				mixer.InterruptVoice()
			}
		},
		// OnStop callback
		func(userID, channelID, redisKey, filePath string) {
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

			if redisKey != "" || filePath != "" {
				go transcribeAudio(dg, userID, channelID, redisKey, filePath)
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
		// Explicitly disconnect from voice before closing the session
		voiceConnectionMutex.Lock()
		if activeVoiceConnection != nil {
			log.Println("Graceful Shutdown: Disconnecting from voice channel...")
			if err := activeVoiceConnection.Disconnect(); err != nil {
				log.Printf("Error disconnecting voice: %v", err)
			} else {
				// Wait briefly for the disconnect packet to be sent
				time.Sleep(500 * time.Millisecond)
			}
		}
		voiceConnectionMutex.Unlock()

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

		// Post startup debug info (IPs, Port) to debug channel
		go postStartupDebugInfo(dg, port)

		// Auto-resolve Role IDs if config is stale
		if serverID != "" {
			resolveRoleIDs(dg, serverID)
		}

		// Enforce roles for all members on boot
		if serverID != "" {
			log.Println("Verifying role permissions for all members...")
			members, err := dg.GuildMembers(serverID, "", 1000)
			if err != nil {
				log.Printf("Error fetching members for role verification: %v", err)
			} else {
				for _, m := range members {
					enforceRoles(dg, serverID, m.User.ID, m.Roles)
				}
				log.Printf("Verified roles for %d members.", len(members))
			}
		}

		// Wait for TTS Service
		if defaultVoiceChannelID != "" {
			waitForTTSService(ctx, ttsServiceURL)
		}

		// Join default channel if configured
		if defaultVoiceChannelID != "" && serverID != "" {
			log.Printf("Joining default voice channel...")
			vc, err := joinOrMoveToVoiceChannel(dg, serverID, defaultVoiceChannelID)
			if err != nil {
				log.Printf("Error joining default voice channel: %v", err)
			} else {
				endpoints.SetActiveVoiceConnection(vc)
				// Only play greeting if humans are present
				if hasHumansInChannel(dg, vc.GuildID, vc.ChannelID) {
					go playGreeting(dg, vc)
				} else {
					log.Printf("Bot joined voice channel alone. Skipping greeting.")
				}
			}
		}

		// Start Voice Watchdog
		go voiceWatchdog(dg)

		// Start Voice Lock Manager (Priority & Locking)
		go voiceLockManager(dg)

		<-ctx.Done()
		return nil
	}
}

// hasHumansInChannel checks if there are any non-bot users in the specified channel.
func hasHumansInChannel(s *discordgo.Session, guildID, channelID string) bool {
	guild, err := s.State.Guild(guildID)
	if err != nil {
		return false
	}

	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == channelID {
			// Ignore self
			if vs.UserID == s.State.User.ID {
				continue
			}
			return true
		}
	}
	return false
}

// voiceLockManager periodically ensures Dexter's voice state is correctly synchronized.
func voiceLockManager(s *discordgo.Session) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		evaluateVoiceState(s)
	}
}

// evaluateVoiceState checks if Dexter is alone in a voice channel and updates the cognitive lock and active process state accordingly.
func evaluateVoiceState(s *discordgo.Session) {
	lockKey := "system:cognitive_lock"
	voiceModeID := "voice-mode"

	voiceConnectionMutex.Lock()
	vc := activeVoiceConnection
	voiceConnectionMutex.Unlock()

	// 1. Not in voice? Release lock if we hold it.
	if vc == nil || vc.ChannelID == "" {
		val, _ := redisClient.Get(context.Background(), lockKey).Result()
		if val == voiceModeID {
			redisClient.Del(context.Background(), lockKey)
			utils.ClearProcess(context.Background(), redisClient, "voice-mode")
			log.Printf("Voice Lock released (Not in voice)")
		}
		return
	}

	// 2. In voice? Count humans.
	guild, err := s.State.Guild(vc.GuildID)
	if err != nil {
		return
	}

	humanCount := 0
	for _, vs := range guild.VoiceStates {
		if vs.ChannelID == vc.ChannelID {
			if vs.UserID == s.State.User.ID {
				continue
			}
			humanCount++
		}
	}

	// 3. Update State
	if humanCount > 0 {
		// Humans present: Acquire or refresh
		holder, _ := redisClient.Get(context.Background(), lockKey).Result()

		if holder == "" || holder == voiceModeID {
			if holder == "" {
				log.Printf("Voice Mode: Acquiring Cognitive Lock (%d humans detected)", humanCount)
			}
			// Set with 60s TTL (renewed frequently)
			redisClient.Set(context.Background(), lockKey, voiceModeID, 60*time.Second)
			utils.ReportProcess(context.Background(), redisClient, "voice-mode", fmt.Sprintf("Voice Active (%d users)", humanCount))
		}
	} else {
		// Alone in channel: Release if we hold it
		holder, _ := redisClient.Get(context.Background(), lockKey).Result()
		if holder == voiceModeID {
			redisClient.Del(context.Background(), lockKey)
			utils.ClearProcess(context.Background(), redisClient, "voice-mode")
			log.Printf("Voice Lock released (Alone in channel)")
		}
	}
}

// resolveRoleIDs attempts to fix invalid role IDs by matching names
func resolveRoleIDs(s *discordgo.Session, guildID string) {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		log.Printf("Failed to fetch guild roles for resolution: %v", err)
		return
	}

	// Helper to check if ID exists
	idExists := func(id string) bool {
		for _, r := range roles {
			if r.ID == id {
				return true
			}
		}
		return false
	}

	// Helper to find ID by name
	findID := func(names []string) string {
		for _, r := range roles {
			for _, n := range names {
				if strings.EqualFold(r.Name, n) {
					return r.ID
				}
			}
		}
		return ""
	}

	// Resolve User Role
	if roleConfig.User == "" || !idExists(roleConfig.User) {
		newID := findID([]string{"User", "Member"})
		if newID != "" {
			log.Printf("Configured User role ID missing or invalid. Auto-resolved to '%s' (ID: %s)", "User", newID)
			roleConfig.User = newID
		}
	}

	// Resolve Admin Role
	if roleConfig.Admin == "" || !idExists(roleConfig.Admin) {
		newID := findID([]string{"Admin", "Administrator"})
		if newID != "" {
			log.Printf("Configured Admin role ID missing or invalid. Auto-resolved to '%s' (ID: %s)", "Admin", newID)
			roleConfig.Admin = newID
		}
	}

	// Resolve Moderator Role
	if roleConfig.Moderator == "" || !idExists(roleConfig.Moderator) {
		newID := findID([]string{"Moderator", "Mod"})
		if newID != "" {
			log.Printf("Configured Moderator role ID missing or invalid. Auto-resolved to '%s' (ID: %s)", "Moderator", newID)
			roleConfig.Moderator = newID
		}
	}

	// Resolve Contributor Role
	if roleConfig.Contributor == "" || !idExists(roleConfig.Contributor) {
		newID := findID([]string{"Contributor"})
		if newID != "" {
			log.Printf("Configured Contributor role ID missing or invalid. Auto-resolved to '%s' (ID: %s)", "Contributor", newID)
			roleConfig.Contributor = newID
		}
	}
}

func waitForTTSService(ctx context.Context, url string) {
	if url == "" {
		return
	}
	log.Printf("Waiting for TTS service at %s...", url)
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timeout:
			log.Println("Timeout waiting for TTS service. Proceeding without it.")
			return
		case <-ticker.C:
			// Check health
			resp, err := http.Get(url)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == 200 || resp.StatusCode == 404 { // 404 means service is up but path not found, which is fine for connectivity check
					log.Println("TTS service is online.")
					return
				}
			}
		}
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
				log.Printf("Voice Watchdog: Connection detected as not ready. Initiating Hard Reset...")

				guildID := vc.GuildID
				channelID := vc.ChannelID

				if guildID != "" && channelID != "" {
					// 1. Explicitly Disconnect to clear session state on Discord's end
					log.Printf("Voice Watchdog: Disconnecting from %s...", channelID)
					_ = vc.Disconnect()

					// Wait for state to clear
					time.Sleep(1 * time.Second)

					// 2. Re-join
					log.Printf("Voice Watchdog: Re-joining %s...", channelID)
					newVC, err := s.ChannelVoiceJoin(guildID, channelID, false, false)
					if err != nil {
						log.Printf("Voice Watchdog: Re-join failed: %v", err)
					} else {
						log.Printf("Voice Watchdog: Re-join successful.")

						// 3. Update Global State
						voiceConnectionMutex.Lock()
						activeVoiceConnection = newVC
						voiceConnectionMutex.Unlock()

						endpoints.SetActiveVoiceConnection(newVC)

						// Re-attach listeners for VAD
						setupVoiceReceivers(s, newVC)

						if voiceRecorder != nil {
							voiceRecorder.SetCurrentChannel(channelID)
						}
					}
				}
			}
		}
	}
}

func playGreeting(s *discordgo.Session, vc *discordgo.VoiceConnection) {
	// Simple TTS greeting
	text := "Dexter online. Systems functional."

	// Create temp file path
	tmpDir := "/tmp/dexter/audio"
	_ = os.MkdirAll(tmpDir, 0777)
	filename := fmt.Sprintf("greeting-%d.wav", time.Now().UnixNano())
	filePath := filepath.Join(tmpDir, filename)

	ttsURL := ttsServiceURL + "/generate"
	reqBody := map[string]string{
		"text":        text,
		"output_path": filePath,
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

	// Check if we got a file path back
	var respData map[string]string
	useFile := false

	// Peek at body to see if it is JSON
	bodyBytes, _ := io.ReadAll(resp.Body)
	if json.Unmarshal(bodyBytes, &respData) == nil && respData["file_path"] != "" {
		useFile = true
	}

	var fakeReq *http.Request
	if useFile {
		fakeReq, _ = http.NewRequest("POST", "/audio/play", nil)
		fakeReq.Header.Set("X-File-Path", filePath)
	} else {
		// Fallback: Use bytes (re-wrap bodyBytes)
		fakeReq, _ = http.NewRequest("POST", "/audio/play", bytes.NewBuffer(bodyBytes))
	}

	fakeW := &fakeResponseWriter{}
	endpoints.PlayAudioHandler(fakeW, fakeReq)

	// Emit event
	event := utils.GenericMessagingEvent{
		Type:        utils.EventTypeMessagingBotVoiceResponse,
		Source:      "discord",
		UserID:      s.State.User.ID,
		UserName:    s.State.User.Username,
		UserLevel:   string(utils.GetUserLevel(s, redisClient, vc.GuildID, s.State.User.ID, roleConfig)),
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
			UserLevel:   string(utils.GetUserLevel(s, redisClient, vc.GuildID, s.State.User.ID, roleConfig)),
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
		// ECHO CANCELLATION: Do not register our own SSRC.
		// If we register it, the recorder will capture our own output, causing feedback loops.
		if vs.UserID == s.State.User.ID {
			return
		}
		voiceRecorder.RegisterSSRC(uint32(vs.SSRC), vs.UserID, vc.ChannelID)
	})

	go func() {
		pktCount := 0
		for p := range vc.OpusRecv {
			if p != nil {
				pktCount++
				if pktCount%250 == 0 { // Log every ~5 seconds of audio (50 packets/sec)
					log.Printf("DEBUG: Rx Voice Packet SSRC %d (Count %d)", p.SSRC, pktCount)
				}
				if err := voiceRecorder.ProcessVoicePacket(p.SSRC, p); err != nil {
					log.Printf("Error processing voice packet: %v", err)
				}
			}
		}
		log.Printf("DEBUG: OpusRecv channel closed for %s", vc.ChannelID)
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
			UserLevel:   string(utils.GetUserLevel(s, redisClient, m.GuildID, m.Author.ID, roleConfig)),
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

	// Also check for role mentions
	if !event.MentionedBot && len(m.MentionRoles) > 0 && m.GuildID != "" {
		member, err := s.GuildMember(m.GuildID, s.State.User.ID)
		if err == nil {
			for _, roleID := range m.MentionRoles {
				for _, memberRole := range member.Roles {
					if roleID == memberRole {
						event.MentionedBot = true
						break
					}
				}
				if event.MentionedBot {
					break
				}
			}
		} else {
			log.Printf("Failed to get bot member for role mention check: %v", err)
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
				UserLevel:   string(utils.GetUserLevel(s, redisClient, v.GuildID, v.UserID, roleConfig)),
				ChannelID:   channelID,
				ChannelName: channel.Name,
				ServerID:    v.GuildID,
				Timestamp:   time.Now(),
			},
		}
		if err := sendEventData(event); err != nil {
			log.Printf("Error sending voice state change event: %v", err)
		}

		// Immediate re-evaluation of voice lock when someone joins or leaves
		evaluateVoiceState(s)
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
			UserLevel:  string(utils.GetUserLevel(s, redisClient, m.GuildID, m.User.ID, roleConfig)),
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

func transcribeAudio(s *discordgo.Session, userID, channelID, redisKey, filePath string) {
	// Call STT Service using multipart form
	// Priority: filePath > redisKey
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	if filePath != "" {
		_ = writer.WriteField("file_path", filePath)
	} else if redisKey != "" {
		_ = writer.WriteField("redis_key", redisKey)
	}

	_ = writer.Close()

	resp, err := http.Post(sttServiceURL+"/transcribe", writer.FormDataContentType(), body)
	if err != nil {
		log.Printf("Failed to call STT service: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		log.Printf("STT service error (status %d): %s", resp.StatusCode, string(bodyBytes))
		return
	}

	// Structure to parse STT service JSON output
	var sttOutput struct {
		Text        string  `json:"text"`
		Language    string  `json:"language"`
		Probability float64 `json:"probability"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&sttOutput); err != nil {
		log.Printf("Failed to parse STT response: %v", err)
		return
	}

	transcription := sttOutput.Text

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
			UserLevel:   string(utils.GetUserLevel(s, redisClient, channel.GuildID, userID, roleConfig)),
			ChannelID:   channelID,
			ChannelName: channel.Name,
			ServerID:    channel.GuildID,
			Timestamp:   time.Now(),
		},
		Transcription: transcription,
		Content:       transcription,
	}
	if err := sendEventData(event); err != nil {
		log.Printf("Error sending transcription event: %v", err)
	}
}

func guildMemberUpdate(s *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	enforceRoles(s, m.GuildID, m.User.ID, m.Roles)
}

func enforceRoles(s *discordgo.Session, guildID, userID string, currentRoles []string) {
	// --- 1. System Permission Roles Enforcement ---
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

	// Determine target system role
	targetSystemRoleID := highestRoleID
	if !hasAnySystemRole {
		// Default to User if configured
		if roleConfig.User != "" {
			targetSystemRoleID = roleConfig.User
		}
	}

	// --- 2. CS2 Color Roles Enforcement ---
	// Define CS2 color names
	cs2Colors := []string{"Blue", "Orange", "Purple", "Yellow", "Green"}
	colorRoleIDs := make(map[string]string)
	var userColorRoles []string

	// Try to fetch color roles from Redis cache
	cacheKey := fmt.Sprintf("discord:roles:colors:%s", guildID)
	cachedColors, err := redisClient.HGetAll(context.Background(), cacheKey).Result()

	if err == nil && len(cachedColors) > 0 {
		// Use cached roles
		colorRoleIDs = cachedColors
		// We still need to check if the user HAS these roles using currentRoles
		for _, colorID := range colorRoleIDs {
			if currentRoleMap[colorID] {
				userColorRoles = append(userColorRoles, colorID)
			}
		}
	} else {
		// Cache miss or empty, fetch from Discord
		roles, err := s.GuildRoles(guildID)
		if err == nil {
			for _, r := range roles {
				for _, colorName := range cs2Colors {
					if strings.EqualFold(r.Name, colorName) {
						colorRoleIDs[colorName] = r.ID
						// Check if user has this color
						if currentRoleMap[r.ID] {
							userColorRoles = append(userColorRoles, r.ID)
						}
					}
				}
			}
			// Update cache
			if len(colorRoleIDs) > 0 {
				if err := redisClient.HMSet(context.Background(), cacheKey, colorRoleIDs).Err(); err == nil {
					redisClient.Expire(context.Background(), cacheKey, 24*time.Hour)
				}
			}
		}
	}

	targetColorRoleID := ""
	if len(userColorRoles) == 1 {
		// User has exactly one color, keep it
		targetColorRoleID = userColorRoles[0]
	} else if len(userColorRoles) > 1 {
		// User has multiple colors, keep the first one found (or random?)
		// Let's keep the first one to be stable
		targetColorRoleID = userColorRoles[0]
	} else {
		// User has NO color, pick a random one
		// We need to know available color IDs first
		var availableColors []string
		for _, id := range colorRoleIDs {
			availableColors = append(availableColors, id)
		}
		if len(availableColors) > 0 {
			// Simple random selection
			// Use time as seed for basic randomness
			idx := time.Now().UnixNano() % int64(len(availableColors))
			targetColorRoleID = availableColors[idx]
		}
	}

	// --- 3. Apply Changes ---
	var toAdd []string
	var toRemove []string

	// System Role Changes
	if targetSystemRoleID != "" {
		if !currentRoleMap[targetSystemRoleID] {
			toAdd = append(toAdd, targetSystemRoleID)
		}
		// Remove other system roles (Exclusive Badge Logic)
		for rID := range systemRoles {
			if rID != targetSystemRoleID && currentRoleMap[rID] {
				toRemove = append(toRemove, rID)
			}
		}
	}

	// Color Role Changes
	if targetColorRoleID != "" {
		if !currentRoleMap[targetColorRoleID] {
			toAdd = append(toAdd, targetColorRoleID)
		}
		// Remove other color roles (Exclusive Color Logic)
		for _, rID := range colorRoleIDs {
			if rID != targetColorRoleID && currentRoleMap[rID] {
				toRemove = append(toRemove, rID)
			}
		}
	}

	if len(toAdd) == 0 && len(toRemove) == 0 {
		return
	}

	log.Printf("Enforcing roles for user %s: Adding %v, Removing %v", userID, toAdd, toRemove)

	for _, rID := range toAdd {
		if err := s.GuildMemberRoleAdd(guildID, userID, rID); err != nil {
			if strings.Contains(err.Error(), "50013") {
				log.Printf("WARNING: Permission Denied (50013) adding role %s. Hint: Drag Dexter's role ABOVE this role in Discord Server Settings.", rID)
			} else {
				log.Printf("Failed to add role %s: %v", rID, err)
			}
		}
	}
	for _, rID := range toRemove {
		if err := s.GuildMemberRoleRemove(guildID, userID, rID); err != nil {
			if strings.Contains(err.Error(), "50013") {
				log.Printf("WARNING: Permission Denied (50013) removing role %s. Hint: Drag Dexter's role ABOVE this role in Discord Server Settings.", rID)
			} else {
				log.Printf("Failed to remove role %s: %v", rID, err)
			}
		}
	}
}

// postStartupDebugInfo Gathers and posts network connectivity information to the debug channel.
func postStartupDebugInfo(s *discordgo.Session, port int) {
	debugChannelID := "1423328325778149438"

	// Gather comprehensive status via CLI
	dexPath := os.ExpandEnv("$HOME/Dexter/bin/dex")
	cmd := exec.Command(dexPath, "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("Failed to run dex status: %v", err)
		return
	}

	var status struct {
		Vitals struct {
			CLI      string `json:"cli"`
			Discord  string `json:"discord"`
			Release  string `json:"release"`
			CPU      string `json:"cpu"`
			GPU      string `json:"gpu"`
			Memory   string `json:"memory"`
			Disk     string `json:"disk"`
			User     string `json:"user"`
			Hostname string `json:"hostname"`
		} `json:"vitals"`
		Network struct {
			LocalIP          string `json:"local_ip"`
			PublicIP         string `json:"public_ip"`
			TailscaleIP      string `json:"tailscale_ip"`
			TailscaleDevices []struct {
				Hostname string   `json:"hostname"`
				IPs      []string `json:"ips"`
				Online   bool     `json:"online"`
				OS       string   `json:"os"`
			} `json:"tailscale_devices"`
		} `json:"network"`
	}

	if err := json.Unmarshal(output, &status); err != nil {
		log.Printf("Failed to parse dex status JSON: %v", err)
		return
	}

	// Helper to create redirect link
	makeLink := func(host string, target string) string {
		return fmt.Sprintf("http://%s:8201/open?target=%s", host, url.QueryEscape(target))
	}

	username := strings.Split(status.Vitals.User, " ")[0]
	localIP := status.Network.LocalIP
	tailscaleIP := status.Network.TailscaleIP
	publicIP := status.Network.PublicIP

	// 6. Construct and Post Message
	message := fmt.Sprintf("🌐 **Dexter Discord Service Started**\n\n"+
		"**System Vitals:**\n"+
		"• **Discord:** `%s`\n"+
		"• **Release:** `%s`\n"+
		"• **CPU:** `%s`\n"+
		"• **RAM:** `%s`\n"+
		"• **Disk:** `%s`\n"+
		"• **GPU:** `%s`\n"+
		"• **User:** `%s`\n"+
		"• **Host:** `%s`\n\n"+
		"**Network:**\n"+
		"• **Local:** `%s`\n"+
		"• **Tailscale:** `%s`\n"+
		"• **Public:** `%s`\n\n"+
		"**Frontend Access:**\n"+
		"🏠 [Local](http://%s:8000)\n"+
		"🔗 [Tailscale (Remote)](http://%s:8000)\n"+
		"🌎 [Production](https://easter.company)\n\n"+
		"**SSH Access:**\n"+
		"💻 [`ssh %s@%s`](%s) (Local)\n"+
		"🌍 [`ssh %s@%s`](%s) (Tailscale)\n"+
		"📱 [`mosh %s@%s`](%s) (Mobile)",
		status.Vitals.Discord, status.Vitals.Release, status.Vitals.CPU, status.Vitals.Memory, status.Vitals.Disk, status.Vitals.GPU, status.Vitals.User, status.Vitals.Hostname,
		localIP, tailscaleIP, publicIP,
		localIP, tailscaleIP,
		username, localIP, makeLink(localIP, "ssh://"+username+"@"+localIP),
		username, tailscaleIP, makeLink(tailscaleIP, "ssh://"+username+"@"+tailscaleIP),
		username, tailscaleIP, makeLink(tailscaleIP, "mosh://"+username+"@"+tailscaleIP))

	// Add Tailscale Devices as a Markdown Table

	if len(status.Network.TailscaleDevices) > 0 {

		tableHeader := "\n\n**Tailscale Network:**\n```\nDEVICE          IP              OS     STATUS\n--------------  --------------  -----  -------\n"

		tableBody := ""

		for _, p := range status.Network.TailscaleDevices {
			statusStr := "OFFLINE"
			if p.Online {
				statusStr = "ONLINE"
			}
			if p.Hostname == status.Vitals.Hostname {
				statusStr = "ONLINE*"
			}
			ip := "N/A"
			if len(p.IPs) > 0 {
				ip = p.IPs[0]
			}

			// Format row with fixed widths
			tableBody += fmt.Sprintf("% -14s  % -14s  % -5s  %s\n",
				uiTruncate(p.Hostname, 14),
				uiTruncate(ip, 14),
				uiTruncate(p.OS, 5),
				statusStr)
		}
		message += tableHeader + tableBody + "```"
	}

	_, err = s.ChannelMessageSend(debugChannelID, message)
	if err != nil {
		log.Printf("Failed to post startup debug info: %v", err)
	}
}

func uiTruncate(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	if maxLen <= 3 {
		return text[:maxLen]
	}
	return text[:maxLen-3] + "..."
}
