package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/TonyZy-Z/teamFinder/backend/internal/matchmaking"
	"github.com/TonyZy-Z/teamFinder/backend/internal/voice"
	"github.com/redis/go-redis/v9"
)

func newVoiceTokenHandler(
	client *redis.Client,
	livekitURL string,
	apiKey string,
	apiSecret string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID verified by the authentication middleware
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Look up the user's assigned match
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		state, err := matchmaking.GetState(ctx, client, userID)
		if err != nil {
			log.Printf("Voice authorization lookup failed: %v", err)
			http.Error(w, "Unable to authorize voice access", http.StatusInternalServerError)
			return
		}

		if state.Status != "ready" {
			http.Error(w, "Your match must be ready before joining voice room", http.StatusConflict)
			return
		}

		// Allow this user to join the voice room for their ready match
		token, err := voice.CreateJoinToken(
			apiKey, apiSecret, state.MatchID, userID,
		)
		if err != nil {
			log.Printf("Voice token creation failed: %v", err)
			http.Error(w, "Unable to create voice token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")

		if err := json.NewEncoder(w).Encode(map[string]any{
			"server_url":   livekitURL,
			"access_token": token,
		}); err != nil {
			log.Printf("Voice token response failed: %v", err)
		}
	}
}
