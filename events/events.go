package events

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/owen/dex-discord-interface/stt"
	"github.com/owen/dex-discord-interface/store"
)

var (
	buffers  = make(map[uint32]*bytes.Buffer)
	mu       sync.Mutex
	speaking = make(map[uint32]bool)
)

// MessageCreate is a handler for the MessageCreate event
func MessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	switch {
	case strings.HasPrefix(m.Content, "!join"):
		joinVoice(s, m)
	case strings.HasPrefix(m.Content, "!leave"):
		leaveVoice(s, m)
	default:
		// Check if the message is a direct message
		if m.GuildID == "" {
			err := store.SaveDirectMessage(m.Author.ID, m.Message)
			if err != nil {
				log.Println("error saving direct message:", err)
			}
			return
		}

		// Get the channel information
		channel, err := s.Channel(m.ChannelID)
		if err != nil {
			log.Println("error getting channel info:", err)
			return
		}

		// Get the server information
		server, err := s.Guild(m.GuildID)
		if err != nil {
			log.Println("error getting server info:", err)
			return
		}

		if err := store.SaveServerMessage(server.Name, channel.Name, m.Message); err != nil {
			log.Println("error saving server message:", err)
		}
	}
}

func joinVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Find the channel that the user is in
	g, err := s.State.Guild(m.GuildID)
	if err != nil {
		log.Println("error getting guild:", err)
		return
	}

	for _, vs := range g.VoiceStates {
		if vs.UserID == m.Author.ID {
			vc, err := s.ChannelVoiceJoin(m.GuildID, vs.ChannelID, false, true)
			if err != nil {
				log.Println("error joining voice channel:", err)
				return
			}
			go handleVoice(vc)
			return
		}
	}
}

func leaveVoice(s *discordgo.Session, m *discordgo.MessageCreate) {
	vc, ok := s.VoiceConnections[m.GuildID]
	if !ok {
		return
	}

	vc.Disconnect()
}

func handleVoice(vc *discordgo.VoiceConnection) {
	for {
		select {
		case p, ok := <-vc.OpusRecv:
			if !ok {
				return
			}

			mu.Lock()
			if _, ok := buffers[p.SSRC]; !ok {
				buffers[p.SSRC] = new(bytes.Buffer)
			}
			buffers[p.SSRC].Write(p.Opus)

			if !speaking[p.SSRC] {
				speaking[p.SSRC] = true
				go func(ssrc uint32) {
					time.Sleep(1 * time.Second)
					mu.Lock()
					defer mu.Unlock()
					speaking[ssrc] = false
					transcript, err := stt.Transcribe(buffers[ssrc].Bytes())
					if err != nil {
						log.Println("error transcribing:", err)
					} else {
						log.Println("transcript:", transcript)
					}
					buffers[ssrc].Reset()
				}(p.SSRC)
			}
			mu.Unlock()
		}
	}
}
