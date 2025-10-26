package main

import (
	"log"

	"github.com/EasterCompany/dex-discord-interface/app"
)

func main() {
	application, err := app.NewApp()
	if err != nil {
		log.Fatalf("Failed to initialize application: %v", err)
	}
	application.Run()
}
