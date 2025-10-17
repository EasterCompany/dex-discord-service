package events

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/guild"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

var (
	db                    interfaces.Database
	stt                   interfaces.STT
	guildStates           sync.Map
	channelHistoryFetched = make(map[string]bool)
	historyMutex          sync.Mutex
)

// Init initializes the events module with the database and stt clients
func Init(database interfaces.Database, sttClient interfaces.STT) {
	db = database
	stt = sttClient
}

// LoadGuildState loads a guild state into the events module
func LoadGuildState(guildID string, state *guild.GuildState) {
	guildStates.Store(guildID, state)
}

// SpeakingUpdate is triggered when a user starts or stops speaking.
func SpeakingUpdate(s *discordgo.Session, p *discordgo.VoiceSpeakingUpdate) {
	// Find the guild the user is in
	for _, g := range s.State.Guilds {
		for _, vs := range g.VoiceStates {
			if vs.UserID == p.UserID {
				value, ok := guildStates.Load(g.ID)
				if !ok {
					// Guild state not initialized yet, should be created by joinVoice
					return
				}
				state := value.(*guild.GuildState)

				state.Mutex.Lock()
				defer state.Mutex.Unlock()

				if p.Speaking {
					state.SSRCUserMap[uint32(p.SSRC)] = p.UserID
					if err := db.SaveGuildState(g.ID, state); err != nil {
						log.Printf("Error saving guild state for guild %s: %v", g.ID, err)
					}
				}
				return
			}
		}
	}
}

// MessageCreate handles incoming messages, routes commands, and logs messages.
func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Log all messages from guilds
	if m.GuildID != "" {
		if err := db.SaveMessage(m.GuildID, m.ChannelID, m.Message); err != nil {
			log.Printf("Error saving message %s: %v", m.ID, err)
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

	log.Printf("Starting history fetch for channel %s", channelID)

	var allMessages []*discordgo.Message
	lastID := ""

	for {
		messages, err := s.ChannelMessages(channelID, 50, lastID, "", "")
		if err != nil {
			log.Printf("Error fetching messages for channel %s: %v", channelID, err)
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
		log.Printf("Error saving message history for channel %s: %v", channelID, err)
		return
	}

	historyMutex.Lock()
	channelHistoryFetched[channelID] = true
	historyMutex.Unlock()

	log.Printf("Successfully fetched and saved history for channel %s", channelID)
}

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		log.Printf("Error getting guild: %v", err)
		return
	}

	// Fetch channel history in a goroutine.
	go fetchAndSaveChannelHistory(s, m.GuildID, m.ChannelID)

	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			value, _ := guildStates.LoadOrStore(m.GuildID, guild.NewGuildState())
			state := value.(*guild.GuildState)
			if err := db.SaveGuildState(m.GuildID, state); err != nil {
				log.Printf("Error saving guild state for guild %s: %v", m.GuildID, err)
			}

			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, true)
			if err != nil {
				log.Printf("Error joining voice channel: %v", err)
				return
			}
			s.AddHandler(SpeakingUpdate)
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
		log.Printf("Error: guild state not found for guild %s", guildID)
		return
	}
	state := value.(*guild.GuildState)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	log.Println("Voice handler started. Listening for audio...")

	for {
		select {
		case p, ok := <-vc.OpusRecv:
			if !ok {
				log.Println("Voice channel OpusRecv channel closed.")
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
			return // We don't know who this is yet, wait for a SpeakingUpdate.
		}

		pr, pw := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())

		oggWriter, err := oggwriter.NewWith(pw, 48000, 2)
		if err != nil {
			log.Printf("Failed to create Ogg writer for SSRC %d: %v", p.SSRC, err)
			cancel()
			pw.CloseWithError(err)
			return
		}

		user, err := s.User(userID)
		if err != nil {
			user = &discordgo.User{Username: "Unknown User", ID: userID}
		}

		startTime := time.Now()
		msg, err := s.ChannelMessageSend(voiceChannelID, fmt.Sprintf("`%s` started speaking at `%s`", user.Username, startTime.Format("15:04:05 MST")))
		if err != nil {
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

		go transcribeStream(ctx, s, pr, stream)
	}

	stream.LastPacket = time.Now()
	rtpPacket := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    0x78,
			SequenceNumber: p.Sequence,
			Timestamp:      p.Timestamp,
			SSRC:           p.SSRC,
		},
		Payload: p.Opus,
	}
	if err := stream.OggWriter.WriteRTP(rtpPacket); err != nil {
		log.Printf("Error writing RTP packet for SSRC %d: %v", p.SSRC, err)
	}
}

func checkStreamTimeouts(state *guild.GuildState) {
	state.Mutex.Lock()
	defer state.Mutex.Unlock()

	for ssrc, stream := range state.ActiveStreams {
		if time.Since(stream.LastPacket) > 2*time.Second {
			log.Printf("User %s timed out. Closing stream.", stream.User.Username)
			stream.Writer.Close()
			stream.CancelFunc()
			delete(state.ActiveStreams, ssrc)
		}
	}
}

func transcribeStream(ctx context.Context, s *discordgo.Session, reader io.Reader, stream *guild.UserStream) {
	transcriptChan := make(chan string)
	errChan := make(chan error, 1)

	go stt.StreamingTranscribe(ctx, reader, transcriptChan, errChan)

	var finalTranscript strings.Builder
	for {
		select {
		case transcript, ok := <-transcriptChan:
			if !ok {
				stopTime := time.Now()
				finalTranscriptStr := strings.TrimSpace(finalTranscript.String())

				if finalTranscriptStr != "" {
					if err := db.LogTranscription(stream.GuildID, stream.ChannelID, stream.User.Username, finalTranscriptStr); err != nil {
						log.Printf("Error logging transcription for user %s: %v", stream.User.Username, err)
					}
				}

				finalContent := fmt.Sprintf("**%s:** %s\n*(`%s` to `%s`)*", stream.User.Username, finalTranscriptStr, stream.StartTime.Format("15:04:05"), stopTime.Format("15:04:05 MST"))
				s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, finalContent)
				return
			}
			finalTranscript.WriteString(transcript)
			interimContent := fmt.Sprintf("`%s:` %s...", stream.User.Username, finalTranscript.String())
			if len(interimContent) > 2000 {
				interimContent = interimContent[:1997] + "..."
			}
			s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, interimContent)

		case err := <-errChan:
			log.Printf("Transcription error for user %s: %v", stream.User.Username, err)
			s.ChannelMessageEdit(stream.Message.ChannelID, stream.Message.ID, fmt.Sprintf("Error during transcription for `%s`.", stream.User.Username))
			return
		case <-ctx.Done():
			return
		}
	}
}
