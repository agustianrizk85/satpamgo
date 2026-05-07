package communication

import (
	"errors"
	"sort"
	"sync"
	"time"
)

const (
	defaultRoomParticipants = 5
	meetingRoomParticipants = 20
	feedRoomParticipants    = 1000
)

var ErrRoomFull = errors.New("room is full")

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{
		rooms: make(map[string]map[string]*Client),
	}
}

func (h *Hub) Join(client *Client) ([]Participant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.roomID]
	if room == nil {
		room = make(map[string]*Client)
		h.rooms[client.roomID] = room
	}

	for peerID, peer := range room {
		if peer.userID == client.userID {
			delete(room, peerID)
			close(peer.send)
			_ = peer.conn.Close()
			break
		}
	}

	if len(room) >= maxParticipantsForRoom(client.roomID) {
		return nil, ErrRoomFull
	}

	room[client.id] = client
	participants := participantsFromRoom(room)

	joined := ServerMessage{
		Type:         "peer-joined",
		RoomID:       client.roomID,
		From:         client.id,
		Participants: participants,
	}

	for _, peer := range room {
		peer.enqueue(joined)
	}

	h.broadcastParticipantsLocked(client.roomID, room)

	return participants, nil
}

func maxParticipantsForRoom(roomID string) int {
	switch roomID {
	case "meeting-global":
		return meetingRoomParticipants
	case "global-patrol-feed":
		return feedRoomParticipants
	default:
		return defaultRoomParticipants
	}
}

func (h *Hub) Leave(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.roomID]
	if room == nil {
		return
	}

	current := room[client.id]
	if current == nil {
		return
	}

	delete(room, client.id)
	close(client.send)

	left := ServerMessage{
		Type:   "peer-left",
		RoomID: client.roomID,
		From:   client.id,
	}

	for _, peer := range room {
		peer.enqueue(left)
	}

	if len(room) == 0 {
		delete(h.rooms, client.roomID)
		return
	}

	h.broadcastParticipantsLocked(client.roomID, room)
}

func (h *Hub) Forward(from *Client, message ClientMessage) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room := h.rooms[from.roomID]
	if room == nil {
		return false
	}

	if room[from.id] == nil {
		return false
	}

	target := room[message.To]
	if target == nil {
		return false
	}

	target.enqueue(ServerMessage{
		Type:      message.Type,
		RoomID:    from.roomID,
		From:      from.id,
		To:        message.To,
		SDP:       message.SDP,
		Candidate: message.Candidate,
	})

	return true
}

func (h *Hub) BroadcastRoom(roomID string, message ServerMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	room := h.rooms[roomID]
	if room == nil {
		return
	}
	message.RoomID = roomID
	for _, peer := range room {
		peer.enqueue(message)
	}
}

func (h *Hub) BroadcastChat(from *Client, text string) {
	h.BroadcastRoom(from.roomID, ServerMessage{
		Type:       "chat",
		From:       from.id,
		SenderID:   from.userID,
		SenderName: from.name,
		Text:       text,
		Timestamp:  time.Now().UnixMilli(),
	})
}

func (h *Hub) SetMuted(client *Client, muted bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.roomID]
	if room == nil || room[client.id] == nil {
		return
	}
	client.isMuted = muted
	h.broadcastParticipantsLocked(client.roomID, room)
}

func (h *Hub) Kick(host *Client, targetID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !host.isHost {
		return false
	}
	room := h.rooms[host.roomID]
	if room == nil {
		return false
	}
	target := room[targetID]
	if target == nil || target.id == host.id {
		return false
	}
	target.enqueue(ServerMessage{Type: "kicked", RoomID: host.roomID, Message: "Anda dikeluarkan oleh host"})
	delete(room, target.id)
	close(target.send)
	_ = target.conn.Close()

	for _, peer := range room {
		peer.enqueue(ServerMessage{Type: "peer-left", RoomID: host.roomID, From: target.id})
	}
	if len(room) == 0 {
		delete(h.rooms, host.roomID)
		return true
	}
	h.broadcastParticipantsLocked(host.roomID, room)
	return true
}

func (h *Hub) broadcastParticipantsLocked(roomID string, room map[string]*Client) {
	message := ServerMessage{
		Type:         "participants",
		RoomID:       roomID,
		Participants: participantsFromRoom(room),
	}

	for _, peer := range room {
		peer.enqueue(message)
	}
}

func participantsFromRoom(room map[string]*Client) []Participant {
	participants := make([]Participant, 0, len(room))

	for _, client := range room {
		participants = append(participants, Participant{
			ID:       client.id,
			UserID:   client.userID,
			Role:     client.role,
			FullName: client.name,
			IsHost:   client.isHost,
			IsMuted:  client.isMuted,
		})
	}

	sort.Slice(participants, func(i, j int) bool {
		if participants[i].UserID == participants[j].UserID {
			return participants[i].ID < participants[j].ID
		}
		return participants[i].UserID < participants[j].UserID
	})

	return participants
}
