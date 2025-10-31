// Package events handles Discord gateway events and dispatches them to appropriate handlers.
package events

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

var rtpPacketPool = sync.Pool{
	New: func() any {
		return &rtp.Packet{
			Header: rtp.Header{
				Version:     2,
				PayloadType: 0x78,
			},
		}
	},
}

type VoiceHandler struct {
	Session      *discordgo.Session
	Logger       logger.Logger
	StateManager *StateManager
	DB           cache.Cache
	BotCfg       *config.BotConfig
	DiscordCfg   *config.DiscordConfig
	SttClient    interfaces.SpeechToText
}

func NewVoiceHandler(s *discordgo.Session, logger logger.Logger, stateManager *StateManager, db cache.Cache, botCfg *config.BotConfig, discordCfg *config.DiscordConfig, sttClient interfaces.SpeechToText) *VoiceHandler {
	return &VoiceHandler{
		Session:      s,
		Logger:       logger,
		StateManager: stateManager,
		DB:           db,
		BotCfg:       botCfg,
		DiscordCfg:   discordCfg,
		SttClient:    sttClient,
	}
}

func (h *VoiceHandler) JoinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		h.Logger.Error("Could not get guild from state", err)
		return
	}

	userVoiceState := ""
	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			userVoiceState = vs.ChannelID
			break
		}
	}

	if userVoiceState == "" {
		_, _ = s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel for me to join!")
		return
	}

	// Check if bot is already connected to a voice channel in this guild
	if vc, ok := s.VoiceConnections[m.GuildID]; ok {
		// If already in the correct channel, do nothing.
		if vc.ChannelID == userVoiceState {
			h.Logger.Info(fmt.Sprintf("Bot is already in the correct voice channel %s, doing nothing.", vc.ChannelID))
			_, _ = s.ChannelMessageSend(m.ChannelID, "I'm already in your voice channel!")
			return
		}

		// If in a different channel, disconnect first before moving.
		h.Logger.Info(fmt.Sprintf("Bot is moving from voice channel %s to %s.", vc.ChannelID, userVoiceState))
		h.disconnectFromVoice(s, m.GuildID)
	}

	// At this point, the bot is not in any voice channel in this guild, so we can join.
	state := h.StateManager.GetOrStoreGuildState(m.GuildID)
	if h.DB != nil {
		if err := h.DB.SaveGuildState(m.GuildID, state); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", m.GuildID), err)
		}
	}

	channel, err := s.Channel(userVoiceState)
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Could not get channel %s", userVoiceState), err)
		return
	}

	msg, _ := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Connecting to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
	if msg != nil {
		state.ConnectionMessageID = msg.ID
		state.ConnectionMessageChannelID = msg.ChannelID
		state.ConnectionStartTime = time.Now()
	}

	const maxRetries = 3
	var vc *discordgo.VoiceConnection

	for i := 0; i < maxRetries; i++ {
		vc, err = s.ChannelVoiceJoin(m.GuildID, userVoiceState, false, false)
		if err == nil {
			break
		}
		h.Logger.Error(fmt.Sprintf("Attempt %d to join voice channel failed", i+1), err)
		retrySeconds := int(math.Pow(2, float64(i)))
		if msg != nil {
			_, _ = s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect, retrying in %d seconds...", retrySeconds))
		}
		time.Sleep(time.Duration(retrySeconds) * time.Second)
	}

	if err != nil {
		if msg != nil {
			_, _ = s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
		}
		h.disconnectFromVoice(s, m.GuildID)
		return
	}

	vc.AddHandler(func(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
		h.SpeakingUpdate(vc, p)
	})
	state.ConnectionChannelID = vc.ChannelID
	go h.handleVoice(s, vc, state)
}

func (h *VoiceHandler) LeaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.disconnectFromVoice(s, m.GuildID)
}

func (h *VoiceHandler) disconnectFromVoice(s *discordgo.Session, guildID string) {
	if vc, ok := s.VoiceConnections[guildID]; ok {
		state, ok := h.StateManager.GetGuildState(guildID)
		if !ok {
			_ = vc.Disconnect()
			return
		}

		// Cancel the context to signal the handleVoice goroutine to exit
		if state.CancelFunc != nil {
			state.CancelFunc()
		}

		// Disconnect from voice
		_ = vc.Disconnect()

		// Poll until the voice connection is actually gone from s.VoiceConnections
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		timeout := time.After(5 * time.Second) // 5 second timeout

		for {
			select {
			case <-ticker.C:
				if _, stillConnected := s.VoiceConnections[guildID]; !stillConnected {
					h.Logger.Info(fmt.Sprintf("Successfully disconnected from voice channel in guild %s.", guildID))
					goto EndPolling // Exit the polling loop
				}
			case <-timeout:
				h.Logger.Error(fmt.Sprintf("Timeout waiting for voice channel disconnection in guild %s.", guildID), nil)
				goto EndPolling // Exit the polling loop due to timeout
			}
		}
	EndPolling:

		state.MetaMutex.Lock()
		defer state.MetaMutex.Unlock()

		if state.ConnectionMessageID != "" {
			duration := time.Since(state.ConnectionStartTime).Round(time.Second)
			channel, _ := s.Channel(state.ConnectionChannelID)
			g, _ := s.State.Guild(guildID)

			var editContent string
			if channel != nil && g != nil {
				// Count unique users who spoke
				uniqueUsers := make(map[string]bool)
				for _, userID := range state.SSRCUserMap {
					uniqueUsers[userID] = true
				}

				editContent = fmt.Sprintf("**Disconnected from %s (%s) at %s (%s)**\n**Duration:** %s\n**Users tracked:** %d\n**Total SSRCs:** %d",
					channel.Name, channel.ID, g.Name, g.ID, duration, len(uniqueUsers), len(state.SSRCUserMap))

				// Add warning about unmapped SSRCs if any
				if len(state.UnmappedSSRCs) > 0 {
					editContent += fmt.Sprintf("\n⚠️ **Unmapped SSRCs:** %d (users who joined before bot)", len(state.UnmappedSSRCs))
				}
			} else {
				editContent = fmt.Sprintf("Disconnected after %s.", duration)
			}
			_, _ = s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, editContent)
		}

		state.StreamsMutex.Lock()
		defer state.StreamsMutex.Unlock()
		for ssrc, stream := range state.ActiveStreams {
			h.finalizeStream(s, guildID, ssrc, stream)
			delete(state.ActiveStreams, ssrc)
		}
		h.StateManager.DeleteGuildState(guildID)
	}
}

func (h *VoiceHandler) getDisplayName(s *discordgo.Session, guildID string, user *discordgo.User) string {
	member, err := s.State.Member(guildID, user.ID)
	if err == nil && member.Nick != "" {
		return member.Nick
	}
	if user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

func (h *VoiceHandler) SpeakingUpdate(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
	user, err := h.Session.User(p.UserID)
	if err != nil {
		user = &discordgo.User{ID: p.UserID, Username: "Unknown User"}
	}

	// Ignore bots (including Dexter itself)
	if user.Bot {
		return
	}

	guildID := vc.GuildID
	state, ok := h.StateManager.GetGuildState(guildID)
	if !ok {
		return
	}

	// IMPORTANT: Map SSRC regardless of speaking status
	// Discord sends VoiceSpeakingUpdate events when:
	// 1. User starts speaking (Speaking=true)
	// 2. User stops speaking (Speaking=false)
	// 3. User joins the channel (may be Speaking=false initially)
	// We want to capture SSRCs in ALL cases to handle pre-existing users
	state.MetaMutex.Lock()
	state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
	state.MetaMutex.Unlock()

	// Save state to DB
	if h.DB != nil {
		if err := h.DB.SaveGuildState(guildID, state); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
		}
	}
}

func (h *VoiceHandler) finalizeStream(s *discordgo.Session, guildID string, ssrc uint32, stream *guild.UserStream) {
	_ = stream.OggWriter.Close()
	if h.DB != nil {
		key := h.DB.GenerateAudioCacheKey(stream.Filename)
		ttl := time.Duration(h.BotCfg.AudioTTLMinutes) * time.Minute
		if err := h.DB.SaveAudio(key, stream.Buffer.Bytes(), ttl); err != nil {
			h.Logger.Error(fmt.Sprintf("Failed to save audio to cache for key %s", key), err)
		}
	}
	endTime := time.Now()
	duration := endTime.Sub(stream.StartTime).Round(time.Second)

	g, err := s.State.Guild(guildID)
	if err != nil {
		g = &discordgo.Guild{Name: "Unknown Server"}
	}

	channel, err := s.State.Channel(stream.VoiceChannelID)
	if err != nil {
		channel = &discordgo.Channel{Name: "Unknown Channel"}
	}

	displayName := h.getDisplayName(s, guildID, stream.User)

	go h.transcribeAndUpdate(s, stream, g, channel, displayName, duration, endTime)
}

func (h *VoiceHandler) transcribeAndUpdate(s *discordgo.Session, stream *guild.UserStream, g *discordgo.Guild, channel *discordgo.Channel, displayName string, duration time.Duration, endTime time.Time) {
	if h.SttClient == nil {
		h.Logger.Error("STT client is nil, cannot transcribe", nil)
		// Delete audio immediately on error
		if h.DB != nil {
			_ = h.DB.DeleteAudio(h.DB.GenerateAudioCacheKey(stream.Filename))
		}
		return
	}

	audio, err := h.DB.GetAudio(h.DB.GenerateAudioCacheKey(stream.Filename))
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to get audio from cache for key %s", stream.Filename), err)
		// Delete audio immediately on error
		if h.DB != nil {
			_ = h.DB.DeleteAudio(h.DB.GenerateAudioCacheKey(stream.Filename))
		}
		return
	}

	transcription, err := h.SttClient.Transcribe(audio)

	// Delete audio immediately after transcription attempt (success or failure)
	if h.DB != nil {
		if err := h.DB.DeleteAudio(h.DB.GenerateAudioCacheKey(stream.Filename)); err != nil {
			h.Logger.Error(fmt.Sprintf("Failed to delete audio from cache for key %s", stream.Filename), err)
		}
	}

	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to transcribe audio for key %s", stream.Filename), err)
		return
	}

	if transcription == "" {
		return
	}

	// Add actual transcription to history
	state, ok := h.StateManager.GetGuildState(g.ID)
	if ok {
		state.MetaMutex.Lock()
		entry := guild.TranscriptionEntry{
			Duration:      duration,
			Username:      stream.User.Username,
			Transcription: transcription,
			Timestamp:     endTime,
			IsEvent:       false,
		}
		// Append to the correct channel's history
		history := state.TranscriptionHistory[stream.VoiceChannelID]
		history = append(history, entry)

		// Cap history at 100 entries
		if len(history) > 100 {
			history = history[len(history)-100:]
		}
		state.TranscriptionHistory[stream.VoiceChannelID] = history
		state.MetaMutex.Unlock()
	}

}

func (h *VoiceHandler) formatHeader(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) (string, error) {
	channel, err := s.Channel(vc.ChannelID)
	if err != nil {
		return "", fmt.Errorf("could not get channel: %w", err)
	}

	g, err := s.State.Guild(channel.GuildID)
	if err != nil {
		return "", fmt.Errorf("could not get guild: %w", err)
	}

	duration := time.Since(state.ConnectionStartTime).Round(time.Second)

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("**Connected to %s (%s) at %s (%s)**\n", channel.Name, channel.ID, g.Name, g.ID))
	msg.WriteString(fmt.Sprintf("**Duration:** %s\n\n", duration))

	return msg.String(), nil
}

func (h *VoiceHandler) formatUserList(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) (string, error) {
	g, err := s.State.Guild(vc.GuildID)
	if err != nil {
		return "", fmt.Errorf("could not get guild: %w", err)
	}

	voiceStates := g.VoiceStates
	usersInChannel := make(map[string]*discordgo.VoiceState)
	for _, vs := range voiceStates {
		if vs.ChannelID == vc.ChannelID {
			usersInChannel[vs.UserID] = vs
		}
	}

	if len(usersInChannel) == 0 {
		return "*No users in channel*", nil
	}

	state.MetaMutex.Lock()
	defer state.MetaMutex.Unlock()

	userSSRCMap := make(map[string][]uint32)
	for ssrc, userID := range state.SSRCUserMap {
		userSSRCMap[userID] = append(userSSRCMap[userID], ssrc)
	}

	type userEntry struct {
		userID      string
		user        *discordgo.User
		displayName string
		vs          *discordgo.VoiceState
	}

	userList := make([]userEntry, 0, len(usersInChannel))
	for userID, vs := range usersInChannel {
		user, err := s.User(userID)
		if err != nil {
			continue
		}
		displayName := h.getDisplayName(s, g.ID, user)
		userList = append(userList, userEntry{
			userID:      userID,
			user:        user,
			displayName: displayName,
			vs:          vs,
		})
	}

	sort.Slice(userList, func(i, j int) bool {
		return strings.ToLower(userList[i].displayName) < strings.ToLower(userList[j].displayName)
	})

	hasUnmappedSSRCs := len(state.UnmappedSSRCs) > 0

	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("**Users in Channel:** %d total\n", len(usersInChannel)))
	msg.WriteString("```\n")

	state.StreamsMutex.Lock()
	defer state.StreamsMutex.Unlock()
	for _, entry := range userList {
		var isSpeaking bool
		var speakingSSRC uint32
		for ssrc, stream := range state.ActiveStreams {
			if stream.User.ID == entry.userID {
				isSpeaking = true
				speakingSSRC = ssrc
				break
			}
		}

		userHasUnmappedSSRC := false
		if hasUnmappedSSRCs {
			_, hasMappedSSRC := userSSRCMap[entry.userID]
			if !hasMappedSSRC {
				userHasUnmappedSSRC = true
			}
		}

		var status string
		if entry.user.Bot {
			if entry.userID == s.State.User.ID {
				status = "🤖 Dexter"
			} else {
				status = "🤖 Bot"
			}
		} else if entry.vs.SelfMute {
			status = "🔇 (muted)"
		} else if entry.vs.SelfDeaf {
			status = "🔇 (deafened)"
		} else if isSpeaking {
			status = fmt.Sprintf("🔴 SPEAKING (SSRC: %d)", speakingSSRC)
		} else if ssrcs, ok := userSSRCMap[entry.userID]; ok && len(ssrcs) > 0 {
			status = fmt.Sprintf("💤 idle (SSRC: %d)", ssrcs[0])
		} else if userHasUnmappedSSRC && hasUnmappedSSRCs {
			status = "🚫 Unavailable"
		} else {
			status = "⏳ Unknown"
		}

		msg.WriteString(fmt.Sprintf("%s | %s (@%s)\n", status, entry.displayName, entry.user.Username))
		msg.WriteString(fmt.Sprintf("    User ID: %s\n", entry.userID))

		if ssrcs, ok := userSSRCMap[entry.userID]; ok && len(ssrcs) > 0 {
			msg.WriteString(fmt.Sprintf("    Known SSRCs: %v\n", ssrcs))
		}
		msg.WriteString("\n")
	}

	msg.WriteString("```")
	msg.WriteString(fmt.Sprintf("\n**Unmapped SSRCs:** %d\n", len(state.UnmappedSSRCs)))
	return msg.String(), nil
}

func (h *VoiceHandler) formatTranscriptionHistory(vc *discordgo.VoiceConnection, state *guild.GuildState) string {
	history := state.TranscriptionHistory[vc.ChannelID]
	var msg bytes.Buffer
	msg.WriteString("\n**Recent Transcriptions:**\n```\n")

	if len(history) == 0 {
		msg.WriteString("No transcriptions yet.\n")
	} else {
		startIdx := 0
		if len(history) > 10 {
			startIdx = len(history) - 10
		}

		for _, entry := range history[startIdx:] {
			if entry.IsEvent && entry.Duration == 0 {
				msg.WriteString(fmt.Sprintf("%s: %s\n", entry.Username, entry.Transcription))
			} else {
				msg.WriteString(fmt.Sprintf("[%s] %s: %s\n", entry.Duration.Round(time.Second), entry.Username, entry.Transcription))
			}
		}
	}
	msg.WriteString("```")
	return msg.String()
}

func (h *VoiceHandler) formatConnectionMessage(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) string {
	header, err := h.formatHeader(s, vc, state)
	if err != nil {
		return "Error formatting connection message header."
	}

	userList, err := h.formatUserList(s, vc, state)
	if err != nil {
		return "Error formatting user list."
	}

	transcriptionHistory := h.formatTranscriptionHistory(vc, state)

	var msg bytes.Buffer
	msg.WriteString(header)
	msg.WriteString(userList)
	msg.WriteString(transcriptionHistory)

	return msg.String()
}

func (h *VoiceHandler) finalizeChannelMove(s *discordgo.Session, vc *discordgo.VoiceConnection, oldState *guild.GuildState) {
	oldChannelID := oldState.ConnectionChannelID
	guildID := vc.GuildID

	// Cancel the old context (this will cause the old handleVoice goroutine to exit)
	// Note: We're in the old goroutine now, but we'll return after this function
	if oldState.CancelFunc != nil {
		oldState.CancelFunc()
	}

	// Finalize the old connection
	oldState.MetaMutex.Lock()
	duration := time.Since(oldState.ConnectionStartTime).Round(time.Second)
	channel, _ := s.Channel(oldChannelID)
	g, _ := s.State.Guild(guildID)

	// Count unique users who spoke

	uniqueUsers := make(map[string]bool)
	for _, userID := range oldState.SSRCUserMap {

		uniqueUsers[userID] = true
	}

	// Update old connection message
	if oldState.ConnectionMessageID != "" {
		var editContent string
		if channel != nil && g != nil {
			editContent = fmt.Sprintf("**Disconnected from %s (%s) at %s (%s)** (moved)\n**Duration:** %s\n**Users tracked:** %d\n**Total SSRCs:** %d",
				channel.Name, channel.ID, g.Name, g.ID, duration, len(uniqueUsers), len(oldState.SSRCUserMap))

			if len(oldState.UnmappedSSRCs) > 0 {
				editContent += fmt.Sprintf("\n⚠️ **Unmapped SSRCs:** %d (users who joined before bot)", len(oldState.UnmappedSSRCs))
			}
		} else {
			editContent = fmt.Sprintf("**Disconnected** (moved) after %s.", duration)
		}
		_, _ = s.ChannelMessageEdit(oldState.ConnectionMessageChannelID, oldState.ConnectionMessageID, editContent)
	}
	oldState.MetaMutex.Unlock()

	// Finalize any active streams
	oldState.StreamsMutex.Lock()
	for ssrc, stream := range oldState.ActiveStreams {
		h.finalizeStream(s, guildID, ssrc, stream)
		delete(oldState.ActiveStreams, ssrc)
	}
	oldState.StreamsMutex.Unlock()
	// Delete the old state
	h.StateManager.DeleteGuildState(guildID)

	// Create new state for the new channel
	newState := h.StateManager.GetOrStoreGuildState(guildID)
	newState.ConnectionChannelID = vc.ChannelID
	newState.ConnectionStartTime = time.Now()

	// Post a new connection message
	newChannel, err := s.Channel(vc.ChannelID)
	if err == nil {
		if g == nil {
			g, _ = s.State.Guild(guildID)
		}
		if g != nil {
			msg, _ := s.ChannelMessageSend(h.DiscordCfg.LogChannelID, fmt.Sprintf("Connecting to %s (%s) at %s (%s).", newChannel.Name, newChannel.ID, g.Name, g.ID))
			if msg != nil {
				newState.ConnectionMessageID = msg.ID
				newState.ConnectionMessageChannelID = msg.ChannelID
			}
		}
	}

	// Save the new state
	if h.DB != nil {
		if err := h.DB.SaveGuildState(guildID, newState); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
		}
	}

	// Start new voice handling for the new channel
	go h.handleVoice(s, vc, newState)
}

func (h *VoiceHandler) handleVoice(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) {
	for i := 0; i < 100; i++ {
		if vc.Ready {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !vc.Ready {
		h.Logger.Error("Timeout waiting for voice connection to be ready", nil)
		return
	}

	timeoutTicker := time.NewTicker(time.Duration(h.BotCfg.VoiceTimeoutSeconds) * time.Second)
	defer timeoutTicker.Stop()

	// Create a ticker for updating the connection status message
	statusUpdateTicker := time.NewTicker(5 * time.Second)
	defer statusUpdateTicker.Stop()

	// Initial message update
	if state.ConnectionMessageID != "" {
		msg := h.formatConnectionMessage(s, vc, state)
		_, _ = s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, msg)
	}

	for {
		select {
		case <-state.Ctx.Done():
			// Context was cancelled - clean shutdown requested
			return
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}
			h.handleAudioPacket(s, vc.GuildID, p, state)
		case <-timeoutTicker.C:
			h.checkStreamTimeouts(s, vc.GuildID, state)
		case <-statusUpdateTicker.C:
			// Check if the bot was moved to a different channel
			if vc.ChannelID != state.ConnectionChannelID {
				// Bot was moved - finalize this connection
				h.finalizeChannelMove(s, vc, state)
				return
			}

			// Update the connection status message periodically
			if state.ConnectionMessageID != "" {
				msg := h.formatConnectionMessage(s, vc, state)
				_, _ = s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, msg)
			}
		}
	}
}

func (h *VoiceHandler) handleAudioPacket(s *discordgo.Session, guildID string, p *discordgo.Packet, state *guild.GuildState) {
	state.StreamsMutex.Lock()
	defer state.StreamsMutex.Unlock()
	stream, ok := state.ActiveStreams[p.SSRC]
	if !ok {
		state.MetaMutex.Lock()
		userID, userOk := state.SSRCUserMap[p.SSRC]
		if !userOk {
			// We're receiving audio from an SSRC we don't have mapped
			// This can happen if Discord didn't send a VoiceSpeakingUpdate for this user
			// Track it for display in the status message
			if state.UnmappedSSRCs == nil {
				state.UnmappedSSRCs = make(map[uint32]bool)
			}
			state.UnmappedSSRCs[p.SSRC] = true
			state.MetaMutex.Unlock()
			return
		}
		state.MetaMutex.Unlock()
		user, err := s.User(userID)
		if err != nil {
			user = &discordgo.User{Username: "Unknown User", ID: userID}
		}

		// Ignore audio from bots (including Dexter itself)
		if user.Bot {
			return
		}
		startTime := time.Now()
		filename := fmt.Sprintf("%s-%d.ogg", user.ID, startTime.UnixNano())
		buffer := new(bytes.Buffer)
		oggWriter, err := oggwriter.NewWith(buffer, 48000, 2)
		if err != nil {
			return
		}

		stream = &guild.UserStream{
			VoiceChannelID: state.ConnectionChannelID,
			OggWriter:      oggWriter,
			Buffer:         buffer,
			LastPacket:     time.Now(),
			User:           user,
			StartTime:      startTime,
			Filename:       filename,
		}
		state.ActiveStreams[p.SSRC] = stream
	}
	stream.LastPacket = time.Now()
	rtpPacket := rtpPacketPool.Get().(*rtp.Packet)
	defer rtpPacketPool.Put(rtpPacket)
	rtpPacket.SequenceNumber = p.Sequence
	rtpPacket.Timestamp = p.Timestamp
	rtpPacket.SSRC = p.SSRC
	rtpPacket.Payload = p.Opus
	if err := stream.OggWriter.WriteRTP(rtpPacket); err != nil {
		log.Printf("Non-critical error writing RTP packet for SSRC %d: %v", p.SSRC, err)
	}
}

func (h *VoiceHandler) checkStreamTimeouts(s *discordgo.Session, guildID string, state *guild.GuildState) {

	state.StreamsMutex.Lock()

	defer state.StreamsMutex.Unlock()

	for ssrc, stream := range state.ActiveStreams {

		if time.Since(stream.LastPacket) > time.Duration(h.BotCfg.VoiceTimeoutSeconds)*time.Second {

			h.finalizeStream(s, guildID, ssrc, stream)

			delete(state.ActiveStreams, ssrc)

		}

	}

}
