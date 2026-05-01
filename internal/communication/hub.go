package communication

import (
	"errors"
	"sort"
	"sync"
)

const maxRoomParticipants = 4

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

	if len(room) >= maxRoomParticipants {
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

	sort.Slice(participants, func(i, j int) bool {
		if participants[i].UserID == participants[j].UserID {
			return participants[i].ID < participants[j].ID
		}
		return participants[i].UserID < participants[j].UserID
	})

	return participants
}
