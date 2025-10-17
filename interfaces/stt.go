// eastercompany/dex-discord-interface/interfaces/stt.go
package interfaces

import (
	"context"
	"io"
)

// STT is the interface for the speech-to-text module
type STT interface {
	StreamingTranscribe(ctx context.Context, reader io.Reader, transcriptChan chan<- string, errChan chan<- error)
}
