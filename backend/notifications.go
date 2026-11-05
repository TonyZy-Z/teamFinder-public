package main

import (
	"context"
	"log"
	"strconv"

	"github.com/TonyZy-Z/teamFinder/backend/internal/matchmaking"
	"github.com/TonyZy-Z/teamFinder/backend/internal/realtime"
	"github.com/redis/go-redis/v9"
)

// matchmakingEvent describes a state update sent over WebSocket
type matchmakingEvent struct {
	Type  string            `json:"type"`
	State matchmaking.State `json:"state"`
}

// It reads Redis before notifying this user
func notifyMatchmakingState(
	ctx context.Context,
	client *redis.Client,
	connections *realtime.Connections,
	userID int64,
) {
	state, err := matchmaking.GetState(ctx, client, userID)
	if err != nil {
		log.Printf("Notification state lookup failed for user %d: %v", userID, err)
		return
	}

	// An user without active notificaiton connection can retrieve their state through the status endpoint
	_, err = connections.Send(ctx, userID, matchmakingEvent{
		Type:  "matchmaking.updated",
		State: state,
	})
	if err != nil {
		log.Printf("Matchmaking notification failed for user %d: %v", userID, err)
	}
}

// It sends the current state to both users in a match
func notifyMatchParticipants(
	ctx context.Context,
	client *redis.Client,
	connections *realtime.Connections,
	matchID string,
) {
	fields, err := client.HGetAll(
		ctx,
		"matchmaking:match:"+matchID,
	).Result()

	if err != nil {
		log.Printf("Notification participant lookup failed: %v", err)
		return
	}

	user1, err := strconv.ParseInt(fields["user_1"], 10, 64)
	if err != nil || user1 <= 0 {
		log.Printf("Invalid first notification participant for match %s", matchID)
		return
	}

	user2, err := strconv.ParseInt(fields["user_2"], 10, 64)
	if err != nil || user2 <= 0 || user1 == user2{
		log.Printf("Invalid second notification participant for match %s", matchID)
		return
	}

	// Each notification reads that user's current Redis state
	notifyMatchmakingState(ctx, client, connections, user1)
	notifyMatchmakingState(ctx, client, connections, user2)
}
