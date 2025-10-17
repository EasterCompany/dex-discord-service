package stt

import (
	"context"

	"cloud.google.com/go/speech/apiv1"
	speechpb "google.golang.org/genproto/googleapis/cloud/speech/v1"
	"google.golang.org/api/option"
)

// Transcribe transcribes the given audio data
func Transcribe(data []byte) (string, error) {
	ctx := context.Background()

	// Creates a client with credentials.
	// TODO: handle credentials properly
	client, err := speech.NewClient(ctx, option.WithAPIKey("YOUR_API_KEY"))
	if err != nil {
		return "", err
	}

	req := &speechpb.RecognizeRequest{
		Config: &speechpb.RecognitionConfig{
			Encoding:        speechpb.RecognitionConfig_OGG_OPUS,
			SampleRateHertz: 48000,
			LanguageCode:    "en-US",
		},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: data},
		},
	}

	resp, err := client.Recognize(ctx, req)
	if err != nil {
		return "", err
	}

	// TODO: Handle multiple results
	if len(resp.Results) > 0 {
		return resp.Results[0].Alternatives[0].Transcript, nil
	}

	return "", nil
}