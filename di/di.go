// Package di provides a dependency injection container for the application.
package di

import (
	"fmt"

	"github.com/EasterCompany/dex-discord-interface/cache"
	"github.com/EasterCompany/dex-discord-interface/config"
	"github.com/EasterCompany/dex-discord-interface/events"
	"github.com/EasterCompany/dex-discord-interface/interfaces"
	"github.com/EasterCompany/dex-discord-interface/llm"
	logger "github.com/EasterCompany/dex-discord-interface/log"
	"github.com/EasterCompany/dex-discord-interface/session"
	"github.com/EasterCompany/dex-discord-interface/stt"
	"github.com/bwmarrin/discordgo"
)

// Container holds all the dependencies for the application.
type Container struct {
	Config       *config.AllConfig
	Session      *discordgo.Session
	Logger       logger.Logger
	LocalCache   cache.Cache
	CloudCache   cache.Cache
	STTClient    interfaces.SpeechToText
	LLMClient    *llm.Client
	StateManager *events.StateManager
	UserManager  *events.UserManager
}

// NewContainer creates a new dependency injection container.
func NewContainer() (*Container, error) {
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
	userManager := events.NewUserManager()

	return &Container{
		Config:       cfg,
		Session:      s,
		Logger:       appLogger,
		LocalCache:   localCache,
		CloudCache:   cloudCache,
		STTClient:    sttClient,
		LLMClient:    llmClient,
		StateManager: stateManager,
		UserManager:  userManager,
	}, nil
}
