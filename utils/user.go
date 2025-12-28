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
	LevelMaster      UserLevel = "Master"
	LevelAdmin       UserLevel = "Admin"
	LevelModerator   UserLevel = "Moderator"
	LevelContributor UserLevel = "Contributor"
	LevelUser        UserLevel = "User"
)

// GetUserLevel resolves the user level for a Discord user.
func GetUserLevel(s *discordgo.Session, redisClient *redis.Client, guildID, userID, masterID string, roles config.DiscordRoleConfig) UserLevel {
	// 1. Self Check
	if userID == s.State.User.ID {
		return LevelMe
	}

	// 2. Master Check
	if userID == masterID {
		return LevelMaster
	}

	// 3. Fetch Member for Roles
	if guildID != "" {
		member, err := s.State.Member(guildID, userID)
		if err != nil {
			member, err = s.GuildMember(guildID, userID)
		}

		if err == nil && member != nil {
			for _, roleID := range member.Roles {
				if roleID == roles.Admin {
					return LevelAdmin
				}
				if roleID == roles.Moderator {
					return LevelModerator
				}
				if roleID == roles.Contributor {
					return LevelContributor
				}
				if roleID == roles.User {
					return LevelUser
				}
			}
		}
	}

	// 4. Default to User
	return LevelUser
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
