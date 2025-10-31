package services

// STTService defines the interface for a Speech-to-Text service.
type STTService interface {
	Transcribe(audio []byte) (string, error)
}

// LLMService defines the interface for a Large Language Model service.
type LLMService interface {
	Process(context, message string) (string, error)
}

// TTSService defines the interface for a Text-to-Speech service.
type TTSService interface {
	Synthesize(text string) ([]byte, error)
}
