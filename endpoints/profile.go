package endpoints

import (
	"encoding/json"
	"net/http"
	"strings"

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

	w.Header().Set("Content-Type", "application/json")
	if p == nil {
		// Return 404 if not found. The frontend will handle this by showing default/mock data or an empty state.
		http.Error(w, "Profile not found", http.StatusNotFound)
		return
	}

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
