// eastercompany/dex-discord-interface/stt/stt.go
package stt

import (
	"context"
	"fmt"
	"io"
	"sync"

	speech "cloud.google.com/go/speech/apiv1"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// STT is the speech-to-text client
type STT struct {
	speechClient *speech.Client
}

// bufferPool is used to reuse byte slices to reduce memory allocations.
var bufferPool = sync.Pool{
	New: func() interface{} {
		// Create a buffer of 1024 bytes, a common size for audio chunks.
		return make([]byte, 1024)
	},
}

// New creates a new Google Cloud Speech client.
// It relies on Application Default Credentials for authentication.
func New() (*STT, error) {
	ctx := context.Background()
	// Create a client without the API key. It will use ADC.
	speechClient, err := speech.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create speech client: %w", err)
	}
	return &STT{speechClient: speechClient}, nil
}

// Close cleans up the speech client connection.
func (s *STT) Close() {
	if s.speechClient != nil {
		s.speechClient.Close()
	}
}

// StreamingTranscribe processes an audio stream and sends transcripts through a channel.
func (s *STT) StreamingTranscribe(ctx context.Context, reader io.Reader, transcriptChan chan<- string, errChan chan<- error) {
	stream, err := s.speechClient.StreamingRecognize(ctx)
	if err != nil {
		errChan <- fmt.Errorf("could not start streaming recognize: %w", err)
		return
	}

	// Send initial configuration
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:        speechpb.RecognitionConfig_OGG_OPUS,
					SampleRateHertz: 48000,
					LanguageCode:    "en-US",
				},
				InterimResults: true,
			},
		},
	}); err != nil {
		errChan <- fmt.Errorf("could not send streaming config: %w", err)
		return
	}

	// Goroutine to stream audio content from the reader
	go func() {
		for {
			// Get a buffer from the pool.
			buf := bufferPool.Get().([]byte)

			n, err := reader.Read(buf)
			if err != nil {
				// Ensure the buffer is returned to the pool on any error, including io.EOF.
				bufferPool.Put(buf)
				if err == io.EOF {
					// Cleanly close the stream to signal the end of audio.
					if closeErr := stream.CloseSend(); closeErr != nil {
						errChan <- fmt.Errorf("failed to close send stream: %w", closeErr)
					}
					return
				}
				errChan <- fmt.Errorf("error reading from audio pipe: %w", err)
				return
			}

			if n > 0 {
				if sendErr := stream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: buf[:n],
					},
				}); sendErr != nil {
					errChan <- fmt.Errorf("could not send audio content: %w", sendErr)
				}
			}

			// Return the buffer to the pool for reuse.
			bufferPool.Put(buf)
		}
	}()

	// Goroutine to receive and process transcripts
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			errChan <- fmt.Errorf("cannot stream results: %w", err)
			return
		}
		if len(resp.Results) > 0 {
			result := resp.Results[0]
			if len(result.Alternatives) > 0 {
				transcriptChan <- result.Alternatives[0].Transcript
			}
		}
	}
	close(transcriptChan)
}
