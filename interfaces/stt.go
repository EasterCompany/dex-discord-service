// eastercompany/dex-discord-interface/interfaces/stt.go
package interfaces

// SpeechToText is the interface for the speech-to-text module
type SpeechToText interface {
	Transcribe(audioData []byte) (string, error)
}