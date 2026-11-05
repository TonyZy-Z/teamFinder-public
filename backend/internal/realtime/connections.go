package realtime

import (
	"context"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Connections keep one notification connection per user
type Connections struct {
	mu     sync.Mutex
	byUser map[int64]*websocket.Conn
}

// It creates an empty connection registry
func NewConnections() *Connections {
	return &Connections{
		byUser: make(map[int64]*websocket.Conn),
	}
}

// It stores this connection and returns the user's previous one
func (c *Connections) Register(
	userID int64,
	conn *websocket.Conn,
) *websocket.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()

	previous := c.byUser[userID]
	c.byUser[userID] = conn
	return previous
}

// It removes the connection only if it is still the current one
func (c *Connections) Unregister(userID int64, conn *websocket.Conn) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.byUser[userID] == conn {
		delete(c.byUser, userID)
	}
}

// It writes a JSON notification to the user's current connection.
// It returns false without an error when the user is not connected
func (c *Connections) Send(
	ctx context.Context,
	userID int64,
	message any,
) (bool, error) {
	// Hold the lock only while reading the shared map
	c.mu.Lock()
	conn := c.byUser[userID]
	c.mu.Unlock()

	if conn == nil {
		return false, nil
	}

	// Do network work outside the lock and limit the waiting time
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := wsjson.Write(writeCtx, conn, message); err != nil {
		return false, err
	}
	return true, nil
}
