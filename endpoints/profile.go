package endpoints

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/EasterCompany/dex-discord-service/profile"
	"github.com/EasterCompany/dex-discord-service/utils"
)

// GetProfileHandler retrieves a user's profile from Redis
func GetProfileHandler(w http.ResponseWriter, r *http.Request) {
	// Path is /profile/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	userID := parts[2]

	if redisClient == nil {
		http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
		return
	}

	store := profile.NewStore(redisClient)
	p, err := store.Get(r.Context(), userID)
	if err != nil {
		http.Error(w, "Failed to fetch profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if p == nil {
		// Auto-create default profile if not found
		username := "Unknown User"
		if discordSession != nil {
			user, err := discordSession.User(userID)
			if err == nil {
				username = user.Username
			}
		}

		p = &profile.UserProfile{
			UserID: userID,
			Identity: profile.Identity{
				Username:  username,
				FirstSeen: time.Now().Format(time.RFC3339),
			},
		}
		// Attempt to save it
		_ = store.Save(r.Context(), p)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(p); err != nil {
		utils.LogError("Failed to encode profile: %v", err)
	}
}

// UpdateProfileHandler allows updating the profile via POST/PUT
func UpdateProfileHandler(w http.ResponseWriter, r *http.Request) {
	// Path is /profile/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 || parts[2] == "" {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}
	userID := parts[2]

	var p profile.UserProfile
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Ensure ID matches URL to prevent ID injection attacks
	p.UserID = userID

	if redisClient == nil {
		http.Error(w, "Redis unavailable", http.StatusServiceUnavailable)
		return
	}

	store := profile.NewStore(redisClient)
	if err := store.Save(r.Context(), &p); err != nil {
		http.Error(w, "Failed to save profile: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
