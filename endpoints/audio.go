package endpoints

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"

	"github.com/EasterCompany/dex-discord-service/audio"
	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

var (
	activeVoiceConnection *discordgo.VoiceConnection
	vcMutex               sync.Mutex
	isSpeaking            bool
	speakingMutex         sync.Mutex
)

// SetActiveVoiceConnection sets the active voice connection for audio playback
func SetActiveVoiceConnection(vc *discordgo.VoiceConnection) {
	vcMutex.Lock()
	defer vcMutex.Unlock()
	activeVoiceConnection = vc
}

// AudioHandler handles requests for audio files from Redis (GET)
func AudioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract Redis key from the URL path
	redisKey := strings.TrimPrefix(r.URL.Path, "/audio/")
	if redisKey == "" {
		http.Error(w, "Missing audio key", http.StatusBadRequest)
		return
	}

	// Get the voice recorder instance
	// NOTE: This assumes audio package has a singleton or we fix dependency injection later.
	// For now, we assume this handler is for debugging recorded audio and might be flaky without DI fix.
	voiceRecorder, err := audio.NewVoiceRecorder(context.Background(), nil, nil)
	if err != nil {
		log.Printf("Error creating voice recorder: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Get audio data from Redis
	audioData, err := voiceRecorder.GetRedis().Get(context.Background(), redisKey).Bytes()
	if err != nil {
		log.Printf("Error getting audio from Redis: %v", err)
		http.Error(w, "Audio not found", http.StatusNotFound)
		return
	}

	// Set headers and write audio data to the response
	w.Header().Set("Content-Type", "audio/wav")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(audioData); err != nil {
		log.Printf("Error writing audio data to response: %v", err)
	}
}

// PlayAudioHandler handles requests to play audio on the active voice connection (POST)
func PlayAudioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	vcMutex.Lock()
	vc := activeVoiceConnection
	vcMutex.Unlock()

	if vc == nil {
		http.Error(w, "No active voice connection", http.StatusServiceUnavailable)
		return
	}

	speakingMutex.Lock()
	if isSpeaking {
		speakingMutex.Unlock()
		http.Error(w, "Bot is already speaking", http.StatusConflict)
		return
	}
	isSpeaking = true
	speakingMutex.Unlock()

	defer func() {
		speakingMutex.Lock()
		isSpeaking = false
		speakingMutex.Unlock()
	}()

	// Stream audio from request body to ffmpeg to opus
	// We use ffmpeg to convert whatever input (likely WAV) to 48kHz stereo PCM (s16le) for Discord

	ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	ffmpeg.Stdin = r.Body

	ffmpegOut, err := ffmpeg.StdoutPipe()
	if err != nil {
		log.Printf("Error creating ffmpeg stdout pipe: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if err := ffmpeg.Start(); err != nil {
		log.Printf("Error starting ffmpeg: %v", err)
		http.Error(w, "ffmpeg error", http.StatusInternalServerError)
		return
	}

	defer func() { _ = ffmpeg.Wait() }()

	// Send speaking signal
	_ = vc.Speaking(true)
	defer func() { _ = vc.Speaking(false) }()

	// Buffered reader for ffmpeg output
	ffBuf := bufio.NewReaderSize(ffmpegOut, 16384)

	// 20ms frame size for 48kHz stereo (2 channels * 2 bytes/sample * 48000 samples/sec * 0.02s = 1920 bytes)
	// Actually, Discord expects 960 samples per frame per channel.
	// 960 samples * 2 channels * 2 bytes = 3840 bytes?
	// Wait, typical opus frame is 20ms.
	// 48000 * 0.02 = 960 samples.
	// Stereo: 960 * 2 = 1920 samples total? No, 960 samples per channel.
	// 1920 total samples * 2 bytes/sample (int16) = 3840 bytes.

	frameSize := 960
	channels := 2
	frameBytes := frameSize * channels * 2

	opusEncoder, err := gopus.NewEncoder(48000, channels, gopus.Voip)
	if err != nil {
		log.Printf("Error creating opus encoder: %v", err)
		http.Error(w, "Opus error", http.StatusInternalServerError)
		return
	}

	for {
		// Read PCM data
		pcmBuf := make([]int16, frameSize*channels)
		err := binary.Read(ffBuf, binary.LittleEndian, &pcmBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			log.Printf("Error reading from ffmpeg: %v", err)
			break
		}

		// Encode to Opus
		opus, err := opusEncoder.Encode(pcmBuf, frameSize, frameBytes)
		if err != nil {
			log.Printf("Error encoding opus: %v", err)
			break
		}

		// Send to Discord
		if vc.OpusSend == nil {
			// Reconnection might have closed it
			break
		}
		vc.OpusSend <- opus
	}

	w.WriteHeader(http.StatusOK)
}
