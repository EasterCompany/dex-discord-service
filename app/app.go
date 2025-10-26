package app

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/reporting"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/startup"
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/bwmarrin/discordgo"
)

type App struct {
	Config       *config.AllConfig
	Session      *discordgo.Session
	Logger       logger.Logger
	LocalCache   cache.Cache
	CloudCache   cache.Cache
	STTClient    interfaces.SpeechToText
	LLMClient    *llm.Client
	StateManager *events.StateManager
}

func NewApp() (*App, error) {
	cfg, _, err := config.LoadAllConfigs()
	if err != nil {
		return nil, fmt.Errorf("fatal error loading config: %w", err)
	}

	s, err := session.NewSession(cfg.Discord.Token)
	if err != nil {
		return nil, fmt.Errorf("error creating Discord session: %w", err)
	}

	appLogger := logger.NewLogger(s, cfg.Discord.LogChannelID)

	localCache, err := cache.New(cfg.Cache.Local)
	if err != nil {
		appLogger.Error("Failed to initialize local cache", err)
	}
	cloudCache, _ := cache.New(cfg.Cache.Cloud)

	sttClient, err := stt.NewClient()
	if err != nil {
		appLogger.Error("Failed to initialize STT client", err)
	}

	llmClient, err := llm.NewClient(cfg.Persona, cfg.Bot, localCache)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize LLM client: %w", err)
	}

	stateManager := events.NewStateManager()

	return &App{
		Config:       cfg,
		Session:      s,
		Logger:       appLogger,
		LocalCache:   localCache,
		CloudCache:   cloudCache,
		STTClient:    sttClient,
		LLMClient:    llmClient,
		StateManager: stateManager,
	}, nil
}

func (a *App) Run() {
	eventHandler := events.NewHandler(a.LocalCache, a.Config.Discord, a.Config.Bot, a.Session, a.Logger, a.StateManager, a.STTClient, a.LLMClient)

	a.Session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		count, size := eventHandler.Ready(s, r)
		a.StateManager.SetAddedMessagesStats(count, size)
	})
	a.Session.AddHandler(eventHandler.MessageCreate)
	a.Session.AddHandler(eventHandler.ChannelCreate)

	if err := a.Session.Open(); err != nil {
		a.Logger.Fatal("Error opening connection to Discord", err)
	}

	bootMessage, err := a.Logger.PostInitialMessage("Dexter is starting up...")
	if err != nil {
		a.Logger.Error("Failed to post initial boot message", err)
	}
	bootMessageID := ""
	if bootMessage != nil {
		bootMessageID = bootMessage.ID
	}

	updateBootMessage := func(content string) {
		if bootMessage != nil {
			a.Logger.UpdateInitialMessage(bootMessageID, content)
		}
	}

	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ LLM client initialized`)

	cleanupReport, audioCleanResult, messageCleanResult := startup.PerformCleanup(a.Session, a.LocalCache, a.Config.Discord, bootMessageID, a.Logger)
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ LLM client initialized
✅ Cleanup complete`)

	startup.LoadGuildStates(a.LocalCache, a.StateManager, a.Logger)
	startup.CacheAllDMs(a.Session, a.LocalCache, a.Logger)
	updateBootMessage(`Dexter is starting up...
✅ Discord connection established
✅ Caches initialized
✅ STT client initialized
✅ LLM client initialized
✅ Cleanup complete
✅ Guild states loaded`)

	reporting.PostFinalStatus(a.Session, a.LocalCache, a.CloudCache, a.Config, bootMessageID, cleanupReport, audioCleanResult, messageCleanResult, a.STTClient, a.StateManager, a.Logger, a.LLMClient.SystemPrompt)

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	_ = a.Session.Close()
	fmt.Println("Bot shutting down.")
}
