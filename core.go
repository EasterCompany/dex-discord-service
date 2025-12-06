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
	"github.com/EasterCompany/dex-discord-service/endpoints"
	"github.com/EasterCompany/dex-discord-service/utils"
	"github.com/bwmarrin/discordgo"
)

var eventServiceURL string
var masterUserID string
var defaultVoiceChannelID string
var serverID string
var voiceRecorder *audio.VoiceRecorder
var activeVoiceConnection *discordgo.VoiceConnection
var voiceConnectionMutex sync.Mutex

// RunCoreLogic manages the Discord session and its event handlers.
func RunCoreLogic(ctx context.Context, token, serviceURL, masterUser, defaultChannel, guildID string) error {
	eventServiceURL = serviceURL
	endpoints.SetEventServiceURL(serviceURL)
	masterUserID = masterUser
	defaultVoiceChannelID = defaultChannel
	serverID = guildID

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

		<-ctx.Done()
		return nil
	}
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	log.Printf("Logged in as %s#%s", s.State.User.Username, s.State.User.Discriminator)
	if err := s.UpdateGameStatus(0, "Listening for events..."); err != nil {
		log.Printf("Error updating game status: %v", err)
	}
	if defaultVoiceChannelID != "" && serverID != "" {
		log.Printf("Joining default voice channel...")
		if _, err := joinOrMoveToVoiceChannel(s, serverID, defaultVoiceChannelID); err != nil {
			log.Printf("Error joining default voice channel: %v", err)
		}
	}
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
	vc, err := s.ChannelVoiceJoin(guildID, channelID, true, false)
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

	log.Printf("Successfully joined/moved to voice channel %s", channelID)
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

	// Pre-process content to replace mentions with usernames
	content := m.Content
	for _, user := range m.Mentions {
		// Replace <@USER_ID> and <@!USER_ID> with @username
		content = strings.ReplaceAll(content, fmt.Sprintf("<@%s>", user.ID), fmt.Sprintf("@%s", user.Username))
		content = strings.ReplaceAll(content, fmt.Sprintf("<@!%s>", user.ID), fmt.Sprintf("@%s", user.Username))
	}

	var attachments []utils.Attachment
	for _, a := range m.Attachments {
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

	event := utils.UserSentMessageEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:        utils.EventTypeMessagingUserSentMessage,
			Source:      "discord",
			UserID:      m.Author.ID,
			UserName:    m.Author.Username,
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
	user, err := s.User(v.UserID)
	if err != nil {
		return
	}

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

	if eventType != "" {
		channel, _ := s.Channel(channelID)
		event := utils.UserVoiceStateChangeEvent{
			GenericMessagingEvent: utils.GenericMessagingEvent{
				Type:        eventType,
				Source:      "discord",
				UserID:      v.UserID,
				UserName:    user.Username,
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
	guild, _ := s.Guild(m.GuildID)
	event := utils.UserServerEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:       utils.EventTypeMessagingUserJoinedServer,
			Source:     "discord",
			UserID:     m.User.ID,
			UserName:   m.User.Username,
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
		DetectedLanguage      string `json:"detected_language"`
		EnglishTranslation    string `json:"english_translation"`
		Error                 string `json:"error"`
	}

	transcription := ""
	detectedLang := "en"
	englishTranslation := ""

	// Attempt to parse JSON output
	if err := json.Unmarshal(outputBytes, &dexOutput); err == nil {
		if dexOutput.Error != "" {
			log.Printf("Error from dex-cli: %s", dexOutput.Error)
			return
		}
		transcription = dexOutput.OriginalTranscription
		detectedLang = dexOutput.DetectedLanguage
		englishTranslation = dexOutput.EnglishTranslation
	} else {
		// Fallback for non-JSON output (or legacy behavior)
		transcription = strings.TrimSpace(string(outputBytes))
	}

	user, _ := s.User(userID)
	channel, _ := s.Channel(channelID)

	log.Printf("user %s in channel %s (lang: %s) said: %s", user.Username, channel.Name, detectedLang, transcription)
	if englishTranslation != "" {
		log.Printf("Translation: %s", englishTranslation)
	}

	event := utils.UserTranscribedEvent{
		GenericMessagingEvent: utils.GenericMessagingEvent{
			Type:        utils.EventTypeMessagingUserTranscribed,
			Source:      "discord",
			UserID:      userID,
			UserName:    user.Username,
			ChannelID:   channelID,
			ChannelName: channel.Name,
			ServerID:    channel.GuildID,
			Timestamp:   time.Now(),
		},
		Transcription:      transcription,
		DetectedLanguage:   detectedLang,
		EnglishTranslation: englishTranslation,
	}
	if err := sendEventData(event); err != nil {
		log.Printf("Error sending transcription event: %v", err)
	}
}
