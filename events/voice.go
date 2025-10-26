package events

import (
	"bytes"
	"fmt"
	"log"
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

	guildID := vc.GuildID
	now := time.Now().Format("15:04:05")
	var logMessage string

	if p.Speaking {
		state, ok := h.StateManager.GetGuildState(guildID)
		if !ok {
			return
		}
		state.Mutex.Lock()
		state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
		state.Mutex.Unlock()
		if h.DB != nil {
			if err := h.DB.SaveGuildState(guildID, state); err != nil {
				h.Logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
			}
		}
	} else {
		logMessage = fmt.Sprintf("`%s` **%s** stopped speaking.", now, user.Username)
	}

	if h.DiscordCfg.TranscriptionChannelID != "" && logMessage != "" {
		_, _ = h.Session.ChannelMessageSend(h.DiscordCfg.TranscriptionChannelID, logMessage)
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

	msgContent := fmt.Sprintf("`[%s - %s]` **%s** (%s) in %s on %s: 🔵 [awaiting transcription] `(%s)` | `Key: %s`",
		stream.StartTime.Format("15:04:05"),
		endTime.Format("15:04:05"),
		displayName,
		stream.User.Username,
		channel.Name,
		g.Name,
		duration,
		stream.Filename,
	)
	_, _ = s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, msgContent)

	go h.transcribeAndUpdate(s, stream, g, channel, displayName, duration, endTime)
}

func (h *Handler) transcribeAndUpdate(s *discordgo.Session, stream *guild.UserStream, g *discordgo.Guild, channel *discordgo.Channel, displayName string, duration time.Duration, endTime time.Time) {
	if h.SttClient == nil {
		h.Logger.Error("STT client is nil, cannot transcribe", nil)
		return
	}

	audio, err := h.DB.GetAudio(h.GenerateAudioCacheKey(stream.Filename))
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to get audio from cache for key %s", stream.Filename), err)
		return
	}

	transcription, err := h.SttClient.Transcribe(audio)
	if err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to transcribe audio for key %s", stream.Filename), err)
		return
	}

	if err := h.DB.DeleteAudio(h.GenerateAudioCacheKey(stream.Filename)); err != nil {
		h.Logger.Error(fmt.Sprintf("Failed to delete audio from cache for key %s", stream.Filename), err)
	}

	if transcription == "" {
		_ = s.ChannelMessageDelete(stream.Message.ChannelID, stream.Message.ID)
		return
	}

	msgContent := fmt.Sprintf("`[%s - %s]` **%s** (%s) in %s on %s: %s",
		stream.StartTime.Format("15:04:05"),
		endTime.Format("15:04:05"),
		displayName,
		stream.User.Username,
		channel.Name,
		g.Name,
		transcription,
	)
	_, _ = s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, msgContent)

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

	ticker := time.NewTicker(time.Duration(h.BotCfg.VoiceTimeoutSeconds) * time.Second)
	defer ticker.Stop()
	if state.ConnectionMessageID != "" {
		channel, err := s.Channel(vc.ChannelID)
		if err == nil {
			g, err := s.State.Guild(channel.GuildID)
			if err == nil {
				_, _ = s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, fmt.Sprintf("Connected to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
			}
		}
	}
	for {
		select {
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}
			h.handleAudioPacket(s, vc.GuildID, p, state)
		case <-ticker.C:
			h.checkStreamTimeouts(s, vc.GuildID, state)
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
			return
		}
		user, err := s.User(userID)
		if err != nil {
			user = &discordgo.User{Username: "Unknown User", ID: userID}
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
