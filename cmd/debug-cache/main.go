package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/bwmarrin/discordgo"
)

func main() {
	cfg, _, err := config.LoadAllConfigs()
	if err != nil {
		log.Fatalf("Fatal error loading config: %v", err)
	}

	debugCache, err := cache.NewDebug(cfg.Cache.Local)
	if err != nil {
		log.Fatalf("Failed to initialize local cache: %v", err)
	}

	keys, err := debugCache.GetAllKeys()
	if err != nil {
		log.Fatalf("Failed to get keys: %v", err)
	}

	for _, key := range keys {
		fmt.Printf("\n--- Key: %s ---\n", key)
		keyType, err := debugCache.GetType(key)
		if err != nil {
			log.Printf("Failed to get type for key %s: %v", key, err)
			continue
		}
		fmt.Printf("Type: %s\n", keyType)

		switch keyType {
		case "string":
			val, err := debugCache.Get(key)
			if err != nil {
				log.Printf("Failed to get string value for key %s: %v", key, err)
				continue
			}
			fmt.Printf("Value: %s\n", val)
		case "list":
			vals, err := debugCache.LRange(key, 0, -1)
			if err != nil {
				log.Printf("Failed to get list value for key %s: %v", key, err)
				continue
			}
			fmt.Printf("Values:\n")
			for _, val := range vals {
				// Pretty print discordgo.Message
				if strings.Contains(key, ":messages:") {
					var msg discordgo.Message
					if err := json.Unmarshal([]byte(val), &msg); err == nil {
						fmt.Printf("  - %s: %s\n", msg.Author.Username, msg.Content)
					} else {
						fmt.Printf("  - %s\n", val)
					}
				} else {
					fmt.Printf("  - %s\n", val)
				}
			}
		default:
			fmt.Println("Value: (unsupported type for printing)")
		}
	}
}
