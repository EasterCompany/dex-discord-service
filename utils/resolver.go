package utils

import (
	"fmt"

	"github.com/EasterCompany/dex-discord-service/config"
)

// ResolveService finds a service in the service map by its ID
func ResolveService(id string) (*config.ServiceEntry, error) {
	sm, err := config.LoadServiceMap()
	if err != nil {
		return nil, err
	}

	for _, services := range sm.Services {
		for _, s := range services {
			if s.ID == id {
				return &s, nil
			}
		}
	}

	return nil, fmt.Errorf("service %s not found", id)
}

// ChunkString splits a string into chunks of a maximum size, safely handling runes.
func ChunkString(s string, chunkSize int) []string {
	if len(s) == 0 {
		return []string{""}
	}
	if len(s) <= chunkSize {
		return []string{s}
	}
	var chunks []string
	runes := []rune(s)

	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}
