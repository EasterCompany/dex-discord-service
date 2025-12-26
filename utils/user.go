package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/bwmarrin/discordgo"
	"github.com/redis/go-redis/v9"
)

// UserLevel represents the system authorization tiers
type UserLevel string

const (
	LevelMe          UserLevel = "Me"
	LevelMaster      UserLevel = "Master User"
	LevelAdmin       UserLevel = "Admin"
	LevelModerator   UserLevel = "Moderator"
	LevelContributor UserLevel = "Contributor"
	LevelUser        UserLevel = "User"
	LevelAnyone      UserLevel = "Anyone"
)

// GetUserLevel determines the authorization level of a Discord user.
// It performs a "2FA" check for Master User (Config ID == Server Owner ID).
func GetUserLevel(s *discordgo.Session, redisClient *redis.Client, guildID, userID string, masterUserID string, roles config.DiscordRoleConfig) UserLevel {
	// 1. Me Check (The Bot itself)
	if s.State.User != nil && userID == s.State.User.ID {
		return LevelMe
	}

	// 2. Master User Check (2FA: Config match + Server Owner match)
	if userID == masterUserID {
		guild, err := s.State.Guild(guildID)
		if err == nil && guild.OwnerID == userID {
			return LevelMaster
		}
		// Fallback: fetch from API if not in state
		guild, err = s.Guild(guildID)
		if err == nil && guild.OwnerID == userID {
			return LevelMaster
		}
	}

	// 3. Fetch Member for Role Checks
	member, err := s.State.Member(guildID, userID)
	if err != nil {
		member, err = s.GuildMember(guildID, userID)
	}

	if err == nil && member != nil {
		roleMap := make(map[string]bool)
		for _, rID := range member.Roles {
			roleMap[rID] = true
		}

		// Dexter Role check (if somehow not caught by ID check)
		if roles.Dexter != "" && roleMap[roles.Dexter] {
			return LevelMe
		}

		if roles.Admin != "" && roleMap[roles.Admin] {
			return LevelAdmin
		}
		if roles.Moderator != "" && roleMap[roles.Moderator] {
			return LevelModerator
		}
		if roles.Contributor != "" && roleMap[roles.Contributor] {
			return LevelContributor
		}
		if roles.User != "" && roleMap[roles.User] {
			return LevelUser
		}
	}

	// 4. Default to Anyone
	return LevelAnyone
}

// GetUserDisplayName returns the most human-readable name for a user:
// 1. Cached value (if available)
// 2. Server Nickname (if in a guild and set)
// 3. Global Display Name (if set)
// 4. Global Username
// It caches the result in Redis for 24 hours.
func GetUserDisplayName(s *discordgo.Session, redisClient *redis.Client, guildID, userID string) string {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("user:displayname:%s:%s", guildID, userID)

	// 1. Try to get from cache
	if redisClient != nil {
		cachedName, err := redisClient.Get(ctx, cacheKey).Result()
		if err == nil && cachedName != "" {
			return cachedName
		}
	}

	var displayName string

	// 2. Try to get server nickname first
	if guildID != "" {
		member, err := s.State.Member(guildID, userID)
		if err == nil && member != nil {
			if member.Nick != "" {
				displayName = member.Nick // Server Nickname
			} else if member.User != nil && member.User.DisplayName() != "" {
				displayName = member.User.DisplayName() // User's global display name (from member object)
			} else if member.User != nil && member.User.Username != "" {
				displayName = member.User.Username // User's global username (from member object)
			}
		}
	}

	// 3. Fallback to global user object if no guild or member data insufficient
	if displayName == "" {
		user, err := s.User(userID)
		if err == nil && user != nil {
			if user.DisplayName() != "" {
				displayName = user.DisplayName() // User's global display name
			} else if user.Username != "" {
				displayName = user.Username // User's global username
			}
		}
	}

	// 4. Final fallback if all else fails
	if displayName == "" {
		displayName = "Unknown User"
	} else if redisClient != nil {
		// Cache the result
		redisClient.Set(ctx, cacheKey, displayName, 24*time.Hour)
	}

	return displayName
}
