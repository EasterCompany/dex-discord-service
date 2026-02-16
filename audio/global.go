package audio

import "sync"

var (
	globalRecorder   *VoiceRecorder
	recorderMu       sync.Mutex
)

// GetGlobalRecorder returns the singleton voice recorder instance.
func GetGlobalRecorder() *VoiceRecorder {
	recorderMu.Lock()
	defer recorderMu.Unlock()
	return globalRecorder
}

// SetGlobalRecorder sets the singleton voice recorder instance.
func SetGlobalRecorder(vr *VoiceRecorder) {
	recorderMu.Lock()
	defer recorderMu.Unlock()
	globalRecorder = vr
}
