// Package stt provides a client for the Google Cloud Speech-to-Text API.
package stt

import (
	"context"
	"fmt"

	speech "cloud.google.com/go/speech/apiv1"
	"cloud.google.com/go/speech/apiv1/speechpb"
)

// Client is a client for the Google Cloud Speech-to-Text API.
type Client struct {
	client *speech.Client
}

// NewClient creates a new speech-to-text client.
func NewClient() (*Client, error) {
	ctx := context.Background()
	client, err := speech.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create speech client: %w", err)
	}
	return &Client{client: client}, nil
}

// Transcribe transcribes the given audio data.
func (c *Client) Transcribe(audioData []byte) (string, error) {
	ctx := context.Background()
	req := &speechpb.RecognizeRequest{
		Config: &speechpb.RecognitionConfig{
			Encoding:          speechpb.RecognitionConfig_OGG_OPUS,
			SampleRateHertz:   48000,
			LanguageCode:      "en-US",
			AudioChannelCount: 2},
		Audio: &speechpb.RecognitionAudio{
			AudioSource: &speechpb.RecognitionAudio_Content{Content: audioData},
		},
	}

	resp, err := c.client.Recognize(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to recognize: %w", err)
	}

	if len(resp.Results) == 0 {
		return "", nil
	}

	return resp.Results[0].Alternatives[0].Transcript, nil
}
