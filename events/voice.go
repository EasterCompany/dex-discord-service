package events

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/guild"
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

func (h *Handler) getDisplayName(s *discordgo.Session, guildID string, user *discordgo.User) string {
	member, err := s.State.Member(guildID, user.ID)
	if err == nil && member.Nick != "" {
		return member.Nick
	}
	if user.GlobalName != "" {
		return user.GlobalName
	}
	return user.Username
}

func (h *Handler) SpeakingUpdate(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
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
	state.Mutex.Lock()
	state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
	state.Mutex.Unlock()

	// Save state to DB
	if h.DB != nil {
		if err := h.DB.SaveGuildState(guildID, state); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
		}
	}
}

func (h *Handler) finalizeStream(s *discordgo.Session, guildID string, ssrc uint32, stream *guild.UserStream) {
	_ = stream.OggWriter.Close()
	if h.DB != nil {
		key := h.GenerateAudioCacheKey(stream.Filename)
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

	// Add speaking events to history
	state, ok := h.StateManager.GetGuildState(guildID)
	if ok {
		state.Mutex.Lock()
		// Add "stopped speaking" event
		stoppedEntry := guild.TranscriptionEntry{
			Duration:      0,
			Username:      stream.User.Username,
			Transcription: "stopped speaking",
			Timestamp:     endTime,
			IsEvent:       true,
		}
		state.TranscriptionHistory = append(state.TranscriptionHistory, stoppedEntry)

		// Add "awaiting transcription" event
		awaitingEntry := guild.TranscriptionEntry{
			Duration:      duration,
			Username:      stream.User.Username,
			Transcription: "🔵 [awaiting transcription]",
			Timestamp:     endTime,
			IsEvent:       true,
		}
		state.TranscriptionHistory = append(state.TranscriptionHistory, awaitingEntry)
		state.Mutex.Unlock()
	}

	go h.transcribeAndUpdate(s, stream, g, channel, displayName, duration, endTime)
}

func (h *Handler) transcribeAndUpdate(s *discordgo.Session, stream *guild.UserStream, g *discordgo.Guild, channel *discordgo.Channel, displayName string, duration time.Duration, endTime time.Time) {
	if h.SttClient == nil {
		h.Logger.Error("STT client is nil, cannot transcribe", nil)
		// Delete audio immediately on error
		if h.DB != nil {
			_ = h.DB.DeleteAudio(h.GenerateAudioCacheKey(stream.Filename))
		}
		return
	}

	audio, err := h.DB.GetAudio(h.GenerateAudioCacheKey(stream.Filename))
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to get audio from cache for key %s", stream.Filename), err)
		// Delete audio immediately on error
		if h.DB != nil {
			_ = h.DB.DeleteAudio(h.GenerateAudioCacheKey(stream.Filename))
		}
		return
	}

	transcription, err := h.SttClient.Transcribe(audio)

	// Delete audio immediately after transcription attempt (success or failure)
	if h.DB != nil {
		if err := h.DB.DeleteAudio(h.GenerateAudioCacheKey(stream.Filename)); err != nil {
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
		state.Mutex.Lock()
		entry := guild.TranscriptionEntry{
			Duration:      duration,
			Username:      stream.User.Username,
			Transcription: transcription,
			Timestamp:     endTime,
			IsEvent:       false,
		}
		state.TranscriptionHistory = append(state.TranscriptionHistory, entry)
		state.Mutex.Unlock()
	}

	if h.DB != nil {
		transcriptionMessage := &discordgo.Message{
			ID:        stream.Message.ID,
			ChannelID: stream.VoiceChannelID,
			GuildID:   g.ID,
			Content:   transcription,
			Timestamp: endTime,
			Author:    stream.User,
		}
		key := h.GenerateMessageCacheKey(g.ID, stream.VoiceChannelID)
		if err := h.DB.AddMessage(key, transcriptionMessage); err != nil {
			h.Logger.Error(fmt.Sprintf("Error saving transcription message %s", stream.Message.ID), err)
		}
	}
}

func (h *Handler) formatConnectionMessage(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) string {
	channel, err := s.Channel(vc.ChannelID)
	if err != nil {
		return "Connected to voice channel."
	}

	g, err := s.State.Guild(channel.GuildID)
	if err != nil {
		return fmt.Sprintf("Connected to %s (%s).", channel.Name, channel.ID)
	}

	duration := time.Since(state.ConnectionStartTime).Round(time.Second)

	// Build base message
	var msg bytes.Buffer
	msg.WriteString(fmt.Sprintf("**Connected to %s (%s) at %s (%s)**\n", channel.Name, channel.ID, g.Name, g.ID))
	msg.WriteString(fmt.Sprintf("**Duration:** %s\n\n", duration))

	// Get users in voice channel
	voiceStates := g.VoiceStates
	usersInChannel := make(map[string]*discordgo.VoiceState)
	for _, vs := range voiceStates {
		if vs.ChannelID == vc.ChannelID {
			usersInChannel[vs.UserID] = vs
		}
	}

	if len(usersInChannel) == 0 {
		msg.WriteString("*No users in channel*")
		return msg.String()
	}

	// Lock state to read SSRC maps safely
	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	// Create reverse map for quick SSRC lookup
	userSSRCMap := make(map[string][]uint32)
	for ssrc, userID := range state.SSRCUserMap {
		userSSRCMap[userID] = append(userSSRCMap[userID], ssrc)
	}

	// Build a sorted list of users by display name
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

	// Sort alphabetically by display name
	sort.Slice(userList, func(i, j int) bool {
		return strings.ToLower(userList[i].displayName) < strings.ToLower(userList[j].displayName)
	})

	// Check if there are any unmapped SSRCs receiving audio
	hasUnmappedSSRCs := len(state.UnmappedSSRCs) > 0

	msg.WriteString(fmt.Sprintf("**Users in Channel:** %d total\n", len(usersInChannel)))
	msg.WriteString("```\n")

	for _, entry := range userList {
		// Check if user is currently speaking
		var isSpeaking bool
		var speakingSSRC uint32
		for ssrc, stream := range state.ActiveStreams {
			if stream.User.ID == entry.userID {
				isSpeaking = true
				speakingSSRC = ssrc
				break
			}
		}

		// Check if this user has audio being received but no SSRC mapping
		userHasUnmappedSSRC := false
		if hasUnmappedSSRCs {
			// If user has no SSRC mapping but we're receiving unmapped audio, they might be unavailable
			_, hasMappedSSRC := userSSRCMap[entry.userID]
			if !hasMappedSSRC {
				// User could be the one with unmapped SSRC
				userHasUnmappedSSRC = true
			}
		}

		// Build user line
		var status string
		if entry.user.Bot {
			// Check if this is Dexter itself
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

		// Show all known SSRCs for this user
		if ssrcs, ok := userSSRCMap[entry.userID]; ok && len(ssrcs) > 0 {
			msg.WriteString(fmt.Sprintf("    Known SSRCs: %v\n", ssrcs))
		}
		msg.WriteString("\n")
	}

	msg.WriteString("```")

	// Display recent transcriptions and events (last 10 entries)
	if len(state.TranscriptionHistory) > 0 {
		msg.WriteString("\n**Recent Transcriptions:**\n```\n")

		// Get last 10 entries
		startIdx := 0
		if len(state.TranscriptionHistory) > 10 {
			startIdx = len(state.TranscriptionHistory) - 10
		}

		for _, entry := range state.TranscriptionHistory[startIdx:] {
			if entry.IsEvent && entry.Duration == 0 {
				// Events without duration (like "stopped speaking")
				msg.WriteString(fmt.Sprintf("%s: %s\n", entry.Username, entry.Transcription))
			} else {
				// Events with duration or actual transcriptions
				msg.WriteString(fmt.Sprintf("[%s] %s: %s\n", entry.Duration.Round(time.Second), entry.Username, entry.Transcription))
			}
		}
		msg.WriteString("```")
	}

	return msg.String()
}

func (h *Handler) finalizeChannelMove(s *discordgo.Session, vc *discordgo.VoiceConnection, oldState *guild.GuildState) {
	oldChannelID := oldState.ConnectionChannelID
	guildID := vc.GuildID

	// Cancel the old context (this will cause the old handleVoice goroutine to exit)
	// Note: We're in the old goroutine now, but we'll return after this function
	if oldState.CancelFunc != nil {
		oldState.CancelFunc()
	}

	// Finalize the old connection
	oldState.Mutex.Lock()
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

	// Finalize any active streams
	for ssrc, stream := range oldState.ActiveStreams {
		h.finalizeStream(s, guildID, ssrc, stream)
		delete(oldState.ActiveStreams, ssrc)
	}
	oldState.Mutex.Unlock()

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

func (h *Handler) handleVoice(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) {
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

func (h *Handler) handleAudioPacket(s *discordgo.Session, guildID string, p *discordgo.Packet, state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	stream, ok := state.ActiveStreams[p.SSRC]
	if !ok {
		userID, userOk := state.SSRCUserMap[p.SSRC]
		if !userOk {
			// We're receiving audio from an SSRC we don't have mapped
			// This can happen if Discord didn't send a VoiceSpeakingUpdate for this user
			// Track it for display in the status message
			if state.UnmappedSSRCs == nil {
				state.UnmappedSSRCs = make(map[uint32]bool)
			}
			state.UnmappedSSRCs[p.SSRC] = true
			return
		}
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

		g, _ := s.State.Guild(guildID)
		channel, _ := s.State.Channel(state.ConnectionChannelID)
		displayName := h.getDisplayName(s, guildID, user)

		msgContent := fmt.Sprintf("`[%s]` **%s** (%s) in %s on %s: 🔴 [speaking...] | `Key: %s`",
			startTime.Format("15:04:05"),
			displayName,
			user.Username,
			channel.Name,
			g.Name,
			filename)

		msg, err := s.ChannelMessageSend(h.DiscordCfg.TranscriptionChannelID, msgContent)
		if err != nil {
			_ = oggWriter.Close()
			return
		}
		stream = &guild.UserStream{
			VoiceChannelID: state.ConnectionChannelID,
			OggWriter:      oggWriter,
			Buffer:         buffer,
			LastPacket:     time.Now(),
			Message:        msg,
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

func (h *Handler) checkStreamTimeouts(s *discordgo.Session, guildID string, state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	for ssrc, stream := range state.ActiveStreams {
		if time.Since(stream.LastPacket) > time.Duration(h.BotCfg.VoiceTimeoutSeconds)*time.Second {
			h.finalizeStream(s, guildID, ssrc, stream)
			delete(state.ActiveStreams, ssrc)
		}
	}
}
