package communication

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"

	"satpam-go/internal/auth"
	"satpam-go/internal/web"
)

type Handler struct {
	hub      *Hub
	upgrader websocket.Upgrader
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{
		hub: hub,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  maxMessageSize,
			WriteBufferSize: maxMessageSize,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *Handler) WS(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.hub == nil {
		log.Println("communication ws error: hub is nil")
		web.WriteError(w, http.StatusInternalServerError, "Voice hub belum siap")
		return
	}

	authCtx, ok := auth.AuthFromContext(r.Context())
	if !ok {
		web.WriteError(w, http.StatusUnauthorized, "Missing bearer token")
		return
	}

	roomID := strings.TrimSpace(r.URL.Query().Get("roomId"))
	if roomID == "" {
		web.WriteError(w, http.StatusBadRequest, "Missing roomId")
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("communication ws upgrade error: %v", err)
		return
	}

	client := &Client{
		id:     newParticipantID(),
		roomID: roomID,
		userID: authCtx.UserID,
		role:   authCtx.Role,
		conn:   conn,
		hub:    h.hub,
		send:   make(chan ServerMessage, sendBufferSize),
	}

	participants, err := h.hub.Join(client)
	if err != nil {
		_ = conn.WriteJSON(ServerMessage{
			Type:    "error",
			RoomID:  roomID,
			Message: "Room penuh, maksimal 4 petugas",
		})
		_ = conn.Close()
		return
	}

	client.enqueue(ServerMessage{
		Type:         "joined",
		RoomID:       roomID,
		From:         client.id,
		Participants: participants,
	})

	go client.writePump()
	client.readPump()
}

func newParticipantID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "peer"
	}
	return hex.EncodeToString(buf[:])
}
