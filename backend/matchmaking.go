package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/TonyZy-Z/teamFinder/backend/internal/matchmaking"
	"github.com/TonyZy-Z/teamFinder/backend/internal/realtime"
	"github.com/redis/go-redis/v9"
)

type joinQueueRequest struct {
	GameID string `json:"game_id"`
}

// This lets a participant confirm their assigned match
func newConfirmMatchHandler(
	client *redis.Client,
	connections *realtime.Connections,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Read the match ID from the URL, not the request body
		matchID := r.PathValue("match_id")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		result, err := matchmaking.ConfirmMatch(ctx, client, matchID, userID)

		if err != nil {
			log.Printf("Match confirmation failed: %v", err)
			http.Error(w, "Unable to confirm match", http.StatusInternalServerError)
			return
		}

		switch result {
		case "pending", "ready":
			// Confirmation succeed
		case "missing":
			http.Error(w, "Match not found", http.StatusNotFound)
			return
		case "not_assigned_to_match":
			http.Error(w, "You are not assigned to this match", http.StatusForbidden)
			return
		case "deadline_passed":
			http.Error(w, "Match confirmation deadline has passed", http.StatusConflict)
			return
		default:
			log.Printf("Unexpected confirmation result: %s", result)
			http.Error(w, "Unable to confirm match", http.StatusInternalServerError)
			return
		}

		state, err := matchmaking.GetState(ctx, client, userID)
		if err != nil {
			log.Printf("State lookup after confirmation failed: %v", err)
			http.Error(w, "Unable to retrieve matchmaking status", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(state); err != nil {
			log.Printf("Confirmation response failed: %v", err)
		}

		// Both users should learn when confirmation makes the match ready
		if result == "ready" {
			notifyMatchParticipants(ctx, client, connections, matchID)
		}
	}
}

func newMatchmakingStatusHandler(client *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		state, err := matchmaking.GetState(ctx, client, userID)
		if err != nil {
			log.Print("Matchmaking status lookup failed: ", err)
			http.Error(w, "Unable to retrieve matchmaking status", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(state); err != nil {
			log.Print("Matchmaking status response failed: ", err)
		}
	}
}

func newCancelQueueHandler(client *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		gameIDs := make([]string, 0, len(supportedGames))
		for _, game := range supportedGames {
			gameIDs = append(gameIDs, game.ID)
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := matchmaking.CancelQueue(ctx, client, userID, gameIDs); err != nil {
			log.Print("Queue cancellation failed: ", err)
			http.Error(w, "Unable to cancel search", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

func newJoinQueueHandler(
	client *redis.Client,
	connections *realtime.Connections,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		defer r.Body.Close()

		var input joinQueueRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()

		if err := decoder.Decode(&input); err != nil {
			http.Error(w, "Invalid queue request", http.StatusBadRequest)
			return
		}

		if !isSupportedGame(input.GameID) {
			http.Error(w, "Unsupported game", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		result, err := matchmaking.JoinQueue(ctx, client, userID, input.GameID)
		if err != nil {
			log.Print("Queue entry failed: ", err)
			http.Error(w, "Unable to join queue", http.StatusInternalServerError)
			return
		}

		switch result {
		case "joined", "already_queued":
			match, pairErr := matchmaking.TryPair(ctx, client, input.GameID)
			if pairErr != nil {
				log.Print("Pairing attempt failed: ", pairErr)
			}

			state, err := matchmaking.GetState(ctx, client, userID)
			if err != nil {
				log.Print("Queue state lookup failed: ", err)
				http.Error(w, "Unable to retrieve matchmaking status",
					http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(state); err != nil {
				log.Print("Queue response failed: ", err)
			}

			// Notify the actual participants, who may differ from the requester
			if pairErr == nil && match != nil {
				notifyMatchmakingState(ctx, client, connections, match.User1)
				notifyMatchmakingState(ctx, client, connections, match.User2)
			}

		case "different_game":
			http.Error(w, "Already searching for another game", http.StatusConflict)
		case "active_match":
			http.Error(w, "Already assigned to a match", http.StatusConflict)
		default:
			log.Print("Unexpected queue result: ", result)
			http.Error(w, "Unable to join queue", http.StatusInternalServerError)
		}
	}
}
