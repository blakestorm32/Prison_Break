package networking

import (
	"time"

	"prison-break/internal/server/auth"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

type ConnectionState string

const (
	ConnectionStateConnected    ConnectionState = "connected"
	ConnectionStateDisconnected ConnectionState = "disconnected"
)

type ConnectionEventType string

const (
	ConnectionEventConnected     ConnectionEventType = "connected"
	ConnectionEventDisconnected  ConnectionEventType = "disconnected"
	ConnectionEventMessageIn     ConnectionEventType = "message_in"
	ConnectionEventMessageOut    ConnectionEventType = "message_out"
	ConnectionEventJoinedMatch   ConnectionEventType = "joined_match"
	ConnectionEventDeliveryError ConnectionEventType = "delivery_error"
	ConnectionEventProtocolError ConnectionEventType = "protocol_error"
)

type ConnectionEvent struct {
	Type         ConnectionEventType  `json:"type"`
	ConnectionID string               `json:"connection_id"`
	MessageType  protocol.MessageType `json:"message_type,omitempty"`
	PlayerID     model.PlayerID       `json:"player_id,omitempty"`
	MatchID      model.MatchID        `json:"match_id,omitempty"`
	At           time.Time            `json:"at"`
	Detail       string               `json:"detail,omitempty"`
}

type ConnectionSnapshot struct {
	ConnectionID string               `json:"connection_id"`
	State        ConnectionState      `json:"state"`
	RemoteAddr   string               `json:"remote_addr"`
	ConnectedAt  time.Time            `json:"connected_at"`
	LastSeenAt   time.Time            `json:"last_seen_at"`
	PlayerID     model.PlayerID       `json:"player_id,omitempty"`
	ViewerID     model.PlayerID       `json:"viewer_id,omitempty"`
	MatchID      model.MatchID        `json:"match_id,omitempty"`
	SessionKind  protocol.SessionKind `json:"session_kind,omitempty"`
	AuthSubject  string               `json:"auth_subject,omitempty"`
	AuthScope    auth.Scope           `json:"auth_scope,omitempty"`
}
