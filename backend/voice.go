package main

import (
	"context"
	"encoding/json"
	lksdk "github.com/livekit/server-sdk-go/v2"
	"log"
	"net/http"
	"time"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/TonyZy-Z/teamFinder/backend/internal/matchmaking"
	"github.com/TonyZy-Z/teamFinder/backend/internal/realtime"
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

// It mark match "ended" in redis and call remote server operation
// to close the LiveKit room
func newLeaveMatchHandler(
	client *redis.Client,
	roomClient *lksdk.RoomServiceClient,
	connections *realtime.Connections,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		matchID := r.PathValue("match_id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		// End the match and release its Redis assignments
		result, err := matchmaking.LeaveMatch(ctx, client, matchID, userID)
		if err != nil {
			log.Printf("Match leave failed: %v", err)
			http.Error(w, "Unable to leave match", http.StatusInternalServerError)
			return
		}

		switch result {
		case "ended":
			// Continue cleanup even when this is a repeated request
		case "missing":
			http.Error(w, "Match not found", http.StatusNotFound)
			return
		case "not_assigned":
			http.Error(w, "You are not a participant in this match", http.StatusForbidden)
			return
		case "not_ready":
			http.Error(w, "Only a ready match can be ended", http.StatusConflict)
			return
		default:
			log.Printf("Unexpected match leave result: %s", result)
			http.Error(w, "Unable to leave match", http.StatusInternalServerError)
			return
		}

		// Get participants IDs from the ended match hash
		fields, err := client.HGetAll(
			ctx,
			"matchmaking:match:"+matchID,
		).Result()

		if err != nil {
			log.Printf("Voice cleanup participant lookup failed: %v", err)
			http.Error(w, "Match ended, but cleanup failed; retry leaving", http.StatusInternalServerError)
			return
		}

		user1 := fields["user_1"]
		user2 := fields["user_2"]
		if user1 == "" || user2 == "" || user1 == user2 {
			log.Printf("Invalid voice cleanup participants for match %s", matchID)
			http.Error(w, "Unable to identify voice participants", http.StatusInternalServerError)
			return
		}

		// Revoke both users' exisiting tokens before deleting the room
		revokeErr := voice.RevokeRoomAccess(
			ctx,
			roomClient,
			matchID,
			[]string{user1, user2},
		)

		// Close the voice room and disconnect its participants
		closeErr := voice.CloseRoom(ctx, roomClient, matchID)

		// Send each participant their actual current matchmaking state
		notifyMatchParticipants(ctx, client, connections, matchID)

		if revokeErr != nil || closeErr != nil {
			log.Printf(
				"Voice cleanup failed for %s: revocation=%v, deletion=%v",
				matchID,
				revokeErr,
				closeErr,
			)
			http.Error(
				w,
				"Match ended, but voice cleanup failed; retry leaving",
				http.StatusBadGateway,
			)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}
