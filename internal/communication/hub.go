package communication

import (
	"errors"
	"sync"
)

const maxRoomParticipants = 4

var ErrRoomFull = errors.New("room is full")

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[string]*Client
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[string]map[string]*Client)}
}

func (h *Hub) Join(client *Client) ([]Participant, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.roomID]
	if room == nil {
		room = make(map[string]*Client)
		h.rooms[client.roomID] = room
	}
	if len(room) >= maxRoomParticipants {
		return nil, ErrRoomFull
	}

	room[client.id] = client
	participants := participantsFromRoom(room)

	joined := ServerMessage{
		Type:   "peer-joined",
		RoomID: client.roomID,
		From:   client.id,
	}
	for id, peer := range room {
		if id != client.id {
			peer.enqueue(joined)
		}
	}

	h.broadcastParticipantsLocked(client.roomID, room)
	return participants, nil
}

func (h *Hub) Leave(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	room := h.rooms[client.roomID]
	if room == nil {
		return
	}
	if _, ok := room[client.id]; !ok {
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
			ID:     client.id,
			UserID: client.userID,
			Role:   client.role,
		})
	}
	return participants
}
