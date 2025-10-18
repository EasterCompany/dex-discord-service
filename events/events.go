package events

import (
	"bytes"
	"fmt"
	"log"
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
	db            cache.Cache
	discordCfg    *config.DiscordConfig
	guildStates   sync.Map
	rtpPacketPool = sync.Pool{
		New: func() interface{} {
			return &rtp.Packet{
				Header: rtp.Header{
					Version:     2,
					PayloadType: 0x78,
				},
			}
		},
	}
)

func Init(database cache.Cache, cfg *config.DiscordConfig) {
	db = database
	discordCfg = cfg
}

// GenerateMessageCacheKey creates a standardized key part for storing messages.
func GenerateMessageCacheKey(guildID, channelID string) string {
	if guildID == "" {
		return fmt.Sprintf("messages:dm:%s", channelID)
	}
	return fmt.Sprintf("messages:guild:%s:channel:%s", guildID, channelID)
}

// GenerateAudioCacheKey creates a standardized key part for storing audio data.
func GenerateAudioCacheKey(filename string) string {
	return fmt.Sprintf("audio:%s", filename)
}

func Ready(s *discordgo.Session, r *discordgo.Ready) {
	logger.Post("Connection established. Starting initial message history sync...")
	for _, g := range r.Guilds {
		channels, err := s.GuildChannels(g.ID)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get channels for guild %s", g.ID), err)
			continue
		}
		for _, c := range channels {
			if c.Type == discordgo.ChannelTypeGuildText {
				go fetchAndStoreLast50Messages(s, c.GuildID, c.ID)
			}
		}
	}
	channels, err := s.UserChannels()
	if err != nil {
		logger.Error("Failed to get user (private) channels", err)
	} else {
		for _, c := range channels {
			go fetchAndStoreLast50Messages(s, "", c.ID)
		}
	}
	logger.Post("Initial message sync process initiated.")
}

func fetchAndStoreLast50Messages(s *discordgo.Session, guildID, channelID string) {
	if db == nil {
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
	key := GenerateMessageCacheKey(guildID, channelID)
	if err := db.BulkInsertMessages(key, messages); err != nil {
		logger.Error(fmt.Sprintf("Failed to bulk insert messages for channel %s", channelID), err)
	}
}

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

func SpeakingUpdate(s *discordgo.Session, p *discordgo.VoiceSpeakingUpdate) {
	if p.Speaking {
		guildID, ok := findGuildIDForUser(s, p.UserID)
		if !ok {
			return
		}
		value, ok := guildStates.Load(guildID)
		if !ok {
			return
		}
		state := value.(*guild.GuildState)
		state.Mutex.Lock()
		defer state.Mutex.Unlock()
		state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
		if db != nil {
			if err := db.SaveGuildState(guildID, state); err != nil {
				logger.Error(fmt.Sprintf("Error saving guild state for guild %s", guildID), err)
			}
		}
	}
}

func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}
	if db != nil {
		key := GenerateMessageCacheKey(m.GuildID, m.ChannelID)
		if err := db.AddMessage(key, m.Message); err != nil {
			logger.Error(fmt.Sprintf("Error saving message %s", m.ID), err)
		}
	}
	if m.GuildID != "" {
		switch {
		case strings.HasPrefix(m.Content, "!join"):
			joinVoice(s, m)
		case strings.HasPrefix(m.Content, "!leave"):
			leaveVoice(s, m)
		}
	}
}

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		logger.Error("Error getting guild", err)
		return
	}
	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			value, _ := guildStates.LoadOrStore(m.GuildID, guild.NewGuildState())
			state := value.(*guild.GuildState)
			if db != nil {
				if err := db.SaveGuildState(m.GuildID, state); err != nil {
					logger.Error(fmt.Sprintf("Error saving guild state for guild %s", m.GuildID), err)
				}
			}
			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, true)
			if err != nil {
				logger.Error("Error joining voice channel", err)
				return
			}
			go handleVoice(s, vc, state)
			return
		}
	}
	s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel for me to join!")
}

func leaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	if vc, ok := s.VoiceConnections[m.GuildID]; ok {
		vc.Disconnect()
		value, ok := guildStates.Load(m.GuildID)
		if !ok {
			return
		}
		state := value.(*guild.GuildState)
		state.Mutex.Lock()
		defer state.Mutex.Unlock()
		for ssrc, stream := range state.ActiveStreams {
			stream.OggWriter.Close()
			delete(state.ActiveStreams, ssrc)
		}
	}
}

func handleVoice(s *discordgo.Session, vc *discordgo.VoiceConnection, state *guild.GuildState) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	logger.Post("Voice handler started. Listening for audio...")
	for {
		select {
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}
			handleAudioPacket(s, p, state)
		case <-ticker.C:
			checkStreamTimeouts(s, state)
		}
	}
}

func handleAudioPacket(s *discordgo.Session, p *discordgo.Packet, state *guild.GuildState) {
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
		msgContent := fmt.Sprintf("`[%s]` **%s**: 🔴 [speaking...] | `Key: %s`",
			startTime.Format("15:04:05"), user.Username, filename)
		msg, err := s.ChannelMessageSend(discordCfg.TranscriptionChannelID, msgContent)
		if err != nil {
			logger.Error("Failed to send initial timeline message", err)
			oggWriter.Close()
			return
		}
		stream = &guild.UserStream{
			OggWriter:  oggWriter,
			Buffer:     buffer,
			LastPacket: time.Now(),
			Message:    msg,
			User:       user,
			StartTime:  startTime,
			Filename:   filename,
		}
		state.ActiveStreams[p.SSRC] = stream
	}
	stream.LastPacket = time.Now()
	rtpPacket := rtpPacketPool.Get().(*rtp.Packet)
	defer rtpPacketPool.Put(rtpPacket)
	rtpPacket.Header.SequenceNumber = p.Sequence
	rtpPacket.Header.Timestamp = p.Timestamp
	rtpPacket.Header.SSRC = p.SSRC
	rtpPacket.Payload = p.Opus
	if err := stream.OggWriter.WriteRTP(rtpPacket); err != nil {
		log.Printf("Non-critical error writing RTP packet for SSRC %d: %v", p.SSRC, err)
	}
}

func checkStreamTimeouts(s *discordgo.Session, state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()
	for ssrc, stream := range state.ActiveStreams {
		if time.Since(stream.LastPacket) > time.Second*1 {
			stream.OggWriter.Close()
			if db != nil {
				key := GenerateAudioCacheKey(stream.Filename)
				ttl := time.Duration(discordCfg.AudioTTLDays) * 24 * time.Hour
				if err := db.SaveAudio(key, stream.Buffer.Bytes(), ttl); err != nil {
					logger.Error(fmt.Sprintf("Failed to save audio to cache for key %s", key), err)
				}
			}
			endTime := time.Now()
			duration := endTime.Sub(stream.StartTime).Round(time.Second)
			msgContent := fmt.Sprintf("`[%s - %s]` **%s**: 🔵 [awaiting transcription] `(%s)` | `Key: %s`",
				stream.StartTime.Format("15:04:05"),
				endTime.Format("15:04:05"),
				stream.User.Username,
				duration,
				stream.Filename,
			)
			s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, msgContent)
			delete(state.ActiveStreams, ssrc)
		}
	}
}
