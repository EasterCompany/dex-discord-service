package endpoints

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/EasterCompany/dex-discord-service/audio"
	"github.com/bwmarrin/discordgo"
)

var (
	vcMutex  sync.Mutex
	activeVC *discordgo.VoiceConnection
)

// GetActiveVoiceConnection returns the current voice connection safely
func GetActiveVoiceConnection() *discordgo.VoiceConnection {
	vcMutex.Lock()
	defer vcMutex.Unlock()
	return activeVC
}

// SetActiveVoiceConnection sets the active voice connection for audio playback
func SetActiveVoiceConnection(vc *discordgo.VoiceConnection) {
	vcMutex.Lock()
	activeVC = vc
	vcMutex.Unlock()

	// Initialize and start the global mixer
	mixer, err := audio.NewAudioMixer(vc)
	if err != nil {
		log.Printf("Failed to create audio mixer: %v", err)
		return
	}
	audio.SetGlobalMixer(mixer)
	mixer.Start()
	log.Println("Audio mixer started")
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

// PlayMusicHandler handles requests to play music from a URL (e.g., YouTube)
func PlayMusicHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	mixer := audio.GetGlobalMixer()
	if mixer == nil {
		http.Error(w, "No active audio mixer", http.StatusServiceUnavailable)
		return
	}

	// Launch async so we don't block the HTTP response
	go func() {
		log.Printf("Starting playback for URL: %s", req.URL)

		// 1. Start yt-dlp to stream audio to stdout
		ytDlp := exec.Command("yt-dlp", "-f", "bestaudio", "-o", "-", req.URL)
		ytOut, err := ytDlp.StdoutPipe()
		if err != nil {
			log.Printf("Error creating yt-dlp stdout pipe: %v", err)
			return
		}

		if err := ytDlp.Start(); err != nil {
			log.Printf("Error starting yt-dlp: %v", err)
			return
		}
		defer func() { _ = ytDlp.Wait() }()

		// 2. Start ffmpeg to convert stdin (from yt-dlp) to PCM s16le 48kHz stereo
		ffmpeg := exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
		ffmpeg.Stdin = ytOut

		ffmpegOut, err := ffmpeg.StdoutPipe()
		if err != nil {
			log.Printf("Error creating ffmpeg stdout pipe: %v", err)
			return
		}

		if err := ffmpeg.Start(); err != nil {
			log.Printf("Error starting ffmpeg: %v", err)
			return
		}
		defer func() { _ = ffmpeg.Wait() }()

		// 3. Stream to Mixer (isVoice = false)
		// This will block until stream ends
		if err := mixer.StreamFromReader(context.Background(), ffmpegOut, false); err != nil {
			log.Printf("Error streaming music to mixer: %v", err)
		}

		log.Printf("Finished playback for URL: %s", req.URL)
	}()

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"playing"}`))
}

// PlayAudioHandler handles requests to play audio on the active voice connection (POST)
func PlayAudioHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mixer := audio.GetGlobalMixer()
	if mixer == nil {
		http.Error(w, "No active audio mixer", http.StatusServiceUnavailable)
		return
	}

	// Check if we are streaming from a file
	filePath := r.URL.Query().Get("file_path")
	if filePath == "" {
		filePath = r.Header.Get("X-File-Path")
	}

	var ffmpeg *exec.Cmd
	if filePath != "" {
		// Validate file exists
		if _, err := os.Stat(filePath); err != nil {
			log.Printf("PlayAudioHandler: File not found: %s", filePath)
			http.Error(w, "File not found", http.StatusBadRequest)
			return
		}
		// Stream from file
		ffmpeg = exec.Command("ffmpeg", "-i", filePath, "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
	} else {
		// Stream from request body
		ffmpeg = exec.Command("ffmpeg", "-i", "pipe:0", "-f", "s16le", "-ar", "48000", "-ac", "2", "pipe:1")
		ffmpeg.Stdin = r.Body
	}

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

	defer func() {
		_ = ffmpeg.Wait()
		// Clean up temp file if used and it looks like a temp file
		if filePath != "" && strings.Contains(filePath, "tmp") {
			_ = os.Remove(filePath)
		}
	}()

	// Stream to Mixer (isVoice = true)
	// We use the mixer's voice context to allow interruption
	if err := mixer.StreamFromReader(mixer.GetVoiceContext(), ffmpegOut, true); err != nil {
		if err == context.Canceled {
			log.Println("PlayAudioHandler: Voice playback interrupted (Barge-In).")
		} else {
			log.Printf("Error streaming voice to mixer: %v", err)
		}
	}

	w.WriteHeader(http.StatusOK)
}
