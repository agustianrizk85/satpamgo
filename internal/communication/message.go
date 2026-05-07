package communication

type ClientMessage struct {
	Type      string `json:"type"`
	To        string `json:"to,omitempty"`
	SDP       string `json:"sdp,omitempty"`
	Candidate any    `json:"candidate,omitempty"`
	Text      string `json:"text,omitempty"`
	Muted     *bool  `json:"muted,omitempty"`
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
	Text         string        `json:"text,omitempty"`
	SenderID     string        `json:"senderId,omitempty"`
	SenderName   string        `json:"senderName,omitempty"`
	Timestamp    int64         `json:"timestamp,omitempty"`
	Muted        *bool         `json:"muted,omitempty"`
}

type Participant struct {
	ID       string `json:"id"`
	UserID   string `json:"userId"`
	Role     string `json:"role"`
	UserName string `json:"userName,omitempty"`
	FullName string `json:"fullName,omitempty"`
	IsHost   bool   `json:"isHost"`
	IsMuted  bool   `json:"isMuted"`
}
