package events

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/worker"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

var (
	db                    interfaces.Database
	stt                   interfaces.STT
	discordCfg            *config.DiscordConfig
	guildStates           sync.Map
	channelHistoryFetched = make(map[string]bool)
	historyMutex          sync.Mutex
	wp                    *worker.WorkerPool
	// rtpPacketPool is used to reuse rtp.Packet objects to reduce memory allocations.
	rtpPacketPool = sync.Pool{
		New: func() interface{} {
			return &rtp.Packet{}
		},
	}
)

// Init initializes the events module with the database, stt clients and discord config
func Init(database interfaces.Database, sttClient interfaces.STT, cfg *config.DiscordConfig, workerPool *worker.WorkerPool) {
	db = database
	stt = sttClient
	discordCfg = cfg
	wp = workerPool
}

// LoadGuildState loads a guild state into the events module
func LoadGuildState(guildID string, state *guild.GuildState) {
	guildStates.Store(guildID, state)
}

// MessageCreate handles incoming messages, routes commands, and logs messages.
func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Log all messages from guilds
	if m.GuildID != "" {
		if err := db.SaveMessage(m.GuildID, m.ChannelID, m.Message); err != nil {
			logger.Error(fmt.Sprintf("saving message %s", m.ID), err)
		}
	}

	// Handle commands
	switch {
	case strings.HasPrefix(m.Content, "!join"):
		joinVoice(s, m)
	case strings.HasPrefix(m.Content, "!leave"):
		leaveVoice(s, m)
	}
}

func fetchAndSaveChannelHistory(s *discordgo.Session, guildID, channelID string) {
	historyMutex.Lock()
	if channelHistoryFetched[channelID] {
		historyMutex.Unlock()
		return
	}
	historyMutex.Unlock()

	var allMessages []*discordgo.Message
	lastID := ""

	for {
		messages, err := s.ChannelMessages(channelID, 50, lastID, "", "")
		if err != nil {
			logger.Error(fmt.Sprintf("fetching messages for channel %s", channelID), err)
			return
		}

		if len(messages) == 0 {
			break
		}

		allMessages = append(allMessages, messages...)
		lastID = messages[len(messages)-1].ID

		// Add a delay to avoid rate-limiting
		time.Sleep(200 * time.Millisecond)
	}

	// Reverse the messages to have them in chronological order.
	for i, j := 0, len(allMessages)-1; i < j; i, j = i+1, j-1 {
		allMessages[i], allMessages[j] = allMessages[j], allMessages[i]
	}

	if err := db.SaveMessageHistory(guildID, channelID, allMessages); err != nil {
		logger.Error(fmt.Sprintf("saving message history for channel %s", channelID), err)
		return
	}

	historyMutex.Lock()
	channelHistoryFetched[channelID] = true
	historyMutex.Unlock()
}

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		logger.Error("getting guild", err)
		return
	}

	// Fetch channel history in a goroutine.
	go fetchAndSaveChannelHistory(s, m.GuildID, m.ChannelID)

	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			value, _ := guildStates.LoadOrStore(m.GuildID, guild.NewGuildState())
			state := value.(*guild.GuildState)
			if err := db.SaveGuildState(m.GuildID, state); err != nil {
				logger.Error(fmt.Sprintf("saving guild state for guild %s", m.GuildID), err)
			}

			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, true)
			if err != nil {
				logger.Error("joining voice channel", err)
				return
			}
			go handleVoice(s, m.GuildID, vs.ChannelID, vc) // Pass GuildID and the voice channel ID
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
			stream.Writer.Close()
			stream.CancelFunc()
			delete(state.ActiveStreams, ssrc)
		}
	}
}

func handleVoice(s *discordgo.Session, guildID, voiceChannelID string, vc *discordgo.VoiceConnection) {
	value, ok := guildStates.Load(guildID)
	if !ok {
		logger.Error(fmt.Sprintf("guild state not found for guild %s", guildID), fmt.Errorf("state not found in sync.Map"))
		return
	}
	state := value.(*guild.GuildState)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}
			handleAudioPacket(s, guildID, voiceChannelID, p, state)
		case <-ticker.C:
			checkStreamTimeouts(state)
		}
	}
}

func handleAudioPacket(s *discordgo.Session, guildID, voiceChannelID string, p *discordgo.Packet, state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	stream, ok := state.ActiveStreams[p.SSRC]
	if !ok {
		userID, userOk := state.SSRCUserMap[p.SSRC]
		if !userOk {
			// A user's audio packet can arrive before their speaking update.
			// We will temporarily map the SSRC to a placeholder, and update it
			// once the speaking update arrives.
			return
		}

		pr, pw := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())

		oggWriter, err := oggwriter.NewWith(pw, 48000, 2)
		if err != nil {
			logger.Error(fmt.Sprintf("creating Ogg writer for SSRC %d", p.SSRC), err)
			cancel()
			pw.CloseWithError(err)
			return
		}

		user, err := s.User(userID)
		if err != nil {
			user = &discordgo.User{Username: "Unknown User", ID: userID}
		}

		startTime := time.Now()
		msg, err := s.ChannelMessageSend(discordCfg.TranscriptionChannelID, fmt.Sprintf("`%s` started speaking at `%s`", user.Username, startTime.Format("15:04:05 MST")))
		if err != nil {
			logger.Error("sending initial transcription message", err)
			cancel()
			pw.Close()
			return
		}

		stream = &guild.UserStream{
			Writer:     oggWriter,
			OggWriter:  oggWriter,
			CancelFunc: cancel,
			LastPacket: time.Now(),
			Message:    msg,
			User:       user,
			StartTime:  startTime,
			GuildID:    guildID,
			ChannelID:  voiceChannelID,
		}
		state.ActiveStreams[p.SSRC] = stream

		job := worker.TranscriptionJob{
			Ctx:     ctx,
			Session: s,
			Reader:  pr,
			Stream:  stream,
			DB:      db,
			STT:     stt,
		}
		wp.Submit(job)
	}

	stream.LastPacket = time.Now()

	// Get a packet from the pool to reuse it.
	rtpPacket := rtpPacketPool.Get().(*rtp.Packet)
	// Defer putting the packet back in the pool until the function returns.
	defer rtpPacketPool.Put(rtpPacket)

	// Populate the packet with the new data.
	rtpPacket.Header = rtp.Header{
		Version:        2,
		PayloadType:    0x78,
		SequenceNumber: p.Sequence,
		Timestamp:      p.Timestamp,
		SSRC:           p.SSRC,
	}
	rtpPacket.Payload = p.Opus

	if err := stream.OggWriter.WriteRTP(rtpPacket); err != nil {
		logger.Error(fmt.Sprintf("writing RTP packet for SSRC %d", p.SSRC), err)
	}
}

func checkStreamTimeouts(state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	for ssrc, stream := range state.ActiveStreams {
		if time.Since(stream.LastPacket) > 2*time.Second {
			stream.Writer.Close()
			stream.CancelFunc()
			delete(state.ActiveStreams, ssrc)
		}
	}
}
