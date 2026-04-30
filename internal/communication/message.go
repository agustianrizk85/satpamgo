package communication

type ClientMessage struct {
	Type      string `json:"type"`
	To        string `json:"to,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Candidate any    `json:"candidate,omitempty"`
}

type ServerMessage struct {
	Type         string        `json:"type"`
	RoomID       string        `json:"roomId,omitempty"`
	From         string        `json:"from,omitempty"`
	To           string        `json:"to,omitempty"`
	SDP          string        `json:"sdp,omitempty"`
	Candidate    any           `json:"candidate,omitempty"`
	Participants []Participant `json:"participants,omitempty"`
	Message      string        `json:"message,omitempty"`
}

type Participant struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Role   string `json:"role"`
}
