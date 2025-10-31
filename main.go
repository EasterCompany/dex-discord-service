package main

import (
	"github.com/EasterCompany/dex-discord-interface/app"
	"github.com/EasterCompany/dex-discord-interface/preinit"
)

func main() {
	application, err := app.NewApp()
	if err != nil {
		logger := preinit.NewLogger()
		logger.Fatal("Failed to initialize application", err)
	}
	application.Run()
}
