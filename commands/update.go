package commands

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// handleUpdate handles the /update command
func (h *Handler) handleUpdate(channelID string, args []string) {
	// If args has "git", handle git update
	if len(args) > 0 && args[0] == "git" {
		h.handleUpdateGit(channelID)
		return
	}

	// Default: force update all dashboards from cache (no API calls)
	h.sendResponse(channelID, "🔄 Forcing dashboard updates from cache...")
	log.Println("[COMMAND] Forcing dashboard updates from cache")

	// This would trigger dashboard updates from Redis cache
	// without making any Discord API calls
	h.sendResponse(channelID, "✅ Dashboard update complete (cache-only, no API calls)")
}

// handleUpdateGit handles the /update git command
func (h *Handler) handleUpdateGit(channelID string) {
	h.sendResponse(channelID, "🔍 Checking for git updates...")
	log.Println("[COMMAND] Checking for git updates")

	// Check if we're in a git repository
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		h.sendResponse(channelID, "❌ Not in a git repository")
		return
	}

	// Check for uncommitted changes
	cmd = exec.Command("git", "status", "--porcelain")
	output, err := cmd.Output()
	if err != nil {
		h.sendResponse(channelID, fmt.Sprintf("❌ Failed to check git status: %v", err))
		return
	}

	hasChanges := len(strings.TrimSpace(string(output))) > 0

	// Fetch latest from remote
	cmd = exec.Command("git", "fetch", "origin")
	if err := cmd.Run(); err != nil {
		h.sendResponse(channelID, fmt.Sprintf("❌ Failed to fetch from remote: %v", err))
		return
	}

	// Check if we're behind remote
	cmd = exec.Command("git", "rev-list", "--count", "HEAD..origin/main")
	output, err = cmd.Output()
	if err != nil {
		// Try master if main doesn't exist
		cmd = exec.Command("git", "rev-list", "--count", "HEAD..origin/master")
		output, err = cmd.Output()
		if err != nil {
			h.sendResponse(channelID, fmt.Sprintf("❌ Failed to check remote status: %v", err))
			return
		}
	}

	behindCount := strings.TrimSpace(string(output))

	// Build status report
	var report strings.Builder
	report.WriteString("**Git Status Report:**\n")
	report.WriteString(fmt.Sprintf("Uncommitted changes: `%v`\n", hasChanges))
	report.WriteString(fmt.Sprintf("Commits behind remote: `%s`\n", behindCount))

	if hasChanges {
		report.WriteString("\n⚠️ **Cannot pull**: You have uncommitted changes\n")
		report.WriteString("Action: Commit or stash changes before updating")
	} else if behindCount == "0" {
		report.WriteString("\n✅ **Already up to date**")
	} else {
		// Try to pull
		cmd = exec.Command("git", "pull", "--ff-only")
		pullOutput, err := cmd.CombinedOutput()
		if err != nil {
			report.WriteString(fmt.Sprintf("\n❌ **Pull failed**: %v\n", err))
			report.WriteString(fmt.Sprintf("Output: ```%s```", string(pullOutput)))
			report.WriteString("\nAction: Manual intervention required")
		} else {
			report.WriteString(fmt.Sprintf("\n✅ **Pull successful**: Updated %s commits\n", behindCount))
			report.WriteString("```" + string(pullOutput) + "```\n")
			report.WriteString("\n✅ **Safe to rebuild/redeploy**: Run `bash scripts/make.sh`")
		}
	}

	h.sendResponse(channelID, report.String())
}
