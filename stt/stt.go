// eastercompany/dex-discord-interface/stt/stt.go
package stt

import (
	"context"
	"io"
	"log"

	speech "cloud.google.com/go/speech/apiv1"
	"google.golang.org/api/option"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
)

// StreamingTranscribe connects to Google's Speech-to-Text API and transcribes an audio stream.
func StreamingTranscribe(ctx context.Context, reader io.Reader, transcriptChan chan<- string, errChan chan<- error) {
	// Ensure the channels are closed on function exit to signal completion.
	defer close(transcriptChan)
	defer close(errChan)

	// IMPORTANT: Replace "YOUR_API_KEY" with your actual Google Cloud API key.
	// For production, consider using a service account file.
	client, err := speech.NewClient(ctx, option.WithAPIKey("YOUR_API_KEY"))
	if err != nil {
		errChan <- err
		return
	}
	defer client.Close()

	stream, err := client.StreamingRecognize(ctx)
	if err != nil {
		errChan <- err
		return
	}

	// Send the initial configuration message.
	if err := stream.Send(&speechpb.StreamingRecognizeRequest{
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					Encoding:        speechpb.RecognitionConfig_WEBM_OPUS, // Use WEBM_OPUS for Ogg/Opus
					SampleRateHertz: 48000,
					LanguageCode:    "en-US",
				},
				InterimResults: true, // Get live updates
			},
		},
	}); err != nil {
		errChan <- err
		return
	}

	// Goroutine to read from the audio pipe and send to Google.
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				if sendErr := stream.Send(&speechpb.StreamingRecognizeRequest{
					StreamingRequest: &speechpb.StreamingRecognizeRequest_AudioContent{
						AudioContent: buf[:n],
					},
				}); sendErr != nil {
					log.Printf("Could not send audio content: %v", sendErr)
				}
			}
			if err == io.EOF {
				// The writer has closed the pipe, indicating the user stopped talking.
				if err := stream.CloseSend(); err != nil {
					log.Printf("Failed to close send stream: %v", err)
				}
				return
			}
			if err != nil {
				log.Printf("Error reading from audio pipe: %v", err)
				return
			}
		}
	}()

	// Main loop to receive transcription results from Google.
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			// The stream has finished.
			return
		}
		if err != nil {
			errChan <- err
			return
		}

		if len(resp.Results) > 0 {
			result := resp.Results[0]
			if len(result.Alternatives) > 0 {
				transcript := result.Alternatives[0].Transcript
				if result.IsFinal {
					// Send the final part of the transcript.
					transcriptChan <- transcript
				} else {
					// Send the interim (in-progress) part.
					transcriptChan <- transcript
				}
			}
		}
	}
}

