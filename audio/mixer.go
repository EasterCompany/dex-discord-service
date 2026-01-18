package audio

import (
	"context"
	"encoding/binary"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

const (
	FrameSize    = 960 // 20ms at 48kHz
	Channels     = 2   // Stereo
	SampleRate   = 48000
	FrameBytes   = FrameSize * Channels * 2 // 16-bit
	MusicDucking = 0.2                      // Volume multiplier for music when voice is active
)

// AudioMixer manages mixing of music and voice streams
type AudioMixer struct {
	vc          *discordgo.VoiceConnection
	musicStream chan []int16
	voiceStream chan []int16
	stopChan    chan struct{}
	running     bool
	playing     atomic.Bool // Tracks if mixer is actively outputting audio
	mu          sync.Mutex
	encoder     *gopus.Encoder

	// Voice Interruption Control
	voiceCtx    context.Context
	voiceCancel context.CancelFunc
}

var globalMixer *AudioMixer
var mixerMu sync.Mutex

// GetGlobalMixer returns the singleton mixer instance
func GetGlobalMixer() *AudioMixer {
	mixerMu.Lock()
	defer mixerMu.Unlock()
	return globalMixer
}

// SetGlobalMixer sets the singleton mixer instance
func SetGlobalMixer(m *AudioMixer) {
	mixerMu.Lock()
	defer mixerMu.Unlock()
	if globalMixer != nil {
		globalMixer.Stop()
	}
	globalMixer = m
}

// NewAudioMixer creates a new mixer for the given voice connection
func NewAudioMixer(vc *discordgo.VoiceConnection) (*AudioMixer, error) {
	encoder, err := gopus.NewEncoder(SampleRate, Channels, gopus.Voip)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &AudioMixer{
		vc:          vc,
		musicStream: make(chan []int16, 100), // Buffer ~2 seconds
		voiceStream: make(chan []int16, 100),
		stopChan:    make(chan struct{}),
		encoder:     encoder,
		voiceCtx:    ctx,
		voiceCancel: cancel,
	}, nil
}

// InterruptVoice stops the current voice playback and clears the queue
func (m *AudioMixer) InterruptVoice() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel current streaming context
	if m.voiceCancel != nil {
		m.voiceCancel()
	}

	// Reset context for next stream
	m.voiceCtx, m.voiceCancel = context.WithCancel(context.Background())

	// Drain channel
loop:
	for {
		select {
		case <-m.voiceStream:
		default:
			break loop
		}
	}
	log.Println("AudioMixer: Voice interrupted and queue cleared.")
}

// GetVoiceContext returns the current interruptible context for voice streaming
func (m *AudioMixer) GetVoiceContext() context.Context {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.voiceCtx
}

// Start begins the mixing loop
func (m *AudioMixer) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	go m.runLoop()
}

// Stop stops the mixing loop
func (m *AudioMixer) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	close(m.stopChan)
	m.mu.Unlock()
}

// StreamMusic adds a PCM frame to the music queue
func (m *AudioMixer) StreamMusic(pcm []int16) {
	if !m.IsRunning() {
		return
	}
	select {
	case m.musicStream <- pcm:
	case <-time.After(1 * time.Second):
		// Drop frame if buffer full (lag)
	}
}

// StreamVoice adds a PCM frame to the voice queue
func (m *AudioMixer) StreamVoice(pcm []int16) {
	if !m.IsRunning() {
		return
	}
	select {
	case m.voiceStream <- pcm:
	case <-time.After(1 * time.Second):
	}
}

func (m *AudioMixer) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// IsPlaying returns true if the mixer is actively outputting audio (voice or music)
func (m *AudioMixer) IsPlaying() bool {
	return m.playing.Load()
}

func (m *AudioMixer) runLoop() {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	var isSpeaking bool
	var silenceFrames int

	// Ensure we stop speaking on exit
	defer func() {
		if isSpeaking {
			_ = m.vc.Speaking(false)
			m.playing.Store(false)
		}
	}()

	for {
		select {
		case <-m.stopChan:
			return
		case <-ticker.C:
			if !m.vc.Ready {
				// Connection not ready, wait or exit?
				// If we exit, we kill the mixer. Better to wait a bit.
				// But if it stays unready, we should probably stop.
				// For now, let's just skip this tick.
				continue
			}

			// Read from music
			var musicFrame []int16
			hasMusic := false
			select {
			case musicFrame = <-m.musicStream:
				hasMusic = true
			default:
			}

			// Read from voice
			var voiceFrame []int16
			hasVoice := false
			select {
			case voiceFrame = <-m.voiceStream:
				hasVoice = true
			default:
			}

			hasAudio := hasMusic || hasVoice

			if hasAudio {
				if !isSpeaking {
					if err := m.vc.Speaking(true); err != nil {
						log.Printf("Mixer Speaking(true) error: %v", err)
					}
					isSpeaking = true
					m.playing.Store(true)
				}
				silenceFrames = 0

				// Mix
				mixed := make([]int16, FrameSize*Channels)

				for i := 0; i < len(mixed); i++ {
					var mSample, vSample float64

					if hasMusic && i < len(musicFrame) {
						mSample = float64(musicFrame[i])
						if hasVoice {
							mSample *= MusicDucking // Duck music
						}
					}

					if hasVoice && i < len(voiceFrame) {
						vSample = float64(voiceFrame[i])
					}

					// Add
					sum := mSample + vSample

					// Clip
					if sum > 32767 {
						sum = 32767
					} else if sum < -32768 {
						sum = -32768
					}

					mixed[i] = int16(sum)
				}

				// Encode
				opus, err := m.encoder.Encode(mixed, FrameSize, FrameBytes)
				if err != nil {
					log.Printf("Mixer encode error: %v", err)
					continue
				}

				// Send
				if m.vc.OpusSend != nil {
					m.vc.OpusSend <- opus
				}

			} else {
				// Silence Logic
				if isSpeaking {
					// Send silence frame to trailing off smoothly
					silenceFrames++

					// Send silence frame (zeros)
					// We need to encode zeros? Or pre-encoded silence?
					// Let's encode a zero buffer.
					zeros := make([]int16, FrameSize*Channels)
					opus, _ := m.encoder.Encode(zeros, FrameSize, FrameBytes)

					if m.vc.OpusSend != nil {
						m.vc.OpusSend <- opus
					}

					if silenceFrames > 5 { // 100ms of silence
						if err := m.vc.Speaking(false); err != nil {
							log.Printf("Mixer Speaking(false) error: %v", err)
						}
						isSpeaking = false
						m.playing.Store(false)
					}
				}
				// If not speaking, do nothing (idle)
			}
		}
	}
}

// StreamFromReader reads PCM s16le stereo 48kHz from a reader and streams it to the specified channel
// isVoice determines if it goes to Voice (true) or Music (false) channel
func (m *AudioMixer) StreamFromReader(ctx context.Context, r io.Reader, isVoice bool) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read a frame
		buf := make([]int16, FrameSize*Channels)
		err := binary.Read(r, binary.LittleEndian, &buf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}

		if isVoice {
			m.StreamVoice(buf)
		} else {
			m.StreamMusic(buf)
		}
	}
	return nil
}
