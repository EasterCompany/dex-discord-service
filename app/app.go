package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/constants"
	"github.com/EasterCompany/dex-discord-interface/di"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/reporting"
	"github.com/EasterCompany/dex-discord-interface/startup"
	"github.com/bwmarrin/discordgo"
)

type App struct {
	Container *di.Container
}

func NewApp() (*App, error) {
	container, err := di.NewContainer()
	if err != nil {
		return nil, err
	}
	return &App{Container: container}, nil
}

func (a *App) Run() {
	c := a.Container
	eventHandler := events.NewHandler(c.LocalCache, c.Config.Discord, c.Config.Bot, c.Session, c.Logger, c.StateManager, c.UserManager, c.STTClient, c.LLMClient)
	voiceHandler := events.NewVoiceHandler(c.Session, c.Logger, c.StateManager, c.LocalCache, c.Config.Bot, c.Config.Discord, c.STTClient)
	commandHandler := events.NewCommandHandler(c.Session, c.Logger, voiceHandler)
	messageHandler := events.NewMessageHandler(c.LocalCache, c.LLMClient, c.UserManager, c.Logger, c.Session, commandHandler)

	c.Session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		count, size := eventHandler.Ready(s, r)
		c.StateManager.SetAddedMessagesStats(count, size)
	})
	c.Session.AddHandler(messageHandler.Handle)
	c.Session.AddHandler(eventHandler.ChannelCreate)

	if err := c.Session.Open(); err != nil {
		c.Logger.Fatal("Error opening connection to Discord", err)
	}

	bootMessage := reporting.NewBootMessage(c.Logger)
	bootMessage.PostInitialMessage()

	bootMessage.Update(constants.BootMessageInit)

	cleanupReport, audioCleanResult, messageCleanResult := startup.PerformCleanup(c.Session, c.LocalCache, c.Config.Discord, bootMessage.MessageID, c.Logger)
	bootMessage.Update(constants.BootMessageCleanup)

	startup.LoadGuildStates(c.LocalCache, c.StateManager, c.Logger)
	startup.CacheAllDMs(c.Session, c.LocalCache, c.Logger)
	bootMessage.Update(constants.BootMessageGuildsLoaded)

	reporting.PostFinalStatus(c.Session, c.LocalCache, c.CloudCache, c.Config, bootMessage.MessageID, cleanupReport, audioCleanResult, messageCleanResult, c.STTClient, c.StateManager, c.Logger, c.LLMClient.SystemPrompt)

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	_ = c.Session.Close()
	fmt.Println("Bot shutting down.")
}
