package communication

import (
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 32 * 1024
	sendBufferSize = 64
)

type Client struct {
	id      string
	roomID  string
	userID  string
	role    string
	name    string
	isHost  bool
	isMuted bool
	conn    *websocket.Conn
	hub     *Hub
	send    chan ServerMessage
}

func (c *Client) enqueue(message ServerMessage) bool {
	select {
	case c.send <- message:
		return true
	default:
		return false
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
		case "offer", "answer", "ice-candidate", "candidate":
			if message.To == "" {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Missing target peer"})
				continue
			}
			if message.Type == "candidate" {
				message.Type = "ice-candidate"
			}
			if ok := c.hub.Forward(c, message); !ok {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Target peer is not in this room"})
			}
		case "chat":
			text := strings.TrimSpace(message.Text)
			if text == "" {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Pesan kosong"})
				continue
			}
			c.hub.BroadcastChat(c, text)
		case "mute-state":
			muted := message.Muted != nil && *message.Muted
			c.hub.SetMuted(c, muted)
		case "mute":
			c.hub.SetMuted(c, true)
		case "unmute":
			c.hub.SetMuted(c, false)
		case "kick":
			if message.To == "" {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Target kick kosong"})
				continue
			}
			if ok := c.hub.Kick(c, message.To); !ok {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Hanya host yang bisa kick atau target tidak ditemukan"})
			}
		case "ping":
			c.enqueue(ServerMessage{Type: "pong", RoomID: c.roomID})
		case "leave":
			return
		default:
			if message.Type == "" && message.Candidate != nil {
				c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Unsupported signaling message type: missing type for candidate"})
				continue
			}
			c.enqueue(ServerMessage{Type: "error", RoomID: c.roomID, Message: "Unsupported signaling message type: " + message.Type})
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
