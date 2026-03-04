package networking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"prison-break/internal/gamecore/roles"
	"prison-break/internal/server/auth"
	"prison-break/internal/server/game"
	"prison-break/internal/server/matchmaking"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

const (
	maxJoinPlayerNameLength    = 32
	maxJoinPlayerIDLength      = 64
	maxPreferredMatchIDLength  = 64
	maxJoinClientBuildLength   = 64
	maxJoinSessionTokenLength  = 4096
	maxEnvelopeRequestIDLength = 128
	maxRegionLatencyQueryBytes = 512
)

type wsConnection struct {
	id                     string
	socket                 *websocket.Conn
	send                   chan protocol.Envelope
	remoteAddr             string
	connectedAt            time.Time
	lastSeenAt             time.Time
	lastSnapshotTick       uint64
	lastClientAckTick      uint64
	lastClientAckClientSeq uint64
	playerID               model.PlayerID
	viewerID               model.PlayerID
	matchID                model.MatchID
	sessionKind            protocol.SessionKind
	authSubject            string
	authScope              auth.Scope
	state                  ConnectionState

	server    *Server
	closeOnce sync.Once
}

func (c *wsConnection) snapshot() ConnectionSnapshot {
	return ConnectionSnapshot{
		ConnectionID: c.id,
		State:        c.state,
		RemoteAddr:   c.remoteAddr,
		ConnectedAt:  c.connectedAt,
		LastSeenAt:   c.lastSeenAt,
		PlayerID:     c.playerID,
		ViewerID:     c.viewerID,
		MatchID:      c.matchID,
		SessionKind:  c.sessionKind,
		AuthSubject:  c.authSubject,
		AuthScope:    c.authScope,
	}
}

func (c *wsConnection) close(reason string) {
	c.closeOnce.Do(func() {
		c.server.unregisterConnection(c.id, reason)
		_ = c.socket.Close()
	})
}

type Server struct {
	mu sync.RWMutex

	manager    *game.Manager
	matchmaker *matchmaking.Service
	config     Config
	upgrader   websocket.Upgrader
	now        func() time.Time
	auth       *auth.TokenService
	authErr    error

	nextConnectionSequence uint64
	connections            map[string]*wsConnection
	playerToConnection     map[model.PlayerID]string
	matchToConnections     map[model.MatchID]map[string]struct{}
	events                 []ConnectionEvent

	snapshotDispatchCancel context.CancelFunc
	snapshotDispatchDone   chan struct{}
}

func NewServer(manager *game.Manager, config Config) *Server {
	if manager == nil {
		manager = game.NewManager(game.DefaultConfig())
	}

	normalized := config.normalized()

	server := &Server{
		manager:    manager,
		matchmaker: matchmaking.NewService(manager),
		config:     normalized,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(_ *http.Request) bool {
				// TODO: restrict origins when auth/session model is introduced.
				return true
			},
		},
		now:                time.Now,
		connections:        make(map[string]*wsConnection),
		playerToConnection: make(map[model.PlayerID]string),
		matchToConnections: make(map[model.MatchID]map[string]struct{}),
		events:             make([]ConnectionEvent, 0, 256),
	}
	if normalized.RequireAuth {
		server.auth, server.authErr = auth.NewTokenService(normalized.AuthSecret, normalized.AuthClockSkew)
	}

	loopCtx, cancel := context.WithCancel(context.Background())
	server.snapshotDispatchCancel = cancel
	server.snapshotDispatchDone = make(chan struct{})
	go server.snapshotDispatchLoop(loopCtx, server.snapshotDispatchDone)

	return server
}

func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	socket, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	conn := s.registerConnection(socket, r.RemoteAddr)
	go s.writePump(conn)
	s.readPump(conn)
}

func (s *Server) HandleLobbiesHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.config.RequireAuth {
		_, status, message := s.authorizeRequestHeader(r, auth.ScopeLobby)
		if status != http.StatusOK {
			http.Error(w, message, status)
			return
		}
	}

	includeRunning := false
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get("include_running"))) {
	case "true", "1", "yes":
		includeRunning = true
	}
	queueRequest := matchmaking.QueueRequest{
		PreferredRegion: strings.TrimSpace(r.URL.Query().Get("preferred_region")),
		RegionLatencyMS: parseRegionLatencyQueryString(r.URL.Query().Get("region_latency_ms")),
	}
	if validationErr := matchmaking.ValidateQueueRequest(queueRequest); validationErr != nil {
		http.Error(w, validationErr.Error(), http.StatusBadRequest)
		return
	}

	payload := protocol.LobbyListMessage{
		Lobbies: s.lobbySummariesForRequest(includeRunning, queueRequest),
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
}

func (s *Server) Shutdown(_ context.Context) error {
	connectionIDs := make([]string, 0, s.ConnectionCount())
	s.mu.RLock()
	for connectionID := range s.connections {
		connectionIDs = append(connectionIDs, connectionID)
	}
	s.mu.RUnlock()

	for _, connectionID := range connectionIDs {
		_ = s.CloseConnection(connectionID, "server_shutdown")
	}

	if s.snapshotDispatchCancel != nil {
		s.snapshotDispatchCancel()
	}
	if s.snapshotDispatchDone != nil {
		<-s.snapshotDispatchDone
	}

	return nil
}

func (s *Server) CloseConnection(connectionID string, reason string) error {
	s.mu.RLock()
	conn, exists := s.connections[connectionID]
	s.mu.RUnlock()
	if !exists {
		return ErrConnectionNotFound
	}

	conn.close(reason)
	return nil
}

func (s *Server) ConnectionCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.connections)
}

func (s *Server) ConnectionSnapshots() []ConnectionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ConnectionSnapshot, 0, len(s.connections))
	for _, conn := range s.connections {
		out = append(out, conn.snapshot())
	}

	sort.Slice(out, func(i int, j int) bool {
		return out[i].ConnectionID < out[j].ConnectionID
	})

	return out
}

func (s *Server) ConnectionEvents() []ConnectionEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]ConnectionEvent, len(s.events))
	copy(out, s.events)
	return out
}

func (s *Server) BroadcastToMatch(matchID model.MatchID, messageType protocol.MessageType, payload any) error {
	envelope, err := protocol.NewEnvelope(messageType, payload)
	if err != nil {
		return err
	}

	connections := s.connectionsForMatch(matchID)
	for _, conn := range connections {
		if queueErr := s.enqueueEnvelope(conn, envelope); queueErr != nil {
			return queueErr
		}
	}

	return nil
}

func (s *Server) registerConnection(socket *websocket.Conn, remoteAddr string) *wsConnection {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextConnectionSequence++
	connectionID := fmt.Sprintf("conn-%06d", s.nextConnectionSequence)
	connectedAt := s.now().UTC()

	conn := &wsConnection{
		id:          connectionID,
		socket:      socket,
		send:        make(chan protocol.Envelope, s.config.SendQueueDepth),
		remoteAddr:  remoteAddr,
		connectedAt: connectedAt,
		lastSeenAt:  connectedAt,
		state:       ConnectionStateConnected,
		server:      s,
	}

	s.connections[connectionID] = conn
	s.appendEventLocked(ConnectionEvent{
		Type:         ConnectionEventConnected,
		ConnectionID: connectionID,
		At:           connectedAt,
		Detail:       remoteAddr,
	})

	return conn
}

func (s *Server) unregisterConnection(connectionID string, reason string) {
	var (
		disconnectedPlayerID model.PlayerID
		disconnectedMatchID  model.MatchID
	)

	s.mu.Lock()
	conn, exists := s.connections[connectionID]
	if !exists {
		s.mu.Unlock()
		return
	}

	delete(s.connections, connectionID)
	if conn.playerID != "" {
		disconnectedPlayerID = conn.playerID
		disconnectedMatchID = conn.matchID
		if currentConnectionID, mapped := s.playerToConnection[conn.playerID]; mapped && currentConnectionID == connectionID {
			delete(s.playerToConnection, conn.playerID)
		}
	}
	if conn.matchID != "" {
		matchConnections, exists := s.matchToConnections[conn.matchID]
		if exists {
			delete(matchConnections, connectionID)
			if len(matchConnections) == 0 {
				delete(s.matchToConnections, conn.matchID)
			}
		}
	}

	conn.state = ConnectionStateDisconnected
	conn.lastSeenAt = s.now().UTC()
	close(conn.send)

	s.appendEventLocked(ConnectionEvent{
		Type:         ConnectionEventDisconnected,
		ConnectionID: connectionID,
		MatchID:      conn.matchID,
		PlayerID:     conn.playerID,
		At:           conn.lastSeenAt,
		Detail:       strings.TrimSpace(reason),
	})
	s.mu.Unlock()

	if disconnectedPlayerID != "" && disconnectedMatchID != "" {
		_ = s.manager.SetPlayerConnected(disconnectedMatchID, disconnectedPlayerID, false)
	}
}

func (s *Server) readPump(conn *wsConnection) {
	socket := conn.socket
	socket.SetReadLimit(s.config.ReadLimitBytes)
	_ = socket.SetReadDeadline(time.Now().Add(s.config.PongTimeout))
	socket.SetPongHandler(func(_ string) error {
		_ = socket.SetReadDeadline(time.Now().Add(s.config.PongTimeout))
		s.touchConnection(conn.id)
		return nil
	})

	for {
		_, raw, err := socket.ReadMessage()
		if err != nil {
			conn.close("read_closed")
			return
		}

		s.touchConnection(conn.id)

		var envelope protocol.Envelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			s.appendEvent(ConnectionEvent{
				Type:         ConnectionEventProtocolError,
				ConnectionID: conn.id,
				At:           s.now().UTC(),
				Detail:       "invalid_json_envelope",
			})
			_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid JSON envelope")
			continue
		}
		if validationErr := validateInboundEnvelope(envelope); validationErr != nil {
			s.appendEvent(ConnectionEvent{
				Type:         ConnectionEventProtocolError,
				ConnectionID: conn.id,
				At:           s.now().UTC(),
				Detail:       validationErr.Error(),
			})
			if errors.Is(validationErr, errUnsupportedProtocolVersion) {
				_ = s.sendProtocolError(conn, protocol.ErrUnsupportedVersion, validationErr.Error())
			} else {
				_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, validationErr.Error())
			}
			continue
		}

		s.appendEvent(ConnectionEvent{
			Type:         ConnectionEventMessageIn,
			ConnectionID: conn.id,
			PlayerID:     conn.playerID,
			MatchID:      conn.matchID,
			MessageType:  envelope.Type,
			At:           s.now().UTC(),
		})

		s.handleEnvelope(conn, envelope)
	}
}

func (s *Server) writePump(conn *wsConnection) {
	pingTicker := time.NewTicker(s.config.PingInterval)
	defer pingTicker.Stop()

	for {
		select {
		case envelope, ok := <-conn.send:
			if !ok {
				return
			}

			if err := conn.socket.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout)); err != nil {
				conn.close("write_deadline_failed")
				return
			}
			if err := conn.socket.WriteJSON(envelope); err != nil {
				conn.close("write_failed")
				return
			}

			s.touchConnection(conn.id)
			s.appendEvent(ConnectionEvent{
				Type:         ConnectionEventMessageOut,
				ConnectionID: conn.id,
				PlayerID:     conn.playerID,
				MatchID:      conn.matchID,
				MessageType:  envelope.Type,
				At:           s.now().UTC(),
			})

		case <-pingTicker.C:
			if err := conn.socket.WriteControl(
				websocket.PingMessage,
				[]byte("ping"),
				time.Now().Add(s.config.WriteTimeout),
			); err != nil {
				conn.close("ping_failed")
				return
			}
		}
	}
}

func (s *Server) handleEnvelope(conn *wsConnection, envelope protocol.Envelope) {
	switch envelope.Type {
	case protocol.MsgJoinGame:
		s.handleJoinGame(conn, envelope)
	case protocol.MsgListLobbies:
		s.handleListLobbies(conn, envelope)
	case protocol.MsgRequestReplay:
		s.handleRequestReplay(conn, envelope)
	case protocol.MsgLeaveMatch:
		conn.close("client_leave")
	case protocol.MsgPing:
		s.handlePing(conn, envelope)
	case protocol.MsgPlayerInput:
		s.handlePlayerInput(conn, envelope)
	case protocol.MsgAbilityUse:
		s.handleAbilityUse(conn, envelope)
	case protocol.MsgCardUse:
		s.handleCardUse(conn, envelope)
	case protocol.MsgAckSnapshot:
		s.handleAckSnapshot(conn, envelope)
	default:
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "unsupported message type")
	}
}

func (s *Server) handleJoinGame(conn *wsConnection, envelope protocol.Envelope) {
	req, err := protocol.DecodePayload[protocol.JoinGameRequest](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid join_game payload")
		return
	}
	if validationErr := validateJoinRequest(req); validationErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, validationErr.Error())
		return
	}
	if req.Spectator {
		claims, authErr := s.verifySessionTokenForScope(req.SessionToken, auth.ScopeGameplay)
		if authErr != nil {
			_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
			return
		}
		if s.config.RequireAuth {
			if !strings.EqualFold(claims.SessionKind, string(protocol.SessionKindSpectator)) {
				_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "session token is not valid for spectator joins")
				return
			}
		}
		s.handleJoinGameAsSpectator(conn, req, claims)
		return
	}

	playerID, claims, resolveErr := s.resolveJoinPlayerIdentity(conn, envelope, req)
	if resolveErr != nil {
		errorCode := protocol.ErrInvalidPayload
		if s.config.RequireAuth {
			errorCode = protocol.ErrUnauthorized
		}
		_ = s.sendProtocolError(conn, errorCode, resolveErr.Error())
		return
	}

	matchID := req.PreferredMatchID
	if s.config.RequireAuth && strings.TrimSpace(claims.MatchID) != "" {
		matchID = model.MatchID(strings.TrimSpace(claims.MatchID))
	}
	quickPlayQueueRequest := matchmaking.QueueRequest{
		PlayerID:        playerID,
		PreferredRegion: req.PreferredRegion,
		RegionLatencyMS: req.RegionLatencyMS,
	}
	if matchID == "" {
		if existingMatchID, exists := s.manager.MatchIDForPlayer(playerID); exists {
			matchID = existingMatchID
		} else {
			matchID = s.matchmaker.FindOrCreateLobbyForRequest(quickPlayQueueRequest).MatchID
		}
	}
	allowFallbackLobbySelection := req.PreferredMatchID == "" && (!s.config.RequireAuth || strings.TrimSpace(claims.MatchID) == "")

	snapshot, joinErr := s.manager.JoinMatch(matchID, playerID, req.PlayerName)
	if errors.Is(joinErr, game.ErrPlayerAlreadyInMatch) {
		snapshot, joinErr = s.manager.ResumePlayer(matchID, playerID, req.PlayerName)
	}
	if joinErr != nil && allowFallbackLobbySelection &&
		(errors.Is(joinErr, game.ErrMatchFull) ||
			errors.Is(joinErr, game.ErrMatchNotJoinable) ||
			errors.Is(joinErr, game.ErrMatchNotFound)) {
		fallbackMatchID := s.matchmaker.FindOrCreateLobbyForRequest(matchmaking.QueueRequest{
			PlayerID:        quickPlayQueueRequest.PlayerID,
			PreferredRegion: quickPlayQueueRequest.PreferredRegion,
			RegionLatencyMS: quickPlayQueueRequest.RegionLatencyMS,
			ExcludeMatchIDs: []model.MatchID{matchID},
		}).MatchID
		if fallbackMatchID != matchID {
			matchID = fallbackMatchID
			snapshot, joinErr = s.manager.JoinMatch(matchID, playerID, req.PlayerName)
			if errors.Is(joinErr, game.ErrPlayerAlreadyInMatch) {
				snapshot, joinErr = s.manager.ResumePlayer(matchID, playerID, req.PlayerName)
			}
		}
	}
	if joinErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(joinErr), joinErr.Error())
		return
	}

	s.bindConnection(conn.id, snapshot.MatchID, playerID, playerID, protocol.SessionKindPlayer, claims.Subject, claims.Scope)
	minPlayers, maxPlayers := s.manager.MatchConstraints()

	accepted := protocol.JoinGameAccepted{
		MatchID:         snapshot.MatchID,
		PlayerID:        playerID,
		MinPlayers:      minPlayers,
		MaxPlayers:      maxPlayers,
		TickRateHz:      s.manager.TickRateHz(),
		SessionKind:     protocol.SessionKindPlayer,
		CanSendCommands: true,
	}
	_ = s.sendTyped(conn, protocol.MsgJoinAccepted, accepted)
	fullSnapshot, snapshotErr := s.manager.FullSnapshot(snapshot.MatchID)
	if snapshotErr == nil {
		visibleSnapshot := roles.ProjectSnapshotForViewer(fullSnapshot, playerID)
		if sendErr := s.sendTyped(conn, protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: visibleSnapshot}); sendErr == nil {
			s.updateSnapshotCursor(conn.id, visibleSnapshot.TickID)
		}
	}

	s.maybeAutoStartMatch(snapshot.MatchID)
}

func (s *Server) handleJoinGameAsSpectator(conn *wsConnection, req protocol.JoinGameRequest, claims auth.Claims) {
	matchID := req.PreferredMatchID
	if matchID == "" {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "spectator join requires preferred_match_id")
		return
	}
	if s.config.RequireAuth {
		if claims.MatchID != "" && claims.MatchID != string(matchID) {
			_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "session token match does not match requested match")
			return
		}
	}

	snapshot, exists := s.manager.MatchSnapshot(matchID)
	if !exists {
		_ = s.sendProtocolError(conn, protocol.ErrMatchNotFound, game.ErrMatchNotFound.Error())
		return
	}
	if snapshot.Status != model.MatchStatusRunning {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "spectator join is only available for running matches")
		return
	}

	fullSnapshot, snapshotErr := s.manager.FullSnapshot(matchID)
	if snapshotErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(snapshotErr), snapshotErr.Error())
		return
	}
	followPlayerID, followSlot, slotCount := resolveSpectatorFollowTarget(
		fullSnapshot,
		req.SpectatorFollowPlayerID,
		req.SpectatorFollowSlot,
	)

	viewerID := model.PlayerID("spectator-" + conn.id)
	s.bindConnection(conn.id, matchID, "", viewerID, protocol.SessionKindSpectator, claims.Subject, claims.Scope)
	minPlayers, maxPlayers := s.manager.MatchConstraints()

	accepted := protocol.JoinGameAccepted{
		MatchID:                 matchID,
		MinPlayers:              minPlayers,
		MaxPlayers:              maxPlayers,
		TickRateHz:              s.manager.TickRateHz(),
		SessionKind:             protocol.SessionKindSpectator,
		CanSendCommands:         false,
		SpectatorFollowPlayerID: followPlayerID,
		SpectatorFollowSlot:     followSlot,
		SpectatorSlotCount:      slotCount,
	}
	_ = s.sendTyped(conn, protocol.MsgJoinAccepted, accepted)

	visibleSnapshot := roles.ProjectSnapshotForViewer(fullSnapshot, viewerID)
	if sendErr := s.sendTyped(conn, protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: visibleSnapshot}); sendErr == nil {
		s.updateSnapshotCursor(conn.id, visibleSnapshot.TickID)
	}
	_ = s.sendTyped(conn, protocol.MsgGameStart, protocol.GameStartMessage{
		MatchID:         matchID,
		StartTickID:     visibleSnapshot.TickID,
		InitialSnapshot: visibleSnapshot,
	})
}

func (s *Server) handleListLobbies(conn *wsConnection, envelope protocol.Envelope) {
	req, err := protocol.DecodePayload[protocol.ListLobbiesRequest](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid list_lobbies payload")
		return
	}
	queueRequest := matchmaking.QueueRequest{
		PreferredRegion: req.PreferredRegion,
		RegionLatencyMS: req.RegionLatencyMS,
	}
	if validationErr := matchmaking.ValidateQueueRequest(queueRequest); validationErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, validationErr.Error())
		return
	}
	if s.config.RequireAuth {
		claims, authErr := s.verifySessionTokenForScope(req.SessionToken, auth.ScopeLobby)
		if authErr != nil {
			_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
			return
		}
		s.setConnectionAuthContext(conn.id, claims.Subject, claims.Scope)
	}

	out := s.lobbySummariesForRequest(req.IncludeRunning, queueRequest)
	_ = s.sendTyped(conn, protocol.MsgLobbyList, protocol.LobbyListMessage{
		Lobbies: out,
	})
}

func (s *Server) handleRequestReplay(conn *wsConnection, envelope protocol.Envelope) {
	req, err := protocol.DecodePayload[protocol.ReplayLogRequest](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid request_replay payload")
		return
	}

	matchID := req.MatchID
	if matchID == "" {
		matchID = conn.matchID
	}
	if matchID == "" {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "join_game required before replay requests")
		return
	}
	if envelope.MatchID != "" && envelope.MatchID != matchID {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "envelope match_id does not match requested replay match")
		return
	}
	if conn.matchID != "" && matchID != conn.matchID {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "requested match does not match bound connection match")
		return
	}
	if authErr := s.authorizeReplayRequest(conn, matchID); authErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
		return
	}

	replay, replayErr := s.manager.ReplayLog(matchID)
	if replayErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(replayErr), replayErr.Error())
		return
	}

	_ = s.sendTyped(conn, protocol.MsgReplayLog, replayLogToProtocolMessage(replay))
}

func (s *Server) LobbySummaries(includeRunning bool) []protocol.LobbySummary {
	return s.lobbySummariesForRequest(includeRunning, matchmaking.QueueRequest{})
}

func (s *Server) lobbySummariesForRequest(
	includeRunning bool,
	request matchmaking.QueueRequest,
) []protocol.LobbySummary {
	lobbies := s.matchmaker.ListLobbiesForRequest(includeRunning, request)
	out := make([]protocol.LobbySummary, 0, len(lobbies))
	for _, lobby := range lobbies {
		out = append(out, protocol.LobbySummary{
			MatchID:      lobby.MatchID,
			Region:       lobby.Region,
			Status:       lobby.Status,
			PlayerCount:  lobby.PlayerCount,
			MinPlayers:   lobby.MinPlayers,
			MaxPlayers:   lobby.MaxPlayers,
			OpenSlots:    lobby.OpenSlots,
			Joinable:     lobby.Joinable,
			ReadyToStart: lobby.ReadyToStart,
			CreatedAt:    lobby.CreatedAt,
		})
	}

	return out
}

func (s *Server) handlePing(conn *wsConnection, envelope protocol.Envelope) {
	req, err := protocol.DecodePayload[protocol.PingMessage](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid ping payload")
		return
	}

	response := protocol.PongMessage{
		ClientSendUnixMS: req.ClientSendUnixMS,
		ServerSendUnixMS: s.now().UTC().UnixMilli(),
	}
	_ = s.sendTyped(conn, protocol.MsgPong, response)
}

func (s *Server) handlePlayerInput(conn *wsConnection, envelope protocol.Envelope) {
	if authErr := s.authorizeGameplayCommand(conn, envelope); authErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
		return
	}

	message, err := protocol.DecodePayload[protocol.PlayerInputMessage](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid player_input payload")
		return
	}

	command := message.Command
	if command.PlayerID != "" && command.PlayerID != conn.playerID {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, "command player_id does not match connection player_id")
		return
	}
	command.PlayerID = conn.playerID

	if _, submitErr := s.manager.SubmitInput(conn.matchID, command); submitErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(submitErr), submitErr.Error())
		return
	}
}

func (s *Server) handleAbilityUse(conn *wsConnection, envelope protocol.Envelope) {
	if authErr := s.authorizeGameplayCommand(conn, envelope); authErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
		return
	}

	message, err := protocol.DecodePayload[protocol.AbilityUseMessage](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid ability_use payload")
		return
	}

	rawPayload, err := json.Marshal(message.Payload)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid ability payload")
		return
	}

	command := model.InputCommand{
		PlayerID:  conn.playerID,
		ClientSeq: message.ClientSeq,
		Type:      model.CmdUseAbility,
		Payload:   rawPayload,
	}
	if _, submitErr := s.manager.SubmitInput(conn.matchID, command); submitErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(submitErr), submitErr.Error())
		return
	}
}

func (s *Server) handleCardUse(conn *wsConnection, envelope protocol.Envelope) {
	if authErr := s.authorizeGameplayCommand(conn, envelope); authErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
		return
	}

	message, err := protocol.DecodePayload[protocol.CardUseMessage](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid card_use payload")
		return
	}

	rawPayload, err := json.Marshal(message.Payload)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid card payload")
		return
	}

	command := model.InputCommand{
		PlayerID:  conn.playerID,
		ClientSeq: message.ClientSeq,
		Type:      model.CmdUseCard,
		Payload:   rawPayload,
	}
	if _, submitErr := s.manager.SubmitInput(conn.matchID, command); submitErr != nil {
		_ = s.sendProtocolError(conn, mapDomainErrorCode(submitErr), submitErr.Error())
		return
	}
}

func (s *Server) handleAckSnapshot(conn *wsConnection, envelope protocol.Envelope) {
	if authErr := s.authorizeGameplayCommand(conn, envelope); authErr != nil {
		_ = s.sendProtocolError(conn, protocol.ErrUnauthorized, authErr.Error())
		return
	}

	ack, err := protocol.DecodePayload[protocol.SnapshotAckMessage](envelope)
	if err != nil {
		_ = s.sendProtocolError(conn, protocol.ErrInvalidPayload, "invalid ack_snapshot payload")
		return
	}

	s.mu.Lock()
	activeConn, exists := s.connections[conn.id]
	if exists {
		if ack.ClientTickID > activeConn.lastClientAckTick {
			activeConn.lastClientAckTick = ack.ClientTickID
		}
		if ack.LastProcessedClientSeq > activeConn.lastClientAckClientSeq {
			activeConn.lastClientAckClientSeq = ack.LastProcessedClientSeq
		}
	}
	s.mu.Unlock()
}

func (s *Server) authorizeGameplayCommand(conn *wsConnection, envelope protocol.Envelope) error {
	if conn.matchID == "" || conn.playerID == "" || conn.sessionKind != protocol.SessionKindPlayer {
		return errors.New("join_game required before gameplay commands")
	}
	if envelope.MatchID != "" && envelope.MatchID != conn.matchID {
		return errors.New("envelope match_id does not match connection match_id")
	}
	if envelope.PlayerID != "" && envelope.PlayerID != conn.playerID {
		return errors.New("envelope player_id does not match connection player_id")
	}
	if !s.config.RequireAuth {
		return nil
	}
	if conn.authScope != auth.ScopeGameplay && conn.authScope != auth.ScopeAdmin {
		return errors.New("session token scope does not allow gameplay commands")
	}
	if strings.TrimSpace(conn.authSubject) == "" {
		return errors.New("authenticated player identity is missing")
	}
	if model.PlayerID(strings.TrimSpace(conn.authSubject)) != conn.playerID {
		return errors.New("session token subject does not match bound player")
	}
	return nil
}

func (s *Server) authorizeReplayRequest(conn *wsConnection, matchID model.MatchID) error {
	if conn.matchID == "" {
		return errors.New("join_game required before replay requests")
	}
	if conn.matchID != matchID {
		return errors.New("requested match does not match bound connection match")
	}
	if !s.config.RequireAuth {
		return nil
	}
	if conn.authScope != auth.ScopeGameplay && conn.authScope != auth.ScopeAdmin {
		return errors.New("session token scope does not allow replay access")
	}
	return nil
}

func (s *Server) resolveJoinPlayerIdentity(
	conn *wsConnection,
	envelope protocol.Envelope,
	req protocol.JoinGameRequest,
) (model.PlayerID, auth.Claims, error) {
	if s.config.RequireAuth {
		claims, err := s.verifySessionTokenForScope(req.SessionToken, auth.ScopeGameplay)
		if err != nil {
			return "", auth.Claims{}, err
		}
		if !strings.EqualFold(claims.SessionKind, string(protocol.SessionKindPlayer)) {
			return "", auth.Claims{}, errors.New("session token is not valid for player joins")
		}
		tokenPlayerID := strings.TrimSpace(claims.Subject)
		if validateErr := validatePlayerID(tokenPlayerID); validateErr != nil {
			return "", auth.Claims{}, errors.New("session token subject is invalid")
		}

		if envelope.PlayerID != "" {
			envelopePlayerID := strings.TrimSpace(string(envelope.PlayerID))
			if model.PlayerID(envelopePlayerID) != model.PlayerID(tokenPlayerID) {
				return "", auth.Claims{}, errors.New("session token subject does not match envelope player_id")
			}
		}
		if req.PreferredMatchID != "" && claims.MatchID != "" && claims.MatchID != string(req.PreferredMatchID) {
			return "", auth.Claims{}, errors.New("session token match does not match requested match")
		}
		return model.PlayerID(tokenPlayerID), claims, nil
	}

	playerID := strings.TrimSpace(string(envelope.PlayerID))
	if playerID == "" {
		playerID = "player-" + conn.id
	}
	if validateErr := validatePlayerID(playerID); validateErr != nil {
		return "", auth.Claims{}, validateErr
	}
	return model.PlayerID(playerID), auth.Claims{}, nil
}

func (s *Server) verifySessionTokenForScope(sessionToken string, scope auth.Scope) (auth.Claims, error) {
	if !s.config.RequireAuth {
		return auth.Claims{}, nil
	}
	if s.auth == nil {
		if s.authErr != nil {
			return auth.Claims{}, fmt.Errorf("authentication is unavailable: %w", s.authErr)
		}
		return auth.Claims{}, errors.New("authentication is unavailable")
	}

	trimmedToken := strings.TrimSpace(sessionToken)
	if trimmedToken == "" {
		return auth.Claims{}, errors.New("session token is required")
	}
	claims, err := s.auth.Verify(trimmedToken)
	if err != nil {
		return auth.Claims{}, errors.New("invalid or expired session token")
	}
	if !claims.Allows(scope) {
		return auth.Claims{}, errors.New("session token scope is not authorized")
	}
	return claims, nil
}

func (s *Server) authorizeRequestHeader(r *http.Request, scope auth.Scope) (auth.Claims, int, string) {
	if !s.config.RequireAuth {
		return auth.Claims{}, http.StatusOK, ""
	}
	if s.auth == nil {
		return auth.Claims{}, http.StatusServiceUnavailable, "authentication is unavailable"
	}

	token, parseErr := bearerTokenFromHeader(r.Header.Get("Authorization"))
	if parseErr != nil {
		return auth.Claims{}, http.StatusUnauthorized, parseErr.Error()
	}

	claims, err := s.auth.Verify(token)
	if err != nil {
		return auth.Claims{}, http.StatusUnauthorized, "invalid or expired bearer token"
	}
	if !claims.Allows(scope) {
		return auth.Claims{}, http.StatusForbidden, "insufficient token scope"
	}

	return claims, http.StatusOK, ""
}

func (s *Server) setConnectionAuthContext(connectionID string, subject string, scope auth.Scope) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[connectionID]
	if !exists {
		return
	}
	conn.authSubject = strings.TrimSpace(subject)
	conn.authScope = scope
}

func bearerTokenFromHeader(headerValue string) (string, error) {
	trimmed := strings.TrimSpace(headerValue)
	if trimmed == "" {
		return "", errors.New("missing bearer token")
	}

	parts := strings.Fields(trimmed)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", errors.New("invalid bearer token")
	}
	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", errors.New("missing bearer token")
	}
	return token, nil
}

var (
	errUnsupportedProtocolVersion = errors.New("unsupported protocol version")
	errMissingMessageType         = errors.New("message type is required")
)

func validateInboundEnvelope(envelope protocol.Envelope) error {
	if envelope.Version != constants.ProtocolVersion {
		return fmt.Errorf(
			"%w: got=%d want=%d",
			errUnsupportedProtocolVersion,
			envelope.Version,
			constants.ProtocolVersion,
		)
	}
	if strings.TrimSpace(string(envelope.Type)) == "" {
		return errMissingMessageType
	}
	if len(strings.TrimSpace(envelope.RequestID)) > maxEnvelopeRequestIDLength {
		return fmt.Errorf("request_id exceeds %d characters", maxEnvelopeRequestIDLength)
	}
	return nil
}

func validateJoinRequest(req protocol.JoinGameRequest) error {
	playerName := strings.TrimSpace(req.PlayerName)
	if !req.Spectator && playerName == "" {
		return errors.New("player_name is required")
	}
	if len(playerName) > maxJoinPlayerNameLength {
		return fmt.Errorf("player_name exceeds %d characters", maxJoinPlayerNameLength)
	}
	if len(strings.TrimSpace(string(req.PreferredMatchID))) > maxPreferredMatchIDLength {
		return fmt.Errorf("preferred_match_id exceeds %d characters", maxPreferredMatchIDLength)
	}
	if len(strings.TrimSpace(req.ClientBuild)) > maxJoinClientBuildLength {
		return fmt.Errorf("client_build exceeds %d characters", maxJoinClientBuildLength)
	}
	if len(strings.TrimSpace(req.SessionToken)) > maxJoinSessionTokenLength {
		return fmt.Errorf("session_token exceeds %d characters", maxJoinSessionTokenLength)
	}
	if !req.Spectator && (req.SpectatorFollowPlayerID != "" || req.SpectatorFollowSlot > 0) {
		return errors.New("spectator follow options require spectator=true")
	}
	if req.SpectatorFollowSlot > constants.MaxPlayers {
		return fmt.Errorf("spectator_follow_slot exceeds max player count %d", constants.MaxPlayers)
	}
	if req.SpectatorFollowPlayerID != "" {
		if validateErr := validatePlayerID(strings.TrimSpace(string(req.SpectatorFollowPlayerID))); validateErr != nil {
			return fmt.Errorf("spectator_follow_player_id is invalid: %w", validateErr)
		}
	}
	if validationErr := matchmaking.ValidateQueueRequest(matchmaking.QueueRequest{
		PreferredRegion: req.PreferredRegion,
		RegionLatencyMS: req.RegionLatencyMS,
	}); validationErr != nil {
		return validationErr
	}
	return nil
}

func parseRegionLatencyQueryString(raw string) map[string]uint16 {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > maxRegionLatencyQueryBytes {
		return nil
	}

	parts := strings.Split(trimmed, ",")
	out := make(map[string]uint16, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}

		pair := strings.SplitN(token, ":", 2)
		if len(pair) != 2 {
			continue
		}

		region := matchmaking.NormalizeRegionID(pair[0])
		if region == "" {
			continue
		}

		value := strings.TrimSpace(pair[1])
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseUint(value, 10, 16)
		if err != nil {
			continue
		}
		latency := uint16(parsed)
		if latency == 0 || latency > matchmaking.MaxRegionLatencyMS {
			continue
		}
		out[region] = latency
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

func validatePlayerID(playerID string) error {
	trimmed := strings.TrimSpace(playerID)
	if trimmed == "" {
		return errors.New("player_id is required")
	}
	if len(trimmed) > maxJoinPlayerIDLength {
		return fmt.Errorf("player_id exceeds %d characters", maxJoinPlayerIDLength)
	}
	if !isSafeIdentifier(trimmed) {
		return errors.New("player_id contains unsupported characters")
	}
	return nil
}

func isSafeIdentifier(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

func resolveSpectatorFollowTarget(
	snapshot model.Snapshot,
	requestedPlayerID model.PlayerID,
	requestedSlot uint8,
) (model.PlayerID, uint8, uint8) {
	if snapshot.State == nil || len(snapshot.State.Players) == 0 {
		return "", 0, 0
	}

	players := append([]model.PlayerState(nil), snapshot.State.Players...)
	sort.Slice(players, func(i int, j int) bool {
		if players[i].Alive != players[j].Alive {
			return players[i].Alive
		}
		return players[i].ID < players[j].ID
	})
	slotCount := uint8(len(players))

	trimmedRequestedID := model.PlayerID(strings.TrimSpace(string(requestedPlayerID)))
	if trimmedRequestedID != "" {
		for index, player := range players {
			if player.ID == trimmedRequestedID {
				return player.ID, uint8(index + 1), slotCount
			}
		}
	}

	if requestedSlot > 0 && int(requestedSlot) <= len(players) {
		player := players[requestedSlot-1]
		return player.ID, requestedSlot, slotCount
	}

	return players[0].ID, 1, slotCount
}

func (s *Server) maybeAutoStartMatch(matchID model.MatchID) {
	started, err := s.manager.StartMatch(matchID)
	if err != nil {
		return
	}

	initialSnapshot, snapshotErr := s.manager.FullSnapshot(started.MatchID)
	if snapshotErr != nil {
		initialSnapshot = model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: started.TickID,
			State:  stateFromMatchSnapshot(started),
		}
	}

	connections := s.connectionsForMatch(matchID)
	for _, conn := range connections {
		visibleSnapshot := roles.ProjectSnapshotForViewer(initialSnapshot, conn.viewerID)
		payload := protocol.GameStartMessage{
			MatchID:         started.MatchID,
			LocalPlayerID:   conn.playerID,
			StartTickID:     started.TickID,
			InitialSnapshot: visibleSnapshot,
		}
		if err := s.sendTyped(conn, protocol.MsgGameStart, payload); err == nil {
			s.updateSnapshotCursor(conn.id, visibleSnapshot.TickID)
		}
	}
}

func stateFromMatchSnapshot(snapshot game.MatchSnapshot) *model.GameState {
	players := make([]model.PlayerState, 0, len(snapshot.Players))
	for _, player := range snapshot.Players {
		players = append(players, model.PlayerState{
			ID:        player.PlayerID,
			Name:      player.Name,
			Connected: true,
			Alive:     true,
			Position:  model.Vector2{},
			Velocity:  model.Vector2{},
			Facing: model.Vector2{
				X: 1,
				Y: 0,
			},
		})
	}

	return &model.GameState{
		MatchID: snapshot.MatchID,
		TickID:  snapshot.TickID,
		Status:  snapshot.Status,
		Players: players,
	}
}

func mapDomainErrorCode(err error) protocol.ErrorCode {
	switch {
	case errors.Is(err, game.ErrInvalidPlayerID), errors.Is(err, game.ErrInvalidPlayerName):
		return protocol.ErrInvalidPayload
	case errors.Is(err, game.ErrMatchNotFound):
		return protocol.ErrMatchNotFound
	case errors.Is(err, game.ErrMatchFull):
		return protocol.ErrRateLimited
	case errors.Is(err, game.ErrMatchNotJoinable), errors.Is(err, game.ErrPlayerAlreadyInMatch), errors.Is(err, game.ErrPlayerNotFound):
		return protocol.ErrUnauthorized
	case errors.Is(err, game.ErrMatchNotRunning), errors.Is(err, game.ErrInputPlayerMismatch):
		return protocol.ErrUnauthorized
	case errors.Is(err, game.ErrInvalidInputCommand), errors.Is(err, game.ErrInvalidInputPayload):
		return protocol.ErrInvalidPayload
	case errors.Is(err, game.ErrInputRateLimited):
		return protocol.ErrRateLimited
	case errors.Is(err, game.ErrDuplicateInput), errors.Is(err, game.ErrInputTooLateDropped):
		return protocol.ErrOutOfDateCommand
	default:
		return protocol.ErrUnknown
	}
}

func (s *Server) sendProtocolError(conn *wsConnection, code protocol.ErrorCode, message string) error {
	return s.sendTyped(conn, protocol.MsgError, protocol.ErrorMessage{
		Code:      code,
		Message:   message,
		Retryable: false,
	})
}

func (s *Server) sendTyped(conn *wsConnection, messageType protocol.MessageType, payload any) error {
	envelope, err := protocol.NewEnvelope(messageType, payload)
	if err != nil {
		return err
	}
	return s.enqueueEnvelope(conn, envelope)
}

func (s *Server) enqueueEnvelope(conn *wsConnection, envelope protocol.Envelope) (err error) {
	defer func() {
		recovered := recover()
		if recovered != nil {
			s.appendEvent(ConnectionEvent{
				Type:         ConnectionEventDeliveryError,
				ConnectionID: conn.id,
				PlayerID:     conn.playerID,
				MatchID:      conn.matchID,
				MessageType:  envelope.Type,
				At:           s.now().UTC(),
				Detail:       "send_on_closed_channel",
			})
			err = ErrConnectionNotFound
		}
	}()

	select {
	case conn.send <- envelope:
		return nil
	default:
		s.appendEvent(ConnectionEvent{
			Type:         ConnectionEventDeliveryError,
			ConnectionID: conn.id,
			PlayerID:     conn.playerID,
			MatchID:      conn.matchID,
			MessageType:  envelope.Type,
			At:           s.now().UTC(),
			Detail:       "send_queue_full",
		})
		conn.close("send_queue_full")
		return ErrSendQueueFull
	}
}

func (s *Server) snapshotDispatchLoop(ctx context.Context, done chan struct{}) {
	defer close(done)

	interval := snapshotDispatchInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.dispatchPendingSnapshots()
		}
	}
}

func snapshotDispatchInterval() time.Duration {
	if constants.ServerTickRateHz == 0 {
		return 33 * time.Millisecond
	}

	interval := time.Second / time.Duration(constants.ServerTickRateHz)
	if interval <= 0 {
		return time.Millisecond
	}
	return interval
}

type snapshotDispatchTarget struct {
	connectionID     string
	connection       *wsConnection
	matchID          model.MatchID
	lastSnapshotTick uint64
}

func (s *Server) dispatchPendingSnapshots() {
	s.mu.RLock()
	targets := make([]snapshotDispatchTarget, 0, len(s.connections))
	for _, conn := range s.connections {
		if conn.matchID == "" || conn.viewerID == "" {
			continue
		}

		targets = append(targets, snapshotDispatchTarget{
			connectionID:     conn.id,
			connection:       conn,
			matchID:          conn.matchID,
			lastSnapshotTick: conn.lastSnapshotTick,
		})
	}
	s.mu.RUnlock()

	for _, target := range targets {
		snapshots, err := s.manager.SnapshotsSince(target.matchID, target.lastSnapshotTick)
		if err != nil || len(snapshots) == 0 {
			continue
		}

		latestTick := target.lastSnapshotTick
		for _, snapshot := range snapshots {
			if snapshot.TickID <= latestTick {
				continue
			}
			visibleSnapshot := roles.ProjectSnapshotForViewer(snapshot, target.connection.viewerID)
			sendErr := s.sendTyped(target.connection, protocol.MsgSnapshot, protocol.SnapshotMessage{
				Snapshot: visibleSnapshot,
			})
			if sendErr != nil {
				break
			}
			latestTick = visibleSnapshot.TickID
		}

		if latestTick > target.lastSnapshotTick {
			s.updateSnapshotCursor(target.connectionID, latestTick)
		}
	}
}

func (s *Server) updateSnapshotCursor(connectionID string, tickID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, exists := s.connections[connectionID]
	if !exists {
		return
	}
	if tickID > conn.lastSnapshotTick {
		conn.lastSnapshotTick = tickID
	}
}

func (s *Server) bindConnection(
	connectionID string,
	matchID model.MatchID,
	playerID model.PlayerID,
	viewerID model.PlayerID,
	sessionKind protocol.SessionKind,
	authSubject string,
	authScope auth.Scope,
) {
	staleConnectionIDs := make([]string, 0, 1)

	s.mu.Lock()
	conn, exists := s.connections[connectionID]
	if !exists {
		s.mu.Unlock()
		return
	}

	if playerID != "" {
		if previousConnectionID, mapped := s.playerToConnection[playerID]; mapped && previousConnectionID != connectionID {
			if previousConn, found := s.connections[previousConnectionID]; found {
				if previousConn.matchID != "" {
					if previousSet, found := s.matchToConnections[previousConn.matchID]; found {
						delete(previousSet, previousConnectionID)
						if len(previousSet) == 0 {
							delete(s.matchToConnections, previousConn.matchID)
						}
					}
				}
				previousConn.playerID = ""
				previousConn.viewerID = ""
				previousConn.matchID = ""
				previousConn.sessionKind = ""
				previousConn.authSubject = ""
				previousConn.authScope = ""
				staleConnectionIDs = append(staleConnectionIDs, previousConnectionID)
			}
			delete(s.playerToConnection, playerID)
		}
	}

	if conn.matchID != "" {
		if existingSet, found := s.matchToConnections[conn.matchID]; found {
			delete(existingSet, connectionID)
			if len(existingSet) == 0 {
				delete(s.matchToConnections, conn.matchID)
			}
		}
	}
	if conn.playerID != "" {
		delete(s.playerToConnection, conn.playerID)
	}

	conn.matchID = matchID
	conn.playerID = playerID
	conn.viewerID = viewerID
	conn.sessionKind = sessionKind
	conn.authSubject = strings.TrimSpace(authSubject)
	conn.authScope = authScope
	conn.lastSnapshotTick = 0
	conn.lastClientAckTick = 0
	conn.lastClientAckClientSeq = 0

	if playerID != "" {
		s.playerToConnection[playerID] = connectionID
	}
	if _, exists := s.matchToConnections[matchID]; !exists {
		s.matchToConnections[matchID] = make(map[string]struct{})
	}
	s.matchToConnections[matchID][connectionID] = struct{}{}

	s.appendEventLocked(ConnectionEvent{
		Type:         ConnectionEventJoinedMatch,
		ConnectionID: connectionID,
		PlayerID:     playerID,
		MatchID:      matchID,
		At:           s.now().UTC(),
		Detail:       string(sessionKind),
	})
	s.mu.Unlock()

	for _, staleConnectionID := range staleConnectionIDs {
		_ = s.CloseConnection(staleConnectionID, "session_rebound")
	}

	if playerID != "" && matchID != "" {
		_ = s.manager.SetPlayerConnected(matchID, playerID, true)
	}
}

func (s *Server) connectionsForMatch(matchID model.MatchID) []*wsConnection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	connectionSet, exists := s.matchToConnections[matchID]
	if !exists {
		return nil
	}

	out := make([]*wsConnection, 0, len(connectionSet))
	for connectionID := range connectionSet {
		if conn, exists := s.connections[connectionID]; exists {
			out = append(out, conn)
		}
	}

	sort.Slice(out, func(i int, j int) bool {
		return out[i].id < out[j].id
	})

	return out
}

func (s *Server) touchConnection(connectionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if conn, exists := s.connections[connectionID]; exists {
		conn.lastSeenAt = s.now().UTC()
	}
}

func (s *Server) appendEvent(event ConnectionEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appendEventLocked(event)
}

func (s *Server) appendEventLocked(event ConnectionEvent) {
	s.events = append(s.events, event)
}
