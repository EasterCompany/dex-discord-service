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
	commandHandler := events.NewCommandHandler(c.Session, c.Logger, voiceHandler, c.Config.Discord)
	messageHandler := events.NewMessageHandler(c.LocalCache, c.LLMClient, c.UserManager, c.Logger, c.Session, commandHandler)

	// Core event handlers
	c.Session.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		count, size := eventHandler.Ready(s, r)
		c.StateManager.SetAddedMessagesStats(count, size)
	})
	c.Session.AddHandler(messageHandler.Handle)

	// Channel event handlers
	c.Session.AddHandler(eventHandler.ChannelCreate)
	c.Session.AddHandler(eventHandler.ChannelUpdate)
	c.Session.AddHandler(eventHandler.ChannelDelete)
	c.Session.AddHandler(eventHandler.ChannelPinsUpdate)

	// Thread event handlers
	c.Session.AddHandler(eventHandler.ThreadCreate)
	c.Session.AddHandler(eventHandler.ThreadUpdate)
	c.Session.AddHandler(eventHandler.ThreadDelete)
	c.Session.AddHandler(eventHandler.ThreadListSync)
	c.Session.AddHandler(eventHandler.ThreadMemberUpdate)
	c.Session.AddHandler(eventHandler.ThreadMembersUpdate)

	// Guild event handlers
	c.Session.AddHandler(eventHandler.GuildCreate)
	c.Session.AddHandler(eventHandler.GuildUpdate)
	c.Session.AddHandler(eventHandler.GuildDelete)
	c.Session.AddHandler(eventHandler.GuildBanAdd)
	c.Session.AddHandler(eventHandler.GuildBanRemove)
	c.Session.AddHandler(eventHandler.GuildMemberAdd)
	c.Session.AddHandler(eventHandler.GuildMemberUpdate)
	c.Session.AddHandler(eventHandler.GuildMemberRemove)
	c.Session.AddHandler(eventHandler.GuildRoleCreate)
	c.Session.AddHandler(eventHandler.GuildRoleUpdate)
	c.Session.AddHandler(eventHandler.GuildRoleDelete)
	c.Session.AddHandler(eventHandler.GuildEmojisUpdate)
	c.Session.AddHandler(eventHandler.GuildStickersUpdate)
	c.Session.AddHandler(eventHandler.GuildMembersChunk)
	c.Session.AddHandler(eventHandler.GuildIntegrationsUpdate)
	c.Session.AddHandler(eventHandler.GuildAuditLogEntryCreate)

	// Stage and scheduled event handlers
	c.Session.AddHandler(eventHandler.StageInstanceEventCreate)
	c.Session.AddHandler(eventHandler.StageInstanceEventUpdate)
	c.Session.AddHandler(eventHandler.StageInstanceEventDelete)
	c.Session.AddHandler(eventHandler.GuildScheduledEventCreate)
	c.Session.AddHandler(eventHandler.GuildScheduledEventUpdate)
	c.Session.AddHandler(eventHandler.GuildScheduledEventDelete)
	c.Session.AddHandler(eventHandler.GuildScheduledEventUserAdd)
	c.Session.AddHandler(eventHandler.GuildScheduledEventUserRemove)

	// Integration event handlers
	c.Session.AddHandler(eventHandler.IntegrationCreate)
	c.Session.AddHandler(eventHandler.IntegrationUpdate)
	c.Session.AddHandler(eventHandler.IntegrationDelete)

	// Message event handlers
	c.Session.AddHandler(eventHandler.MessageUpdate)
	c.Session.AddHandler(eventHandler.MessageDelete)
	c.Session.AddHandler(eventHandler.MessageDeleteBulk)
	c.Session.AddHandler(eventHandler.MessageReactionAdd)
	c.Session.AddHandler(eventHandler.MessageReactionRemove)
	c.Session.AddHandler(eventHandler.MessageReactionRemoveAll)
	c.Session.AddHandler(eventHandler.MessagePollVoteAdd)
	c.Session.AddHandler(eventHandler.MessagePollVoteRemove)

	// User and presence event handlers
	c.Session.AddHandler(eventHandler.PresenceUpdate)
	c.Session.AddHandler(eventHandler.UserUpdate)
	c.Session.AddHandler(eventHandler.TypingStart)

	// Voice event handlers
	c.Session.AddHandler(eventHandler.VoiceServerUpdate)
	c.Session.AddHandler(eventHandler.VoiceStateUpdate)

	// Interaction and command handlers
	c.Session.AddHandler(eventHandler.InteractionCreate)
	c.Session.AddHandler(eventHandler.ApplicationCommandPermissionsUpdate)

	// Invite handlers
	c.Session.AddHandler(eventHandler.InviteCreate)
	c.Session.AddHandler(eventHandler.InviteDelete)

	// Webhook handlers
	c.Session.AddHandler(eventHandler.WebhooksUpdate)

	// Auto-moderation handlers
	c.Session.AddHandler(eventHandler.AutoModerationRuleCreate)
	c.Session.AddHandler(eventHandler.AutoModerationRuleUpdate)
	c.Session.AddHandler(eventHandler.AutoModerationRuleDelete)
	c.Session.AddHandler(eventHandler.AutoModerationActionExecution)

	// Entitlement and subscription handlers
	c.Session.AddHandler(eventHandler.EntitlementCreate)
	c.Session.AddHandler(eventHandler.EntitlementUpdate)
	c.Session.AddHandler(eventHandler.EntitlementDelete)
	c.Session.AddHandler(eventHandler.SubscriptionCreate)
	c.Session.AddHandler(eventHandler.SubscriptionUpdate)
	c.Session.AddHandler(eventHandler.SubscriptionDelete)

	// Connection event handlers
	c.Session.AddHandler(eventHandler.Connect)
	c.Session.AddHandler(eventHandler.Disconnect)
	c.Session.AddHandler(eventHandler.RateLimit)
	c.Session.AddHandler(eventHandler.Resumed)

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
