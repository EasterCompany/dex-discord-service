package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/owen/dex-discord-interface/events"
	"github.com/owen/dex-discord-interface/session"
)

func main() {
	// Create a new Discord session
	s, err := session.NewSession("MTQyMzEwNTY0NzcyOTMxMTgwNQ.GJTIqd.s4rThpJeaxybNftuXargrrgcFE9oGJI216mAMQ")
	if err != nil {
		fmt.Println("error creating Discord session,", err)
		return
	}

	// Add the message create event handler
	s.AddHandler(events.MessageCreate)

	// Open a websocket connection to Discord and begin listening
	err = s.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Wait here until CTRL-C or other term signal is received
	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc

	// Cleanly close down the Discord session
	s.Close()
}
