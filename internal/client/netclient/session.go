package netclient

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

var (
	ErrReadOnlySession = errors.New("netclient: session is read-only")
	ErrSendQueueFull   = errors.New("netclient: send queue full")
	ErrSessionClosed   = errors.New("netclient: session closed")
)

type SessionConfig struct {
	ServerURL               string
	PlayerName              string
	PlayerID                model.PlayerID
	PreferredMatchID        model.MatchID
	PreferredRegion         string
	RegionLatencyMS         map[string]uint16
	Spectator               bool
	SpectatorFollowPlayerID model.PlayerID
	SpectatorFollowSlot     uint8
	AuthToken               string

	HandshakeTimeout time.Duration
	WriteTimeout     time.Duration
	SendQueueDepth   int
}

func DefaultSessionConfig() SessionConfig {
	return SessionConfig{
		ServerURL:        "ws://127.0.0.1:8080/ws",
		PlayerName:       "Player",
		HandshakeTimeout: 6 * time.Second,
		WriteTimeout:     2 * time.Second,
		SendQueueDepth:   128,
	}
}

func (c SessionConfig) normalized() SessionConfig {
	defaults := DefaultSessionConfig()

	c.ServerURL = strings.TrimSpace(c.ServerURL)
	if c.ServerURL == "" {
		c.ServerURL = defaults.ServerURL
	}

	c.PlayerName = strings.TrimSpace(c.PlayerName)
	if c.PlayerName == "" {
		if c.Spectator {
			c.PlayerName = "Spectator"
		} else {
			c.PlayerName = defaults.PlayerName
		}
	}
	c.AuthToken = strings.TrimSpace(c.AuthToken)
	c.PreferredRegion = strings.ToLower(strings.TrimSpace(c.PreferredRegion))
	c.SpectatorFollowPlayerID = model.PlayerID(strings.TrimSpace(string(c.SpectatorFollowPlayerID)))
	if len(c.RegionLatencyMS) > 0 {
		cloned := make(map[string]uint16, len(c.RegionLatencyMS))
		for region, latency := range c.RegionLatencyMS {
			normalizedRegion := strings.ToLower(strings.TrimSpace(region))
			if normalizedRegion == "" || latency == 0 {
				continue
			}
			cloned[normalizedRegion] = latency
		}
		c.RegionLatencyMS = cloned
	}

	if c.HandshakeTimeout <= 0 {
		c.HandshakeTimeout = defaults.HandshakeTimeout
	}
	if c.WriteTimeout <= 0 {
		c.WriteTimeout = defaults.WriteTimeout
	}
	if c.SendQueueDepth <= 0 {
		c.SendQueueDepth = defaults.SendQueueDepth
	}
	return c
}

type Session struct {
	mu sync.RWMutex

	conn  *websocket.Conn
	store *SnapshotStore

	localPlayerID           model.PlayerID
	sessionKind             protocol.SessionKind
	matchID                 model.MatchID
	minPlayers              uint8
	maxPlayers              uint8
	spectatorFollowPlayerID model.PlayerID
	spectatorFollowSlot     uint8
	spectatorSlotCount      uint8

	writeTimeout time.Duration
	writeMu      sync.Mutex
	sendQueue    chan protocol.Envelope

	closeOnce sync.Once
	done      chan struct{}
	sendDone  chan struct{}

	lastSnapshotAckTick uint64
	lastServerAckSeq    uint64
	lastServerAckTick   uint64
	lastAsyncErr        error
}

func (s *Session) Store() *SnapshotStore {
	return s.store
}

func (s *Session) LocalPlayerID() model.PlayerID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.localPlayerID
}

func (s *Session) SessionKind() protocol.SessionKind {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessionKind
}

func (s *Session) MatchID() model.MatchID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.matchID
}

func (s *Session) MinPlayers() uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.minPlayers
}

func (s *Session) MaxPlayers() uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.maxPlayers
}

func (s *Session) SpectatorFollowPlayerID() model.PlayerID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spectatorFollowPlayerID
}

func (s *Session) SpectatorFollowSlot() uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spectatorFollowSlot
}

func (s *Session) SpectatorSlotCount() uint8 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.spectatorSlotCount
}

func (s *Session) Done() <-chan struct{} {
	return s.done
}

func (s *Session) LastAsyncError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastAsyncErr
}

func (s *Session) LastSnapshotAckTick() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSnapshotAckTick
}

func (s *Session) LastServerAckedClientSeq() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastServerAckSeq
}

func (s *Session) LastServerAckTick() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastServerAckTick
}

func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.conn != nil {
			closeErr = s.conn.Close()
		}
		if s.done != nil {
			<-s.done
		}
		if s.sendQueue != nil {
			close(s.sendQueue)
		}
		if s.sendDone != nil {
			<-s.sendDone
		}
	})
	return closeErr
}

func DialAndJoin(ctx context.Context, config SessionConfig) (*Session, error) {
	normalized := config.normalized()

	parsedURL, err := url.Parse(normalized.ServerURL)
	if err != nil {
		return nil, fmt.Errorf("netclient: parse server URL: %w", err)
	}
	if parsedURL.Scheme != "ws" && parsedURL.Scheme != "wss" {
		return nil, errors.New("netclient: server URL must use ws or wss scheme")
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: normalized.HandshakeTimeout,
	}
	conn, _, err := dialer.DialContext(ctx, normalized.ServerURL, nil)
	if err != nil {
		return nil, fmt.Errorf("netclient: dial websocket: %w", err)
	}

	store := NewSnapshotStore()
	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:              normalized.PlayerName,
		PreferredMatchID:        normalized.PreferredMatchID,
		PreferredRegion:         normalized.PreferredRegion,
		RegionLatencyMS:         normalized.RegionLatencyMS,
		Spectator:               normalized.Spectator,
		SpectatorFollowPlayerID: normalized.SpectatorFollowPlayerID,
		SpectatorFollowSlot:     normalized.SpectatorFollowSlot,
		SessionToken:            normalized.AuthToken,
	})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("netclient: build join envelope: %w", err)
	}
	if normalized.PlayerID != "" {
		joinEnvelope.PlayerID = normalized.PlayerID
	}
	_ = conn.SetWriteDeadline(time.Now().Add(normalized.WriteTimeout))
	if err := conn.WriteJSON(joinEnvelope); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("netclient: send join envelope: %w", err)
	}
	_ = conn.SetWriteDeadline(time.Time{})

	handshakeDeadline := time.Now().Add(normalized.HandshakeTimeout)
	_ = conn.SetReadDeadline(handshakeDeadline)

	var (
		accepted protocol.JoinGameAccepted
		gotJoin  bool
		gotState bool
	)

	for !(gotJoin && gotState) {
		select {
		case <-ctx.Done():
			_ = conn.Close()
			return nil, fmt.Errorf("netclient: join canceled: %w", ctx.Err())
		default:
		}

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("netclient: read join handshake envelope: %w", err)
		}

		switch envelope.Type {
		case protocol.MsgJoinAccepted:
			decoded, decodeErr := protocol.DecodePayload[protocol.JoinGameAccepted](envelope)
			if decodeErr != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("netclient: decode join_accepted payload: %w", decodeErr)
			}
			accepted = decoded
			gotJoin = true

		case protocol.MsgSnapshot:
			payload, decodeErr := protocol.DecodePayload[protocol.SnapshotMessage](envelope)
			if decodeErr != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("netclient: decode snapshot payload during handshake: %w", decodeErr)
			}
			if store.ApplySnapshot(payload.Snapshot) {
				gotState = true
			}

		case protocol.MsgGameStart:
			payload, decodeErr := protocol.DecodePayload[protocol.GameStartMessage](envelope)
			if decodeErr != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("netclient: decode game_start payload during handshake: %w", decodeErr)
			}
			if store.ApplySnapshot(payload.InitialSnapshot) {
				gotState = true
			}
			if !gotJoin {
				// Support out-of-order handshake events from future protocol changes.
				accepted.MatchID = payload.MatchID
				accepted.PlayerID = payload.LocalPlayerID
			}

		case protocol.MsgError:
			payload, decodeErr := protocol.DecodePayload[protocol.ErrorMessage](envelope)
			if decodeErr != nil {
				_ = conn.Close()
				return nil, fmt.Errorf("netclient: decode error payload during handshake: %w", decodeErr)
			}
			_ = conn.Close()
			return nil, fmt.Errorf("netclient: join rejected (%s): %s", payload.Code, payload.Message)

		default:
			// Ignore non-handshake messages.
		}
	}

	_ = conn.SetReadDeadline(time.Time{})

	var initialServerAckSeq uint64
	var initialServerAckTick uint64
	if meta, ok := store.LatestSnapshotMeta(); ok && accepted.PlayerID != "" {
		if ackSeq, found := findAckedClientSeq(meta.PlayerAcks, accepted.PlayerID); found {
			initialServerAckSeq = ackSeq
			initialServerAckTick = meta.TickID
		}
	}

	session := &Session{
		conn:                    conn,
		store:                   store,
		localPlayerID:           accepted.PlayerID,
		sessionKind:             accepted.SessionKind,
		matchID:                 accepted.MatchID,
		minPlayers:              accepted.MinPlayers,
		maxPlayers:              accepted.MaxPlayers,
		spectatorFollowPlayerID: accepted.SpectatorFollowPlayerID,
		spectatorFollowSlot:     accepted.SpectatorFollowSlot,
		spectatorSlotCount:      accepted.SpectatorSlotCount,
		writeTimeout:            normalized.WriteTimeout,
		sendQueue:               make(chan protocol.Envelope, normalized.SendQueueDepth),
		done:                    make(chan struct{}),
		sendDone:                make(chan struct{}),
		lastServerAckSeq:        initialServerAckSeq,
		lastServerAckTick:       initialServerAckTick,
	}
	go session.readLoop()
	go session.writeLoop()
	return session, nil
}

func (s *Session) readLoop() {
	defer close(s.done)

	for {
		var envelope protocol.Envelope
		if err := s.conn.ReadJSON(&envelope); err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			s.setAsyncErr(err)
			return
		}

		switch envelope.Type {
		case protocol.MsgSnapshot:
			payload, err := protocol.DecodePayload[protocol.SnapshotMessage](envelope)
			if err != nil {
				s.setAsyncErr(fmt.Errorf("decode snapshot payload: %w", err))
				continue
			}
			s.onAuthoritativeSnapshot(payload.Snapshot)

		case protocol.MsgGameStart:
			payload, err := protocol.DecodePayload[protocol.GameStartMessage](envelope)
			if err != nil {
				s.setAsyncErr(fmt.Errorf("decode game_start payload: %w", err))
				continue
			}
			s.onAuthoritativeSnapshot(payload.InitialSnapshot)

		case protocol.MsgJoinAccepted:
			payload, err := protocol.DecodePayload[protocol.JoinGameAccepted](envelope)
			if err != nil {
				s.setAsyncErr(fmt.Errorf("decode join_accepted payload: %w", err))
				continue
			}
			s.mu.Lock()
			if payload.PlayerID != "" {
				s.localPlayerID = payload.PlayerID
			}
			if payload.SessionKind != "" {
				s.sessionKind = payload.SessionKind
			}
			if payload.MatchID != "" {
				s.matchID = payload.MatchID
			}
			if payload.MinPlayers > 0 {
				s.minPlayers = payload.MinPlayers
			}
			if payload.MaxPlayers > 0 {
				s.maxPlayers = payload.MaxPlayers
			}
			if payload.SpectatorFollowPlayerID != "" {
				s.spectatorFollowPlayerID = payload.SpectatorFollowPlayerID
			}
			if payload.SpectatorFollowSlot > 0 {
				s.spectatorFollowSlot = payload.SpectatorFollowSlot
			}
			if payload.SpectatorSlotCount > 0 {
				s.spectatorSlotCount = payload.SpectatorSlotCount
			}
			s.mu.Unlock()

		case protocol.MsgError:
			payload, err := protocol.DecodePayload[protocol.ErrorMessage](envelope)
			if err != nil {
				s.setAsyncErr(fmt.Errorf("decode error payload: %w", err))
				continue
			}
			s.setAsyncErr(fmt.Errorf("server error (%s): %s", payload.Code, payload.Message))
		}
	}
}

func (s *Session) writeLoop() {
	defer close(s.sendDone)

	for envelope := range s.sendQueue {
		s.writeMu.Lock()
		_ = s.conn.SetWriteDeadline(time.Now().Add(s.writeTimeout))
		err := s.conn.WriteJSON(envelope)
		s.writeMu.Unlock()
		if err != nil {
			s.setAsyncErr(fmt.Errorf("write envelope (%s): %w", envelope.Type, err))
			return
		}
	}
}

func (s *Session) SendInputCommand(command model.InputCommand) error {
	s.mu.RLock()
	sessionKind := s.sessionKind
	playerID := s.localPlayerID
	s.mu.RUnlock()

	if sessionKind != protocol.SessionKindPlayer {
		return ErrReadOnlySession
	}
	if playerID == "" {
		return ErrSessionClosed
	}

	command.PlayerID = playerID
	envelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: command,
	})
	if err != nil {
		return fmt.Errorf("netclient: build player_input envelope: %w", err)
	}

	return s.enqueueEnvelope(envelope)
}

func (s *Session) onAuthoritativeSnapshot(snapshot model.Snapshot) {
	if !s.store.ApplySnapshot(snapshot) {
		return
	}

	s.updateServerAck(snapshot)
	s.sendSnapshotAck(snapshot.TickID)
}

func (s *Session) updateServerAck(snapshot model.Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.localPlayerID == "" {
		return
	}

	ackSeq, found := findAckedClientSeq(snapshot.PlayerAcks, s.localPlayerID)
	if !found {
		return
	}
	if ackSeq > s.lastServerAckSeq {
		s.lastServerAckSeq = ackSeq
	}
	if snapshot.TickID > s.lastServerAckTick {
		s.lastServerAckTick = snapshot.TickID
	}
}

func (s *Session) sendSnapshotAck(snapshotTick uint64) {
	s.mu.Lock()
	if s.sessionKind != protocol.SessionKindPlayer || s.localPlayerID == "" {
		s.mu.Unlock()
		return
	}
	if snapshotTick == 0 || snapshotTick <= s.lastSnapshotAckTick {
		s.mu.Unlock()
		return
	}

	ackPayload := protocol.SnapshotAckMessage{
		ClientTickID:           snapshotTick,
		LastProcessedClientSeq: s.lastServerAckSeq,
	}
	s.lastSnapshotAckTick = snapshotTick
	s.mu.Unlock()

	envelope, err := protocol.NewEnvelope(protocol.MsgAckSnapshot, ackPayload)
	if err != nil {
		s.setAsyncErr(fmt.Errorf("build ack_snapshot envelope: %w", err))
		return
	}

	err = s.enqueueEnvelope(envelope)
	if err == nil || errors.Is(err, ErrSessionClosed) || errors.Is(err, ErrSendQueueFull) {
		return
	}
	s.setAsyncErr(fmt.Errorf("enqueue ack_snapshot envelope: %w", err))
}

func (s *Session) enqueueEnvelope(envelope protocol.Envelope) (err error) {
	select {
	case <-s.done:
		return ErrSessionClosed
	default:
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			err = ErrSessionClosed
		}
	}()

	select {
	case s.sendQueue <- envelope:
		return nil
	default:
		return ErrSendQueueFull
	}
}

func (s *Session) setAsyncErr(err error) {
	if err == nil {
		return
	}

	s.mu.Lock()
	s.lastAsyncErr = err
	s.mu.Unlock()
}

func findAckedClientSeq(acks []model.PlayerAck, playerID model.PlayerID) (uint64, bool) {
	for _, ack := range acks {
		if ack.PlayerID == playerID {
			return ack.LastProcessedClientSeq, true
		}
	}
	return 0, false
}
