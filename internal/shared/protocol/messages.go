package protocol

import (
	"encoding/json"
	"time"

	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type MessageType string

const (
	MsgJoinGame      MessageType = "join_game"
	MsgListLobbies   MessageType = "list_lobbies"
	MsgRequestReplay MessageType = "request_replay"
	MsgLeaveMatch    MessageType = "leave_match"
	MsgPlayerInput   MessageType = "player_input"
	MsgAbilityUse    MessageType = "ability_use"
	MsgCardUse       MessageType = "card_use"
	MsgPing          MessageType = "ping"
	MsgAckSnapshot   MessageType = "ack_snapshot"

	MsgJoinAccepted MessageType = "join_accepted"
	MsgLobbyList    MessageType = "lobby_list"
	MsgReplayLog    MessageType = "replay_log"
	MsgPlayerJoined MessageType = "player_joined"
	MsgPlayerLeft   MessageType = "player_left"
	MsgGameStart    MessageType = "game_start"
	MsgSnapshot     MessageType = "snapshot"
	MsgPhaseChange  MessageType = "phase_change"
	MsgGameOver     MessageType = "game_over"
	MsgPong         MessageType = "pong"
	MsgError        MessageType = "error"
)

type Envelope struct {
	Version uint16      `json:"version"`
	Type    MessageType `json:"type"`

	RequestID string         `json:"request_id,omitempty"`
	MatchID   model.MatchID  `json:"match_id,omitempty"`
	PlayerID  model.PlayerID `json:"player_id,omitempty"`

	Payload json.RawMessage `json:"payload,omitempty"`
}

func NewEnvelope(messageType MessageType, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}

	return Envelope{
		Version: constants.ProtocolVersion,
		Type:    messageType,
		Payload: raw,
	}, nil
}

func DecodePayload[T any](env Envelope) (T, error) {
	var out T
	if len(env.Payload) == 0 {
		return out, nil
	}

	err := json.Unmarshal(env.Payload, &out)
	return out, err
}

type JoinGameRequest struct {
	PlayerName              string            `json:"player_name"`
	PreferredMatchID        model.MatchID     `json:"preferred_match_id,omitempty"`
	PreferredRegion         string            `json:"preferred_region,omitempty"`
	RegionLatencyMS         map[string]uint16 `json:"region_latency_ms,omitempty"`
	Spectator               bool              `json:"spectator,omitempty"`
	SpectatorFollowPlayerID model.PlayerID    `json:"spectator_follow_player_id,omitempty"`
	SpectatorFollowSlot     uint8             `json:"spectator_follow_slot,omitempty"`
	SessionToken            string            `json:"session_token,omitempty"`
	ClientBuild             string            `json:"client_build,omitempty"`
}

type SessionKind string

const (
	SessionKindPlayer    SessionKind = "player"
	SessionKindSpectator SessionKind = "spectator"
)

type JoinGameAccepted struct {
	MatchID                 model.MatchID  `json:"match_id"`
	PlayerID                model.PlayerID `json:"player_id,omitempty"`
	MinPlayers              uint8          `json:"min_players"`
	MaxPlayers              uint8          `json:"max_players"`
	TickRateHz              uint32         `json:"tick_rate_hz"`
	SessionKind             SessionKind    `json:"session_kind"`
	CanSendCommands         bool           `json:"can_send_commands"`
	SpectatorFollowPlayerID model.PlayerID `json:"spectator_follow_player_id,omitempty"`
	SpectatorFollowSlot     uint8          `json:"spectator_follow_slot,omitempty"`
	SpectatorSlotCount      uint8          `json:"spectator_slot_count,omitempty"`
}

type ListLobbiesRequest struct {
	IncludeRunning  bool              `json:"include_running,omitempty"`
	PreferredRegion string            `json:"preferred_region,omitempty"`
	RegionLatencyMS map[string]uint16 `json:"region_latency_ms,omitempty"`
	SessionToken    string            `json:"session_token,omitempty"`
}

type ReplayLogRequest struct {
	MatchID model.MatchID `json:"match_id,omitempty"`
}

type LeaveMatchRequest struct {
	Reason string `json:"reason,omitempty"`
}

type PlayerInputMessage struct {
	Command model.InputCommand `json:"command"`
}

type AbilityUseMessage struct {
	ClientSeq uint64                  `json:"client_seq"`
	Payload   model.AbilityUsePayload `json:"payload"`
}

type CardUseMessage struct {
	ClientSeq uint64               `json:"client_seq"`
	Payload   model.CardUsePayload `json:"payload"`
}

type PingMessage struct {
	ClientSendUnixMS int64 `json:"client_send_unix_ms"`
}

type PongMessage struct {
	ClientSendUnixMS int64 `json:"client_send_unix_ms"`
	ServerSendUnixMS int64 `json:"server_send_unix_ms"`
}

type SnapshotAckMessage struct {
	ClientTickID           uint64 `json:"client_tick_id"`
	LastProcessedClientSeq uint64 `json:"last_processed_client_seq"`
}

type PlayerJoinedMessage struct {
	PlayerID model.PlayerID `json:"player_id"`
	Name     string         `json:"name"`
}

type PlayerLeftMessage struct {
	PlayerID model.PlayerID `json:"player_id"`
	Reason   string         `json:"reason,omitempty"`
}

type LobbySummary struct {
	MatchID      model.MatchID     `json:"match_id"`
	Region       string            `json:"region,omitempty"`
	Status       model.MatchStatus `json:"status"`
	PlayerCount  uint8             `json:"player_count"`
	MinPlayers   uint8             `json:"min_players"`
	MaxPlayers   uint8             `json:"max_players"`
	OpenSlots    uint8             `json:"open_slots"`
	Joinable     bool              `json:"joinable"`
	ReadyToStart bool              `json:"ready_to_start"`
	CreatedAt    time.Time         `json:"created_at"`
}

type LobbyListMessage struct {
	Lobbies []LobbySummary `json:"lobbies"`
}

type ReplayInputEntry struct {
	AcceptedTick uint64             `json:"accepted_tick"`
	IngressSeq   uint64             `json:"ingress_seq"`
	AcceptedAt   time.Time          `json:"accepted_at"`
	Command      model.InputCommand `json:"command"`
}

type ReplayLogMessage struct {
	MatchID     model.MatchID      `json:"match_id"`
	Status      model.MatchStatus  `json:"status"`
	TickRateHz  uint32             `json:"tick_rate_hz"`
	CreatedAt   time.Time          `json:"created_at"`
	StartedAt   *time.Time         `json:"started_at,omitempty"`
	EndedAt     *time.Time         `json:"ended_at,omitempty"`
	EndedReason string             `json:"ended_reason,omitempty"`
	Entries     []ReplayInputEntry `json:"entries"`
}

type GameStartMessage struct {
	MatchID         model.MatchID  `json:"match_id"`
	LocalPlayerID   model.PlayerID `json:"local_player_id"`
	StartTickID     uint64         `json:"start_tick_id"`
	InitialSnapshot model.Snapshot `json:"initial_snapshot"`
}

type SnapshotMessage struct {
	Snapshot model.Snapshot `json:"snapshot"`
}

type PhaseChangeMessage struct {
	TickID     uint64           `json:"tick_id"`
	CycleCount uint8            `json:"cycle_count"`
	Phase      model.PhaseState `json:"phase"`
}

type GameOverMessage struct {
	TickID uint64              `json:"tick_id"`
	Result model.GameOverState `json:"result"`
}

type ErrorCode string

const (
	ErrUnknown            ErrorCode = "unknown"
	ErrInvalidPayload     ErrorCode = "invalid_payload"
	ErrUnsupportedVersion ErrorCode = "unsupported_version"
	ErrUnauthorized       ErrorCode = "unauthorized"
	ErrRateLimited        ErrorCode = "rate_limited"
	ErrOutOfDateCommand   ErrorCode = "out_of_date_command"
	ErrMatchNotFound      ErrorCode = "match_not_found"
)

type ErrorMessage struct {
	Code      ErrorCode `json:"code"`
	Message   string    `json:"message"`
	Retryable bool      `json:"retryable"`
}
