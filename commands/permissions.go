package commands

import (
	"github.com/EasterCompany/dex-discord-service/config"
	"github.com/bwmarrin/discordgo"
)

// PermissionChecker handles command permission checks
type PermissionChecker struct {
	config   *config.Config
	serverID string
}

// NewPermissionChecker creates a new permission checker
func NewPermissionChecker(cfg *config.Config, serverID string) *PermissionChecker {
	return &PermissionChecker{
		config:   cfg,
		serverID: serverID,
	}
}

// CanExecuteCommand checks if a user has permission to execute commands
func (pc *PermissionChecker) CanExecuteCommand(session *discordgo.Session, userID string) bool {
	// Check whitelist first (angels can always execute)
	for _, whitelistedID := range pc.config.CommandPermissions.UserWhitelist {
		if userID == whitelistedID {
			return true
		}
	}

	// Get guild to check owner
	guild, err := session.Guild(pc.serverID)
	if err != nil {
		return false
	}

	// Check if user is server owner
	if userID == guild.OwnerID {
		return true
	}

	// Check permission level
	switch pc.config.CommandPermissions.DefaultLevel {
	case 0:
		// Owner only (already checked above)
		return false
	case 1:
		// Owner and allowed roles
		member, err := session.GuildMember(pc.serverID, userID)
		if err != nil {
			return false
		}
		return pc.hasAllowedRole(member.Roles)
	case 2:
		// Everyone
		return true
	default:
		// Unknown level, default to owner only
		return false
	}
}

// hasAllowedRole checks if user has any of the allowed roles
func (pc *PermissionChecker) hasAllowedRole(userRoles []string) bool {
	for _, userRole := range userRoles {
		for _, allowedRole := range pc.config.CommandPermissions.AllowedRoles {
			if userRole == allowedRole {
				return true
			}
		}
	}
	return false
}
