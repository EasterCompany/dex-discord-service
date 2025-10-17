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

	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/bwmarrin/discordgo"
	"github.com/pion/webrtc/v3/pkg/media/oggwriter"
)

// userStream holds the state for a single user's audio stream.
type userStream struct {
	writer     io.WriteCloser
	cancelFunc context.CancelFunc
	lastPacket time.Time
	message    *discordgo.Message
	user       *discordgo.User
	startTime  time.Time
}

var (
	// activeStreams tracks the audio streams for each user (by SSRC).
	activeStreams = make(map[uint32]*userStream)
	mu            sync.Mutex
)

// MessageCreate handles incoming messages and routes commands.
func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore messages from the bot itself.
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.HasPrefix(m.Content, "!join"):
		joinVoice(s, m)
	case strings.HasPrefix(m.Content, "!leave"):
		leaveVoice(s, m)
	}
}

// joinVoice finds the user's voice channel and joins it.
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
			// Start the voice handling goroutine.
			go handleVoice(s, m.ChannelID, vc)
			return
		}
	}
	s.ChannelMessageSend(m.ChannelID, "You need to be in a voice channel for me to join!")
}

// leaveVoice disconnects the bot from the voice channel.
func leaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	if vc, ok := s.VoiceConnections[m.GuildID]; ok {
		vc.Disconnect()
		mu.Lock()
		// Clean up all active streams when leaving.
		for ssrc, stream := range activeStreams {
			stream.writer.Close()
			stream.cancelFunc()
			delete(activeStreams, ssrc)
		}
		mu.Unlock()
	}
}

// handleVoice listens for Opus packets and user timeouts.
func handleVoice(s *discordgo.Session, textChannelID string, vc *discordgo.VoiceConnection) {
	// A timer to periodically check for users who have stopped speaking.
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
			// Process the incoming audio packet.
			handleAudioPacket(s, textChannelID, p)
		case <-ticker.C:
			// Check for streams that have timed out.
			checkStreamTimeouts()
		case <-vc.Done():
			log.Println("Voice connection closed.")
			return
		}
	}
}

// handleAudioPacket processes a single audio packet from a user.
func handleAudioPacket(s *discordgo.Session, textChannelID string, p *discordgo.Packet) {
	mu.Lock()
	defer mu.Unlock()

	stream, ok := activeStreams[p.SSRC]
	if !ok {
		// If this is a new user, create a new stream.
		pr, pw := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())

		// We need to wrap the raw Opus packets into an Ogg container for Google Speech-to-Text.
		oggWriter, err := oggwriter.NewWith(pw, 48000, 2)
		if err != nil {
			log.Printf("Failed to create Ogg writer for SSRC %d: %v", p.SSRC, err)
			cancel()
			pw.CloseWithError(err)
			return
		}

		// Get user details for the message.
		user, err := s.User(p.UserID)
		if err != nil {
			log.Printf("Could not get user info for ID %s: %v", p.UserID, err)
			user = &discordgo.User{Username: "Unknown User", ID: p.UserID}
		}

		// Post the initial message.
		startTime := time.Now()
		msg, err := s.ChannelMessageSend(textChannelID, fmt.Sprintf("`%s` started speaking at `%s`", user.Username, startTime.Format("15:04:05 MST")))
		if err != nil {
			log.Printf("Failed to send initial message: %v", err)
			cancel()
			pw.Close()
			return
		}

		stream = &userStream{
			writer:     oggWriter,
			cancelFunc: cancel,
			lastPacket: time.Now(),
			message:    msg,
			user:       user,
			startTime:  startTime,
		}
		activeStreams[p.SSRC] = stream

		// Start the transcription goroutine for this new stream.
		go transcribeStream(ctx, s, pr, stream)
	}

	// Update the last packet time and write the audio data.
	stream.lastPacket = time.Now()
	if _, err := stream.writer.Write(p.Opus); err != nil {
		log.Printf("Error writing Opus data for SSRC %d: %v", p.SSRC, err)
	}
}

// checkStreamTimeouts iterates through active streams and closes any that have gone silent.
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

// transcribeStream handles the speech-to-text for a single audio stream.
func transcribeStream(ctx context.Context, s *discordgo.Session, reader io.Reader, stream *userStream) {
	transcriptChan := make(chan string)
	errChan := make(chan error, 1)

	// Start the streaming transcription with the Google STT API.
	go stt.StreamingTranscribe(ctx, reader, transcriptChan, errChan)

	var finalTranscript strings.Builder
	for {
		select {
		case transcript, ok := <-transcriptChan:
			if !ok {
				// The channel is closed, meaning the user stopped talking.
				stopTime := time.Now()
				finalContent := fmt.Sprintf("**%s:** %s\n*(`%s` to `%s`)*", stream.user.Username, finalTranscript.String(), stream.startTime.Format("15:04:05"), stopTime.Format("15:04:05 MST"))
				_, err := s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, finalContent)
				if err != nil {
					log.Printf("Error editing final message: %v", err)
				}
				return
			}
			// This is an interim or final transcript part.
			finalTranscript.WriteString(transcript)
			// Edit the message to show the live transcript.
			interimContent := fmt.Sprintf("`%s:` %s...", stream.user.Username, finalTranscript.String())
			if len(interimContent) > 2000 {
				interimContent = interimContent[:1997] + "..."
			}
			_, err := s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, interimContent)
			if err != nil {
				// Don't log rate limit errors, as they are expected during live transcription.
				if !strings.Contains(err.Error(), "429") {
					log.Printf("Error editing interim message: %v", err)
				}
			}

		case err := <-errChan:
			log.Printf("Transcription error for user %s: %v", stream.user.Username, err)
			_, editErr := s.ChannelMessageEdit(stream.message.ChannelID, stream.message.ID, fmt.Sprintf("Error during transcription for `%s`.", stream.user.Username))
			if editErr != nil {
				log.Printf("Error editing error message: %v", editErr)
			}
			return

		case <-ctx.Done():
			log.Printf("Context cancelled for user %s transcription.", stream.user.Username)
			return
		}
	}
}
