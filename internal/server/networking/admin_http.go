package networking

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"prison-break/internal/server/auth"
	"prison-break/internal/server/balance"
	"prison-break/internal/server/game"
	"prison-break/internal/server/matchmaking"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

const (
	defaultAdminEventLimit = 100
	maxAdminEventLimit     = 2000
	maxAdminReasonLength   = 160
)

type adminOverviewResponse struct {
	GeneratedAt              time.Time                       `json:"generated_at"`
	AuthRequired             bool                            `json:"auth_required"`
	ConnectionCount          int                             `json:"connection_count"`
	PlayerConnectionCount    int                             `json:"player_connection_count"`
	SpectatorConnectionCount int                             `json:"spectator_connection_count"`
	MatchStatusCounts        map[model.MatchStatus]int       `json:"match_status_counts"`
	MatchCount               int                             `json:"match_count"`
	ConnectionEventCounts    map[ConnectionEventType]int     `json:"connection_event_counts"`
	LifecycleEventCounts     map[game.LifecycleEventType]int `json:"lifecycle_event_counts"`
	AbuseSignals             adminAbuseSignalSummary         `json:"abuse_signals"`
	QueueMetrics             matchmaking.QueueMetrics        `json:"queue_metrics"`
}

type adminAbuseSignalSummary struct {
	ProtocolErrorCount  int `json:"protocol_error_count"`
	DeliveryErrorCount  int `json:"delivery_error_count"`
	SendQueueFullCount  int `json:"send_queue_full_count"`
	SessionReboundCount int `json:"session_rebound_count"`
	ClientLeaveCount    int `json:"client_leave_count"`
}

type adminMatchListResponse struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Matches     []adminMatchSummary `json:"matches"`
}

type adminMatchSummary struct {
	Snapshot            game.MatchSnapshot `json:"snapshot"`
	ConnectionCount     int                `json:"connection_count"`
	PendingInputTotal   int                `json:"pending_input_total"`
	PendingInputByTick  map[uint64]int     `json:"pending_input_by_tick,omitempty"`
	LifecycleEventCount int                `json:"lifecycle_event_count"`
	ReplayEntryCount    int                `json:"replay_entry_count"`
}

type adminMatchDetailResponse struct {
	GeneratedAt        time.Time                 `json:"generated_at"`
	MatchID            model.MatchID             `json:"match_id"`
	Snapshot           game.MatchSnapshot        `json:"snapshot"`
	Connections        []ConnectionSnapshot      `json:"connections"`
	LifecycleEvents    []game.LifecycleEvent     `json:"lifecycle_events"`
	PendingInputByTick map[uint64]int            `json:"pending_input_by_tick"`
	Replay             protocol.ReplayLogMessage `json:"replay"`
}

type adminConnectionsResponse struct {
	GeneratedAt time.Time            `json:"generated_at"`
	Connections []ConnectionSnapshot `json:"connections"`
	Events      []ConnectionEvent    `json:"events,omitempty"`
}

type adminQueueResponse struct {
	GeneratedAt time.Time                        `json:"generated_at"`
	Metrics     matchmaking.QueueMetrics         `json:"metrics"`
	Entries     []matchmaking.QueueEntrySnapshot `json:"entries"`
}

type adminBalanceResponse struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Report      balance.Report `json:"report"`
}

type adminDisconnectRequest struct {
	Reason string `json:"reason,omitempty"`
}

type adminDisconnectResponse struct {
	Disconnected bool      `json:"disconnected"`
	ConnectionID string    `json:"connection_id,omitempty"`
	PlayerID     string    `json:"player_id,omitempty"`
	Reason       string    `json:"reason"`
	At           time.Time `json:"at"`
}

func (s *Server) HandleAdminHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminScope(w, r) {
		return
	}

	trimmedPath := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin"), "/")
	if trimmedPath == "" {
		s.writeAdminJSON(w, http.StatusOK, map[string]any{
			"endpoints": []string{
				"GET /admin/overview",
				"GET /admin/matches",
				"GET /admin/matches/{match_id}",
				"GET /admin/matches/{match_id}/replay?format=json|ndjson",
				"GET /admin/connections?include_events=true&event_limit=100",
				"GET /admin/queue",
				"GET /admin/balance",
				"POST /admin/connections/{connection_id}/disconnect",
				"POST /admin/players/{player_id}/disconnect",
			},
		})
		return
	}

	segments := strings.Split(trimmedPath, "/")
	switch segments[0] {
	case "overview":
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleAdminOverviewHTTP(w, r)
		return
	case "matches":
		s.handleAdminMatchesHTTP(w, r, segments)
		return
	case "connections":
		s.handleAdminConnectionsHTTP(w, r, segments)
		return
	case "queue":
		s.handleAdminQueueHTTP(w, r, segments)
		return
	case "balance":
		s.handleAdminBalanceHTTP(w, r, segments)
		return
	case "players":
		s.handleAdminPlayersHTTP(w, r, segments)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func (s *Server) requireAdminScope(w http.ResponseWriter, r *http.Request) bool {
	if !s.config.RequireAuth {
		return true
	}
	_, status, message := s.authorizeRequestHeader(r, auth.ScopeAdmin)
	if status != http.StatusOK {
		http.Error(w, message, status)
		return false
	}
	return true
}

func (s *Server) handleAdminOverviewHTTP(w http.ResponseWriter, _ *http.Request) {
	connections := s.ConnectionSnapshots()
	connectionEvents := s.ConnectionEvents()
	matches := s.manager.ListMatchSnapshots()
	lifecycleEvents := s.manager.LifecycleEvents("")

	matchStatusCounts := make(map[model.MatchStatus]int, 3)
	for _, snapshot := range matches {
		matchStatusCounts[snapshot.Status]++
	}

	connectionEventCounts := make(map[ConnectionEventType]int, 8)
	abuseSignals := adminAbuseSignalSummary{}
	for _, event := range connectionEvents {
		connectionEventCounts[event.Type]++
		switch event.Type {
		case ConnectionEventProtocolError:
			abuseSignals.ProtocolErrorCount++
		case ConnectionEventDeliveryError:
			abuseSignals.DeliveryErrorCount++
			if event.Detail == "send_queue_full" {
				abuseSignals.SendQueueFullCount++
			}
		case ConnectionEventDisconnected:
			if event.Detail == "session_rebound" {
				abuseSignals.SessionReboundCount++
			}
			if event.Detail == "client_leave" {
				abuseSignals.ClientLeaveCount++
			}
		}
	}

	lifecycleEventCounts := make(map[game.LifecycleEventType]int, 8)
	for _, event := range lifecycleEvents {
		lifecycleEventCounts[event.Type]++
	}

	playerConnectionCount := 0
	spectatorConnectionCount := 0
	for _, connection := range connections {
		if connection.PlayerID != "" {
			playerConnectionCount++
		}
		if connection.SessionKind == protocol.SessionKindSpectator {
			spectatorConnectionCount++
		}
	}

	s.writeAdminJSON(w, http.StatusOK, adminOverviewResponse{
		GeneratedAt:              s.now().UTC(),
		AuthRequired:             s.config.RequireAuth,
		ConnectionCount:          len(connections),
		PlayerConnectionCount:    playerConnectionCount,
		SpectatorConnectionCount: spectatorConnectionCount,
		MatchStatusCounts:        matchStatusCounts,
		MatchCount:               len(matches),
		ConnectionEventCounts:    connectionEventCounts,
		LifecycleEventCounts:     lifecycleEventCounts,
		AbuseSignals:             abuseSignals,
		QueueMetrics:             s.matchmaker.QueueMetrics(),
	})
}

func (s *Server) handleAdminMatchesHTTP(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleAdminMatchListHTTP(w)
		return
	}

	matchID := model.MatchID(strings.TrimSpace(segments[1]))
	if matchID == "" {
		http.Error(w, "match_id is required", http.StatusBadRequest)
		return
	}

	if len(segments) == 2 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleAdminMatchDetailHTTP(w, r, matchID)
		return
	}

	if len(segments) == 3 && segments[2] == "replay" {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleAdminReplayExportHTTP(w, r, matchID)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleAdminMatchListHTTP(w http.ResponseWriter) {
	matchSnapshots := s.manager.ListMatchSnapshots()
	connections := s.ConnectionSnapshots()
	lifecycleEvents := s.manager.LifecycleEvents("")

	connectionCountByMatch := make(map[model.MatchID]int)
	for _, connection := range connections {
		if connection.MatchID != "" {
			connectionCountByMatch[connection.MatchID]++
		}
	}

	lifecycleCountByMatch := make(map[model.MatchID]int)
	for _, event := range lifecycleEvents {
		lifecycleCountByMatch[event.MatchID]++
	}

	summaries := make([]adminMatchSummary, 0, len(matchSnapshots))
	for _, snapshot := range matchSnapshots {
		pendingInputByTick, _ := s.manager.PendingInputCounts(snapshot.MatchID)
		pendingInputTotal := 0
		for _, count := range pendingInputByTick {
			pendingInputTotal += count
		}
		replayLog, _ := s.manager.ReplayLog(snapshot.MatchID)

		summaries = append(summaries, adminMatchSummary{
			Snapshot:            snapshot,
			ConnectionCount:     connectionCountByMatch[snapshot.MatchID],
			PendingInputTotal:   pendingInputTotal,
			PendingInputByTick:  pendingInputByTick,
			LifecycleEventCount: lifecycleCountByMatch[snapshot.MatchID],
			ReplayEntryCount:    len(replayLog.Entries),
		})
	}

	sort.Slice(summaries, func(i int, j int) bool {
		return summaries[i].Snapshot.MatchID < summaries[j].Snapshot.MatchID
	})

	s.writeAdminJSON(w, http.StatusOK, adminMatchListResponse{
		GeneratedAt: s.now().UTC(),
		Matches:     summaries,
	})
}

func (s *Server) handleAdminMatchDetailHTTP(w http.ResponseWriter, r *http.Request, matchID model.MatchID) {
	snapshot, exists := s.manager.MatchSnapshot(matchID)
	if !exists {
		http.Error(w, game.ErrMatchNotFound.Error(), http.StatusNotFound)
		return
	}

	replayLog, replayErr := s.manager.ReplayLog(matchID)
	if replayErr != nil {
		http.Error(w, replayErr.Error(), http.StatusNotFound)
		return
	}

	pendingInputByTick, pendingErr := s.manager.PendingInputCounts(matchID)
	if pendingErr != nil {
		pendingInputByTick = map[uint64]int{}
	}

	lifecycleEvents := s.manager.LifecycleEvents(matchID)
	connectionSnapshots := s.connectionsForMatchSnapshot(matchID)
	if parseOptionalBoolQuery(r, "include_events", true) {
		// Keep lifecycle list sorted for predictable operator readout.
		sort.Slice(lifecycleEvents, func(i int, j int) bool {
			if lifecycleEvents[i].TickID != lifecycleEvents[j].TickID {
				return lifecycleEvents[i].TickID < lifecycleEvents[j].TickID
			}
			return lifecycleEvents[i].At.Before(lifecycleEvents[j].At)
		})
	}

	s.writeAdminJSON(w, http.StatusOK, adminMatchDetailResponse{
		GeneratedAt:        s.now().UTC(),
		MatchID:            matchID,
		Snapshot:           snapshot,
		Connections:        connectionSnapshots,
		LifecycleEvents:    lifecycleEvents,
		PendingInputByTick: pendingInputByTick,
		Replay:             replayLogToProtocolMessage(replayLog),
	})
}

func (s *Server) handleAdminReplayExportHTTP(w http.ResponseWriter, r *http.Request, matchID model.MatchID) {
	replayLog, replayErr := s.manager.ReplayLog(matchID)
	if replayErr != nil {
		http.Error(w, replayErr.Error(), http.StatusNotFound)
		return
	}
	message := replayLogToProtocolMessage(replayLog)

	if parseOptionalBoolQuery(r, "download", false) {
		filename := fmt.Sprintf("replay_%s_%d.json", message.MatchID, s.now().UTC().Unix())
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}

	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" || format == "json" {
		s.writeAdminJSON(w, http.StatusOK, message)
		return
	}
	if format != "ndjson" {
		http.Error(w, "unsupported replay export format", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	encoder := json.NewEncoder(w)

	meta := map[string]any{
		"type":         "meta",
		"match_id":     message.MatchID,
		"status":       message.Status,
		"tick_rate_hz": message.TickRateHz,
		"created_at":   message.CreatedAt,
		"started_at":   message.StartedAt,
		"ended_at":     message.EndedAt,
		"ended_reason": message.EndedReason,
		"entry_count":  len(message.Entries),
	}
	_ = encoder.Encode(meta)
	for _, entry := range message.Entries {
		row := map[string]any{
			"type":  "entry",
			"entry": entry,
		}
		_ = encoder.Encode(row)
	}
}

func (s *Server) handleAdminConnectionsHTTP(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		s.handleAdminConnectionListHTTP(w, r)
		return
	}

	if len(segments) == 3 && segments[2] == "disconnect" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		connectionID := strings.TrimSpace(segments[1])
		if connectionID == "" {
			http.Error(w, "connection_id is required", http.StatusBadRequest)
			return
		}
		s.handleAdminConnectionDisconnectHTTP(w, r, connectionID)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleAdminPlayersHTTP(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) == 3 && segments[2] == "disconnect" {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		playerID := model.PlayerID(strings.TrimSpace(segments[1]))
		if playerID == "" {
			http.Error(w, "player_id is required", http.StatusBadRequest)
			return
		}
		s.handleAdminPlayerDisconnectHTTP(w, r, playerID)
		return
	}

	http.NotFound(w, r)
}

func (s *Server) handleAdminQueueHTTP(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	s.writeAdminJSON(w, http.StatusOK, adminQueueResponse{
		GeneratedAt: s.now().UTC(),
		Metrics:     s.matchmaker.QueueMetrics(),
		Entries:     s.matchmaker.QueueSnapshot(),
	})
}

func (s *Server) handleAdminBalanceHTTP(w http.ResponseWriter, r *http.Request, segments []string) {
	if len(segments) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	matchSnapshots := s.manager.ListMatchSnapshots()
	fullStateByMatch := make(map[model.MatchID]model.GameState, len(matchSnapshots))
	replayByMatch := make(map[model.MatchID]game.MatchReplay, len(matchSnapshots))

	for _, snapshot := range matchSnapshots {
		fullSnapshot, fullErr := s.manager.FullSnapshot(snapshot.MatchID)
		if fullErr == nil && fullSnapshot.State != nil {
			fullStateByMatch[snapshot.MatchID] = *fullSnapshot.State
		}
		replay, replayErr := s.manager.ReplayLog(snapshot.MatchID)
		if replayErr == nil {
			replayByMatch[snapshot.MatchID] = replay
		}
	}

	report := balance.BuildReport(s.now().UTC(), matchSnapshots, fullStateByMatch, replayByMatch)
	s.writeAdminJSON(w, http.StatusOK, adminBalanceResponse{
		GeneratedAt: s.now().UTC(),
		Report:      report,
	})
}

func (s *Server) handleAdminConnectionListHTTP(w http.ResponseWriter, r *http.Request) {
	connections := s.ConnectionSnapshots()
	response := adminConnectionsResponse{
		GeneratedAt: s.now().UTC(),
		Connections: connections,
	}

	if parseOptionalBoolQuery(r, "include_events", false) {
		limit := parseBoundedIntQuery(r, "event_limit", defaultAdminEventLimit, 1, maxAdminEventLimit)
		events := s.ConnectionEvents()
		if len(events) > limit {
			events = append([]ConnectionEvent(nil), events[len(events)-limit:]...)
		}
		response.Events = events
	}

	s.writeAdminJSON(w, http.StatusOK, response)
}

func (s *Server) handleAdminConnectionDisconnectHTTP(w http.ResponseWriter, r *http.Request, connectionID string) {
	reason, err := decodeAdminDisconnectReason(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if closeErr := s.CloseConnection(connectionID, reason); closeErr != nil {
		if errors.Is(closeErr, ErrConnectionNotFound) {
			http.Error(w, closeErr.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		return
	}

	s.writeAdminJSON(w, http.StatusOK, adminDisconnectResponse{
		Disconnected: true,
		ConnectionID: connectionID,
		Reason:       reason,
		At:           s.now().UTC(),
	})
}

func (s *Server) handleAdminPlayerDisconnectHTTP(w http.ResponseWriter, r *http.Request, playerID model.PlayerID) {
	reason, err := decodeAdminDisconnectReason(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	connectionID, closeErr := s.ClosePlayerConnection(playerID, reason)
	if closeErr != nil {
		if errors.Is(closeErr, ErrConnectionNotFound) {
			http.Error(w, closeErr.Error(), http.StatusNotFound)
			return
		}
		http.Error(w, closeErr.Error(), http.StatusInternalServerError)
		return
	}

	s.writeAdminJSON(w, http.StatusOK, adminDisconnectResponse{
		Disconnected: true,
		ConnectionID: connectionID,
		PlayerID:     string(playerID),
		Reason:       reason,
		At:           s.now().UTC(),
	})
}

func decodeAdminDisconnectReason(r *http.Request) (string, error) {
	reason := "admin_disconnect"
	if r.Body == nil {
		return reason, nil
	}

	var request adminDisconnectRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil && !errors.Is(err, io.EOF) {
		return "", errors.New("invalid disconnect request payload")
	}
	if strings.TrimSpace(request.Reason) != "" {
		reason = strings.TrimSpace(request.Reason)
	}
	if len(reason) > maxAdminReasonLength {
		return "", fmt.Errorf("reason exceeds %d characters", maxAdminReasonLength)
	}
	return reason, nil
}

func (s *Server) writeAdminJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) ClosePlayerConnection(playerID model.PlayerID, reason string) (string, error) {
	trimmedPlayerID := model.PlayerID(strings.TrimSpace(string(playerID)))
	if trimmedPlayerID == "" {
		return "", ErrConnectionNotFound
	}

	s.mu.RLock()
	connectionID, exists := s.playerToConnection[trimmedPlayerID]
	s.mu.RUnlock()
	if !exists {
		return "", ErrConnectionNotFound
	}

	return connectionID, s.CloseConnection(connectionID, reason)
}

func (s *Server) connectionsForMatchSnapshot(matchID model.MatchID) []ConnectionSnapshot {
	connections := s.connectionsForMatch(matchID)
	out := make([]ConnectionSnapshot, 0, len(connections))
	for _, connection := range connections {
		out = append(out, connection.snapshot())
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i].ConnectionID < out[j].ConnectionID
	})
	return out
}

func replayLogToProtocolMessage(replay game.MatchReplay) protocol.ReplayLogMessage {
	entries := make([]protocol.ReplayInputEntry, 0, len(replay.Entries))
	for _, entry := range replay.Entries {
		entries = append(entries, protocol.ReplayInputEntry{
			AcceptedTick: entry.AcceptedTick,
			IngressSeq:   entry.IngressSeq,
			AcceptedAt:   entry.AcceptedAt,
			Command:      entry.Command,
		})
	}
	return protocol.ReplayLogMessage{
		MatchID:     replay.MatchID,
		Status:      replay.Status,
		TickRateHz:  replay.TickRateHz,
		CreatedAt:   replay.CreatedAt,
		StartedAt:   replay.StartedAt,
		EndedAt:     replay.EndedAt,
		EndedReason: replay.EndedReason,
		Entries:     entries,
	}
}

func parseOptionalBoolQuery(r *http.Request, key string, defaultValue bool) bool {
	value := strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key)))
	switch value {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return defaultValue
	}
}

func parseBoundedIntQuery(r *http.Request, key string, defaultValue int, min int, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}
