package realtime

import (
	"sync"

	"github.com/coder/websocket"
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
