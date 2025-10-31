package services

import "log"

// StubSTTService is a mock implementation of the STTService.
type StubSTTService struct{}

// Transcribe returns a canned transcription.
func (s *StubSTTService) Transcribe(audio []byte) (string, error) {
	log.Println("[STUB_STT] Transcribe called")
	return "[transcribed audio]", nil
}

// StubLLMService is a mock implementation of the LLMService.
type StubLLMService struct{}

// Process returns a canned response.
func (s *StubLLMService) Process(context, message string) (string, error) {
	log.Printf("[STUB_LLM] Process called with context: %s, message: %s", context, message)
	return "[mock llm response]", nil
}

// StubTTSService is a mock implementation of the TTSService.
type StubTTSService struct{}

// Synthesize returns empty audio data.
func (s *StubTTSService) Synthesize(text string) ([]byte, error) {
	log.Printf("[STUB_TTS] Synthesize called with text: %s", text)
	return []byte{}, nil
}
