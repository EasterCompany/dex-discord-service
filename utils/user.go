package utils

import "github.com/bwmarrin/discordgo"

// GetUserDisplayName returns the nickname if present, otherwise the username.
func GetUserDisplayName(s *discordgo.Session, guildID, userID string) string {
	// Try to get guild member first to check for nickname
	if guildID != "" {
		member, err := s.State.Member(guildID, userID)
		if err == nil {
			if member.Nick != "" {
				return member.Nick
			}
			// Fallback to member.User if nickname is empty but member found
			if member.User != nil {
				return member.User.Username
			}
		}
		// If not in state, try fetching from API (optional, might be expensive if done often, but safer for consistency)
		// For now, let's skip API fetch for member to avoid rate limits in high traffic, unless critical.
		// We can fallback to basic user fetch.
	}

	// Fallback to global user object
	user, err := s.User(userID)
	if err == nil {
		return user.Username
	}

	return "Unknown User"
}
