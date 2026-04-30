package communication

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 32 * 1024
	sendBufferSize = 32
)

type Client struct {
	id     string
	roomID string
	userID string
	role   string
	conn   *websocket.Conn
	hub    *Hub
	send   chan ServerMessage
}

func (c *Client) enqueue(message ServerMessage) {
	select {
	case c.send <- message:
	default:
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.Leave(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var message ClientMessage
		if err := c.conn.ReadJSON(&message); err != nil {
			return
		}

		switch message.Type {
		case "offer", "answer", "ice-candidate":
			if message.To == "" {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Missing target peer"})
				continue
			}
			if ok := c.hub.Forward(c, message); !ok {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Target peer is not in this room"})
			}
		case "ping":
			c.enqueue(ServerMessage{Type: "pong", RoomID: c.roomID})
		case "leave":
			return
		default:
			c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Unsupported signaling message type"})
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteJSON(message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
