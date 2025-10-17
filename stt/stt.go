// eastercompany/dex-discord-interface/stt/stt.go
package stt

import (
	"context"
	"fmt"
	"io"
	"log"

	speech "cloud.google.com/go/speech/apiv1"
	"google.golang.org/api/option"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

var (
	apiKey       string
	speechClient *speech.Client
)

// Initialize sets up the speech-to-text client with the provided API key.
func Initialize(key string) error {
	if key == "" {
		return fmt.Errorf("Google Cloud API key is missing")
	}
	apiKey = key

	ctx := context.Background()
	var err error
	speechClient, err = speech.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return fmt.Errorf("failed to create speech client: %w", err)
	}
	return nil
}

// Close cleans up the speech client connection.
func Close() {
	if speechClient != nil {
		speechClient.Close()
	}
}

// StreamingTranscribe performs live speech-to-text on an audio stream.
func StreamingTranscribe(ctx context.Context, reader io.Reader, transcriptChan chan<- string, errChan chan<- error) {
	if speechClient == nil {
		errChan <- fmt.Errorf("STT service not initialized")
		return
	}

	stream, err := speechClient.StreamingRecognize(ctx)
	if err != nil {
		errChan <- err
		return
	}

	// Send the initial configuration message.
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
		errChan <- err
		return
	}

	// Goroutine to continuously read from the audio pipe and send to Google.
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if err == io.EOF {
				stream.CloseSend()
				return
			}
			if err != nil {
				log.Printf("Could not read from audio reader: %v", err)
				stream.CloseSend() // Close the stream on read error
				return
			}
			if err := stream.Send(&speechpb.StreamingRecognizeRequest{
				StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
					AudioContent: buf[:n],
				},
			}); err != nil {
				// Don't report this error if the context was already cancelled.
				if ctx.Err() == nil {
					log.Printf("Could not send audio to STT service: %v", err)
				}
			}
		}
	}()

	// Goroutine to receive transcripts from Google.
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			errChan <- err
			return
		}
		if len(resp.Results) > 0 && len(resp.Results[0].Alternatives) > 0 {
			transcriptChan <- resp.Results[0].Alternatives[0].Transcript
		}
	}
	close(transcriptChan)
}

