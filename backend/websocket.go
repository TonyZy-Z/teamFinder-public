package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/TonyZy-Z/teamFinder/backend/internal/auth"
	"github.com/TonyZy-Z/teamFinder/backend/internal/realtime"
)

// It opens an authenticated notification connection
func newWebSocketHandler(
	appCtx context.Context,
	connections *realtime.Connections,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := auth.UserIDFromContext(r.Context())
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Switch from HTTP to WebSocket after authentication succeeds
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			log.Printf("WebSocket connection failed: %v", err)
			return
		}
		defer conn.CloseNow()

		// Watch for client disconnection or backend shutdown
		// Clients receive notifications; actions still use HTTP endpoints.
		connectionCtx := conn.CloseRead(appCtx)

		// Limit how long we wait to send the inital acknowledgment
		writeCtx, cancel := context.WithTimeout(connectionCtx, 5*time.Second)
		err = wsjson.Write(writeCtx, conn, map[string]any{
			"type":    "connection.ready",
			"user_id": userID,
		})
		cancel()
		if err != nil {
			log.Printf("WebSocket acknowledgment failed: %v", err)
			return
		}

		// Make this user's connection available for future notifications
		previous := connections.Register(userID, conn)
		defer connections.Unregister(userID, conn)

		// Keep only the newest connection for this user
		// CLose outside the registry lock so network work does not hold it
		if previous != nil {
			previous.Close(websocket.StatusNormalClosure, "Replaced by a new connection")
		}

		// Keep the handler alive while the connection is open
		<-connectionCtx.Done()
	}
}
