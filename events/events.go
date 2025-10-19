package events

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/guild"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

var (
	guildStates   sync.Map
	rtpPacketPool = sync.Pool{
		New: func() any {
			return &rtp.Packet{
				Header: rtp.Header{
					Version:     2,
					PayloadType: 0x78,
				},
			}
		},
	}
)

// Handler holds the dependencies for the event handlers.
type Handler struct {
	DB         cache.Cache
	DiscordCfg *config.DiscordConfig
	BotCfg     *config.BotConfig
	Session    *discordgo.Session
}

// NewHandler creates a new event handler with its dependencies.
func NewHandler(db cache.Cache, discordCfg *config.DiscordConfig, botCfg *config.BotConfig, s *discordgo.Session) *Handler {
	return &Handler{
		DB:         db,
		DiscordCfg: discordCfg,
		BotCfg:     botCfg,
		Session:    s,
	}
}

// GenerateMessageCacheKey creates a standardized key for storing messages.
func (h *Handler) GenerateMessageCacheKey(guildID, channelID string) string {
	if guildID == "" {
		return fmt.Sprintf("messages:dm:%s", channelID)
	}
	return fmt.Sprintf("messages:guild:%s:channel:%s", guildID, channelID)
}

// GenerateAudioCacheKey creates a standardized key for storing audio data.
func (h *Handler) GenerateAudioCacheKey(filename string) string {
	return fmt.Sprintf("audio:%s", filename)
}

// Ready is called when the bot has successfully connected to Discord.
func (h *Handler) Ready(s *discordgo.Session, r *discordgo.Ready) {
	logger.Post("Connection established. Starting initial message history sync...")
	for _, g := range r.Guilds {
		channels, err := s.GuildChannels(g.ID)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get channels for guild %s", g.ID), err)
			continue
		}
		for _, c := range channels {
			if c.Type == discordgo.ChannelTypeGuildText {
				go h.fetchAndStoreLast50Messages(s, c.GuildID, c.ID)
			}
		}
	}
	for _, c := range r.PrivateChannels {
		go h.fetchAndStoreLast50Messages(s, "", c.ID)
	}
	logger.Post("Initial message sync process initiated.")
}

func (h *Handler) fetchAndStoreLast50Messages(s *discordgo.Session, guildID, channelID string) {
	if h.DB == nil {
		return
	}
	messages, err := s.ChannelMessages(channelID, 50, "", "", "")
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to fetch messages for channel %s", channelID), err)
		return
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	key := h.GenerateMessageCacheKey(guildID, channelID)
	if err := h.DB.BulkInsertMessages(key, messages); err != nil {
		logger.Error(fmt.Sprintf("Failed to bulk insert messages for channel %s", channelID), err)
	}
}

// LoadGuildState is a top-level function to allow main to populate initial state.
func LoadGuildState(guildID string, state *guild.GuildState) {
	guildStates.Store(guildID, state)
}

func findGuildIDForUser(s *discordgo.Session, userID string) (string, bool) {
	for _, g := range s.State.Guilds {
		for _, vs := range g.VoiceStates {
			if vs.UserID == userID {
				return g.ID, true
			}
		}
	}
	return "", false
}

// SpeakingUpdate handles users starting to speak.
func (h *Handler) SpeakingUpdate(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
	user, err := h.Session.User(p.UserID)
	if err != nil {
		user = &discordgo.User{ID: p.UserID, Username: "Unknown User"}
	}

	guildID := vc.GuildID

	now := time.Now().Format("15:04:05")
	var logMessage string

	if p.Speaking {
		value, ok := guildStates.Load(guildID)
		if !ok {
			return
		}
		state := value.(*guild.GuildState)
		state.Mutex.Lock()
		state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
		state.Mutex.Unlock()
		if h.DB != nil {
			if err := h.DB.SaveGuildState(guildID, state); err != nil {
				logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
			}
		}
	} else {
		logMessage = fmt.Sprintf("`%s` **%s** stopped speaking.", now, user.Username)
	}

	if h.DiscordCfg.TranscriptionChannelID != "" && logMessage != "" {
		h.Session.ChannelMessageSend(h.DiscordCfg.TranscriptionChannelID, logMessage)
	}
}

// MessageCreate handles new messages.
func (h *Handler) MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	if h.DB != nil {
		key := h.GenerateMessageCacheKey(m.GuildID, m.ChannelID)
		if err := h.DB.AddMessage(key, m.Message); err != nil {
			logger.Error(fmt.Sprintf("Error saving message %s", m.ID), err)
		}
	}
	if m.GuildID != "" {
		switch {
		case strings.HasPrefix(m.Content, "!join"):
			h.joinVoice(s, m)
		case strings.HasPrefix(m.Content, "!leave"):
			h.leaveVoice(s, m)
		}
	}
}

func (h *Handler) finalizeStream(s *discordgo.Session, guildID string, ssrc uint32, stream *guild.UserStream) {
	stream.OggWriter.Close()
	if h.DB != nil {
		key := h.GenerateAudioCacheKey(stream.Filename)
		ttl := time.Duration(h.BotCfg.AudioTTLMinutes) * time.Minute
		if err := h.DB.SaveAudio(key, stream.Buffer.Bytes(), ttl); err != nil {
			logger.Error(fmt.Sprintf("Failed to save audio to cache for key %s", key), err)
		}
	}
	endTime := time.Now()
	duration := endTime.Sub(stream.StartTime).Round(time.Second)

	// Get Guild
	g, err := s.State.Guild(guildID)
	if err != nil {
		logger.Error(fmt.Sprintf("Error getting guild %s for transcription message", guildID), err)
		g = &discordgo.Guild{Name: "Unknown Server"}
	}

	// Get Channel
	channel, err := s.State.Channel(stream.VoiceChannelID)
	if err != nil {
		logger.Error(fmt.Sprintf("Error getting channel %s for transcription message", stream.VoiceChannelID), err)
		channel = &discordgo.Channel{Name: "Unknown Channel"}
	}

	// Get Member (for nickname)
	member, err := s.State.Member(guildID, stream.User.ID)
	var displayName string
	if err == nil && member.Nick != "" {
		displayName = member.Nick
	} else if stream.User.GlobalName != "" {
		displayName = stream.User.GlobalName
	} else {
		displayName = stream.User.Username
	}

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
	s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, msgContent)
}

func (h *Handler) disconnectFromVoice(s *discordgo.Session, guildID string) {
	if vc, ok := s.VoiceConnections[guildID]; ok {
		vc.Disconnect()
		value, ok := guildStates.Load(guildID)
		if !ok {
			return
		}
		state := value.(*guild.GuildState)
		state.Mutex.Lock()
		defer state.Mutex.Unlock()

		if state.ConnectionMessageID != "" {
			duration := time.Since(state.ConnectionStartTime).Round(time.Second)
			channel, _ := s.Channel(state.ConnectionChannelID)
			g, _ := s.State.Guild(guildID)

			var editContent string
			if channel != nil && g != nil {
				editContent = fmt.Sprintf("Disconnected from %s (%s) at %s (%s) after %s.",
					channel.Name, channel.ID, g.Name, g.ID, duration)
			} else {
				editContent = fmt.Sprintf("Disconnected after %s.", duration)
			}
			s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, editContent)
		}

		for ssrc, stream := range state.ActiveStreams {
			h.finalizeStream(s, guildID, ssrc, stream)
			delete(state.ActiveStreams, ssrc)
		}
		guildStates.Delete(guildID)
	}
}

func (h *Handler) joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.disconnectFromVoice(s, m.GuildID)
	time.Sleep(1 * time.Second) // Wait for the disconnection to complete

	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		logger.Error("Error getting guild", err)
		return
	}
	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			value, _ := guildStates.LoadOrStore(m.GuildID, guild.NewGuildState())
			state := value.(*guild.GuildState)
			if h.DB != nil {
				if err := h.DB.SaveGuildState(m.GuildID, state); err != nil {
					logger.Error(fmt.Sprintf("Error saving guild state for guild %s", m.GuildID), err)
				}
			}
			channel, err := s.Channel(vs.ChannelID)
			if err != nil {
				logger.Error("Error getting channel", err)
				return
			}
			msg, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Connecting to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
			if err != nil {
				logger.Error("Error sending connecting message", err)
			}
			if msg != nil {
				state.ConnectionMessageID = msg.ID
				state.ConnectionMessageChannelID = msg.ChannelID
				state.ConnectionStartTime = time.Now()
			}

			const maxRetries = 3
			var vc *discordgo.VoiceConnection

			for i := 0; i < maxRetries; i++ {
				vc, err = s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, false)
				if err == nil {
					break // Success
				}

				retrySeconds := int(math.Pow(2, float64(i)))
				if msg != nil {
					s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect, retrying in %d seconds...", retrySeconds))
				}
				time.Sleep(time.Duration(retrySeconds) * time.Second)
			}

			if err != nil {
				logger.Error("Error joining voice channel", err)
				if msg != nil {
					s.ChannelMessageEdit(m.ChannelID, msg.ID, fmt.Sprintf("Failed to connect to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
				}
				h.disconnectFromVoice(s, m.GuildID) // Clean up after failed join
				return
			}

			vc.AddHandler(func(vc *discordgo.VoiceConnection, p *discordgo.VoiceSpeakingUpdate) {
				h.SpeakingUpdate(vc, p)
			})
			state.ConnectionChannelID = vc.ChannelID // Update with actual voice channel ID
			go h.handleVoice(s, vc, state)
			return
		}
	}
	s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel for me to join!")
}

func (h *Handler) leaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	h.disconnectFromVoice(s, m.GuildID)
}

func (h *Handler) handleVoice(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) {
	// Wait for the voice connection to be ready, with a timeout
	for i := 0; i < 100; i++ { // 10 seconds timeout
		if vc.Ready {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !vc.Ready {
		logger.Error("Timeout waiting for voice connection to be ready", nil)
		return
	}

	ticker := time.NewTicker(time.Duration(h.BotCfg.VoiceTimeoutSeconds) * time.Second)
	defer ticker.Stop()
	if state.ConnectionMessageID != "" {
		channel, err := s.Channel(vc.ChannelID)
		if err != nil {
			logger.Error("Error getting channel", err)
		} else {
			g, err := s.State.Guild(channel.GuildID)
			if err != nil {
				logger.Error("Error getting guild", err)
			} else {
				s.ChannelMessageEdit(state.ConnectionMessageChannelID, state.ConnectionMessageID, fmt.Sprintf("Connected to %s (%s) at %s (%s).", channel.Name, channel.ID, g.Name, g.ID))
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
			logger.Error(fmt.Sprintf("Failed to create Ogg writer for %s", filename), err)
			return
		}

		// Get Guild
		g, err := s.State.Guild(guildID)
		if err != nil {
			logger.Error(fmt.Sprintf("Error getting guild %s for transcription message", guildID), err)
			g = &discordgo.Guild{Name: "Unknown Server"}
		}

		// Get Channel
		channel, err := s.State.Channel(state.ConnectionChannelID)
		if err != nil {
			logger.Error(fmt.Sprintf("Error getting channel %s for transcription message", state.ConnectionChannelID), err)
			channel = &discordgo.Channel{Name: "Unknown Channel"}
		}

		// Get Member (for nickname)
		member, err := s.State.Member(guildID, user.ID)
		var displayName string
		if err == nil && member.Nick != "" {
			displayName = member.Nick
		} else if user.GlobalName != "" {
			displayName = user.GlobalName
		} else {
			displayName = user.Username
		}

		msgContent := fmt.Sprintf("`[%s]` **%s** (%s) in %s on %s: 🔴 [speaking...] | `Key: %s`",
			startTime.Format("15:04:05"),
			displayName,
			user.Username,
			channel.Name,
			g.Name,
			filename)

		msg, err := s.ChannelMessageSend(h.DiscordCfg.TranscriptionChannelID, msgContent)
		if err != nil {
			logger.Error("Failed to send initial timeline message", err)
			oggWriter.Close()
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
