// eastercompany/dex-discord-interface/events/events.go
package events

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/EasterCompany/dex-discord-interface/store"
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// userStream holds the state for a single user's audio stream.
type userStream struct {
	writer     io.Closer
	oggWriter  *oggwriter.OggWriter
	cancelFunc context.CancelFunc
	lastPacket time.Time
	message    *discordgo.Message
	user       *discordgo.User
	startTime  time.Time
	guildID    string
	channelID  string
}

var (
	activeStreams    = make(map[uint32]*userStream)
	ssrcUserMap      = make(map[uint32]string)
	mu               sync.Mutex
	ssrcUserMapMutex sync.Mutex
)

// SpeakingUpdate is triggered when a user starts or stops speaking.
func SpeakingUpdate(s *discordgo.Session, p *discordgo.SpeakingUpdate) {
	ssrcUserMapMutex.Lock()
	defer ssrcUserMapMutex.Unlock()
	// Map the SSRC to the UserID when they start speaking.
	if p.Speaking {
		ssrcUserMap[uint32(p.SSRC)] = p.UserID
	}
}

// MessageCreate handles incoming messages, routes commands, and logs messages.
func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Log all messages from guilds
	if m.GuildID != "" {
		if err := store.SaveMessage(m.GuildID, m.ChannelID, m.Message); err != nil {
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

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		log.Printf("Error getting guild: %v", err)
		return
	}

	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, true)
			if err != nil {
				log.Printf("Error joining voice channel: %v", err)
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
		mu.Lock()
		for ssrc, stream := range activeStreams {
			stream.writer.Close()
			stream.cancelFunc()
			delete(activeStreams, ssrc)
		}
		mu.Unlock()
	}
}

func handleVoice(s *discordgo.Session, guildID, voiceChannelID string, vc *discordgo.VoiceConnection) {
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
			handleAudioPacket(s, guildID, voiceChannelID, p)
		case <-ticker.C:
			checkStreamTimeouts()
		}
	}
}

func handleAudioPacket(s *discordgo.Session, guildID, voiceChannelID string, p *discordgo.Packet) {
	mu.Lock()
	defer mu.Unlock()

	stream, ok := activeStreams[p.SSRC]
	if !ok {
		ssrcUserMapMutex.Lock()
		userID, userOk := ssrcUserMap[p.SSRC]
		ssrcUserMapMutex.Unlock()

		if !userOk {
			return // Don't log, we don't know who this is yet.
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
		// A voice channel is its own channel, so we use its ID for messages.
		msg, err := s.ChannelMessageSend(voiceChannelID, fmt.Sprintf("`%s` started speaking at `%s`", user.Username, startTime.Format("15:04:05 MST")))
		if err != nil {
			cancel()
			pw.Close()
			return
		}

		stream = &userStream{
			writer:     oggWriter,
			oggWriter:  oggWriter,
			cancelFunc: cancel,
			lastPacket: time.Now(),
			message:    msg,
			user:       user,
			startTime:  startTime,
			guildID:    guildID,
			channelID:  voiceChannelID, // Use the voice channel ID for logging.
		}
		activeStreams[p.SSRC] = stream

		go transcribeStream(ctx, s, pr, stream)
	}

	stream.lastPacket = time.Now()
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
	if err := stream.oggWriter.WriteRTP(rtpPacket); err != nil {
		log.Printf("Error writing RTP packet for SSRC %d: %v", p.SSRC, err)
	}
}

func checkStreamTimeouts() {
	mu.Lock()
	defer mu.Unlock()

	for ssrc, stream := range activeStreams {
		if time.Since(stream.lastPacket) > 2*time.Second {
			log.Printf("User with SSRC %d timed out. Closing stream.", ssrc)
			stream.writer.Close()
			stream.cancelFunc()
			delete(activeStreams, ssrc)
		}
	}
}

func transcribeStream(ctx context.Context, s *discordgo.Session, reader io.Reader, stream *userStream) {
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
					if err := store.LogTranscription(stream.guildID, stream.channelID, stream.user.Username, finalTranscriptStr); err != nil {
						log.Printf("Error logging transcription for user %s: %v", stream.user.Username, err)
					}
				}

				finalContent := fmt.Sprintf("**%s:** %s\n*(`%s` to `%s`)*", stream.user.Username, finalTranscriptStr, stream.startTime.Format("15:04:05"), stopTime.Format("15:04:05 MST"))
				s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, finalContent)
				return
			}
			finalTranscript.WriteString(transcript)
			interimContent := fmt.Sprintf("`%s:` %s...", stream.user.Username, finalTranscript.String())
			if len(interimContent) > 2000 {
				interimContent = interimContent[:1997] + "..."
			}
			s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, interimContent)

		case err := <-errChan:
			log.Printf("Transcription error for user %s: %v", stream.user.Username, err)
			s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, fmt.Sprintf("Error during transcription for `%s`.", stream.user.Username))
			return
		case <-ctx.Done():
			return
		}
	}
}
