package networking

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"prison-break/internal/server/auth"
	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestAdminOverviewReportsOperationalMetrics(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-overview",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()
	matchID := joinClient(t, client, "player-1", "")
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	response := doAdminRequest(t, h, http.MethodGet, "/admin/overview", "", "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin overview, got %d", response.StatusCode)
	}

	var payload adminOverviewResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin overview payload: %v", err)
	}
	if payload.ConnectionCount < 1 {
		t.Fatalf("expected at least one active connection, got %d", payload.ConnectionCount)
	}
	if payload.PlayerConnectionCount < 1 {
		t.Fatalf("expected at least one player-bound connection, got %d", payload.PlayerConnectionCount)
	}
	if payload.MatchCount < 1 {
		t.Fatalf("expected at least one match in admin overview, got %d", payload.MatchCount)
	}
	if payload.MatchStatusCounts[model.MatchStatusRunning] < 1 {
		t.Fatalf("expected running match status count for %s", matchID)
	}
	if payload.LifecycleEventCounts[game.LifecycleEventMatchCreated] < 1 {
		t.Fatalf("expected lifecycle event counts in admin overview")
	}
}

func TestAdminMatchEndpointsExposeReplayAndExportFormats(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-match",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()
	matchID := joinClient(t, client, "player-1", "")
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	inputEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build player_input envelope: %v", err)
	}
	writeEnvelope(t, client, inputEnvelope)

	waitFor(t, "replay entry for admin replay export", func() bool {
		replay, replayErr := h.manager.ReplayLog(matchID)
		return replayErr == nil && len(replay.Entries) >= 1
	})

	matchListResponse := doAdminRequest(t, h, http.MethodGet, "/admin/matches", "", "")
	defer matchListResponse.Body.Close()
	if matchListResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin match list, got %d", matchListResponse.StatusCode)
	}
	var listPayload adminMatchListResponse
	if err := json.NewDecoder(matchListResponse.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode admin match list payload: %v", err)
	}
	if len(listPayload.Matches) == 0 {
		t.Fatalf("expected non-empty admin match list")
	}

	matchDetailResponse := doAdminRequest(t, h, http.MethodGet, "/admin/matches/"+string(matchID), "", "")
	defer matchDetailResponse.Body.Close()
	if matchDetailResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin match detail, got %d", matchDetailResponse.StatusCode)
	}
	var detailPayload adminMatchDetailResponse
	if err := json.NewDecoder(matchDetailResponse.Body).Decode(&detailPayload); err != nil {
		t.Fatalf("decode admin match detail payload: %v", err)
	}
	if detailPayload.MatchID != matchID {
		t.Fatalf("expected admin match detail for %s, got %s", matchID, detailPayload.MatchID)
	}
	if len(detailPayload.Replay.Entries) < 1 {
		t.Fatalf("expected replay entries in admin match detail")
	}

	replayJSONResponse := doAdminRequest(
		t,
		h,
		http.MethodGet,
		"/admin/matches/"+string(matchID)+"/replay?format=json",
		"",
		"",
	)
	defer replayJSONResponse.Body.Close()
	if replayJSONResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from replay json export, got %d", replayJSONResponse.StatusCode)
	}
	var replayJSON protocol.ReplayLogMessage
	if err := json.NewDecoder(replayJSONResponse.Body).Decode(&replayJSON); err != nil {
		t.Fatalf("decode replay json export: %v", err)
	}
	if len(replayJSON.Entries) < 1 {
		t.Fatalf("expected replay json export to include entries")
	}

	replayNDJSONResponse := doAdminRequest(
		t,
		h,
		http.MethodGet,
		"/admin/matches/"+string(matchID)+"/replay?format=ndjson",
		"",
		"",
	)
	defer replayNDJSONResponse.Body.Close()
	if replayNDJSONResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from replay ndjson export, got %d", replayNDJSONResponse.StatusCode)
	}
	bodyRaw, err := io.ReadAll(replayNDJSONResponse.Body)
	if err != nil {
		t.Fatalf("read replay ndjson body: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(bodyRaw)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected replay ndjson export to include meta and at least one entry line, got %d lines", len(lines))
	}
	if !strings.Contains(lines[0], `"type":"meta"`) {
		t.Fatalf("expected first ndjson row to be metadata, got %s", lines[0])
	}
	if !strings.Contains(lines[1], `"type":"entry"`) {
		t.Fatalf("expected second ndjson row to be replay entry, got %s", lines[1])
	}

	invalidFormatResponse := doAdminRequest(
		t,
		h,
		http.MethodGet,
		"/admin/matches/"+string(matchID)+"/replay?format=csv",
		"",
		"",
	)
	defer invalidFormatResponse.Body.Close()
	if invalidFormatResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported replay export format, got %d", invalidFormatResponse.StatusCode)
	}
}

func TestAdminConnectionsEndpointAndDisconnectAction(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-connections",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()
	_ = joinClient(t, clientA, "p1", "")
	_ = readUntilType(t, clientA, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, clientA, protocol.MsgGameStart, 2*time.Second)
	_ = joinClient(t, clientB, "p2", "")
	_ = readUntilType(t, clientB, protocol.MsgSnapshot, 2*time.Second)

	connectionResponse := doAdminRequest(
		t,
		h,
		http.MethodGet,
		"/admin/connections?include_events=true&event_limit=5",
		"",
		"",
	)
	defer connectionResponse.Body.Close()
	if connectionResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin connections endpoint, got %d", connectionResponse.StatusCode)
	}
	var connectionsPayload adminConnectionsResponse
	if err := json.NewDecoder(connectionResponse.Body).Decode(&connectionsPayload); err != nil {
		t.Fatalf("decode admin connections payload: %v", err)
	}
	if len(connectionsPayload.Connections) < 2 {
		t.Fatalf("expected at least two connections, got %d", len(connectionsPayload.Connections))
	}
	if len(connectionsPayload.Events) > 5 {
		t.Fatalf("expected capped event list <= 5, got %d", len(connectionsPayload.Events))
	}

	var targetConnectionID string
	for _, connection := range connectionsPayload.Connections {
		if connection.PlayerID == "p1" {
			targetConnectionID = connection.ConnectionID
			break
		}
	}
	if targetConnectionID == "" {
		t.Fatalf("expected to find connection for p1")
	}

	disconnectBody := `{"reason":"abuse_spam"}`
	disconnectResponse := doAdminRequest(
		t,
		h,
		http.MethodPost,
		"/admin/connections/"+targetConnectionID+"/disconnect",
		disconnectBody,
		"",
	)
	defer disconnectResponse.Body.Close()
	if disconnectResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin connection disconnect, got %d", disconnectResponse.StatusCode)
	}

	waitFor(t, "target connection disconnect by admin action", func() bool {
		snapshots := h.server.ConnectionSnapshots()
		for _, snapshot := range snapshots {
			if snapshot.ConnectionID == targetConnectionID {
				return false
			}
		}
		return true
	})

	disconnectMissing := doAdminRequest(
		t,
		h,
		http.MethodPost,
		"/admin/connections/"+targetConnectionID+"/disconnect",
		`{"reason":"abuse_spam"}`,
		"",
	)
	defer disconnectMissing.Body.Close()
	if disconnectMissing.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for already disconnected connection, got %d", disconnectMissing.StatusCode)
	}
}

func TestAdminPlayerDisconnectAction(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-player-disconnect",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()
	_ = joinClient(t, client, "kickme", "")
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	response := doAdminRequest(
		t,
		h,
		http.MethodPost,
		"/admin/players/kickme/disconnect",
		`{"reason":"moderation_kick"}`,
		"",
	)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin player disconnect, got %d", response.StatusCode)
	}

	waitFor(t, "player connection removed after moderation kick", func() bool {
		return h.server.ConnectionCount() == 0
	})
}

func TestAdminEndpointsRequireAdminScopeWhenAuthEnabled(t *testing.T) {
	secret := "admin-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-auth",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	noToken := doAdminRequest(t, h, http.MethodGet, "/admin/overview", "", "")
	defer noToken.Body.Close()
	if noToken.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for admin endpoint without token, got %d", noToken.StatusCode)
	}

	gameplayToken := mustIssueToken(t, secret, auth.Claims{
		Subject:     "player-1",
		SessionKind: "player",
		Scope:       auth.ScopeGameplay,
	})
	gameplayAttempt := doAdminRequest(t, h, http.MethodGet, "/admin/overview", "", "Bearer "+gameplayToken)
	defer gameplayAttempt.Body.Close()
	if gameplayAttempt.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin token on admin endpoint, got %d", gameplayAttempt.StatusCode)
	}

	adminToken := mustIssueToken(t, secret, auth.Claims{
		Subject:     "ops-1",
		SessionKind: "player",
		Scope:       auth.ScopeAdmin,
	})
	adminAttempt := doAdminRequest(t, h, http.MethodGet, "/admin/overview", "", "Bearer "+adminToken)
	defer adminAttempt.Body.Close()
	if adminAttempt.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for admin token on admin endpoint, got %d", adminAttempt.StatusCode)
	}
}

func TestAdminDisconnectRejectsOverlongReason(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-reason",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()
	_ = joinClient(t, client, "p1", "")
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	response := doAdminRequest(t, h, http.MethodGet, "/admin/connections", "", "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin connections endpoint, got %d", response.StatusCode)
	}
	var payload adminConnectionsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode connections payload: %v", err)
	}
	if len(payload.Connections) == 0 {
		t.Fatalf("expected at least one connection")
	}
	targetConnectionID := payload.Connections[0].ConnectionID

	overlongReason := strings.Repeat("x", maxAdminReasonLength+1)
	body, _ := json.Marshal(adminDisconnectRequest{Reason: overlongReason})
	badResponse := doAdminRequest(
		t,
		h,
		http.MethodPost,
		"/admin/connections/"+targetConnectionID+"/disconnect",
		string(body),
		"",
	)
	defer badResponse.Body.Close()
	if badResponse.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for overlong disconnect reason, got %d", badResponse.StatusCode)
	}
}

func TestAdminQueueEndpointReturnsMetricsAndEntries(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "admin-queue",
	})
	defer h.Close(t)

	response := doAdminRequest(t, h, http.MethodGet, "/admin/queue", "", "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin queue endpoint, got %d", response.StatusCode)
	}

	var payload adminQueueResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin queue payload: %v", err)
	}
	if payload.Metrics.QueuedTotal != 0 {
		t.Fatalf("expected no queued allocations before joins, got %d", payload.Metrics.QueuedTotal)
	}
	if len(payload.Entries) != 0 {
		t.Fatalf("expected empty queue entries before joins, got %d", len(payload.Entries))
	}
}

func TestAdminBalanceEndpointReturnsAggregatedReport(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "admin-balance",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()
	matchID := joinClient(t, client, "balance-player", "")
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	reloadEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build reload command envelope: %v", err)
	}
	writeEnvelope(t, client, reloadEnvelope)

	waitFor(t, "replay entry recorded for balance report", func() bool {
		replay, replayErr := h.manager.ReplayLog(matchID)
		return replayErr == nil && len(replay.Entries) >= 1
	})
	if _, err := h.manager.EndMatch(matchID, "playtest_end"); err != nil {
		t.Fatalf("end match for balance report: %v", err)
	}

	response := doAdminRequest(t, h, http.MethodGet, "/admin/balance", "", "")
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from admin balance endpoint, got %d", response.StatusCode)
	}

	var payload adminBalanceResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin balance payload: %v", err)
	}
	if payload.Report.MatchCount < 1 {
		t.Fatalf("expected at least one match in balance report, got %d", payload.Report.MatchCount)
	}
	if payload.Report.CompletedMatchCount < 1 {
		t.Fatalf("expected at least one completed match in balance report, got %d", payload.Report.CompletedMatchCount)
	}
	if payload.Report.CommandUsage[model.CmdReload] < 1 {
		t.Fatalf("expected reload command usage in balance report, got %+v", payload.Report.CommandUsage)
	}
	if len(payload.Report.Recommendations) == 0 {
		t.Fatalf("expected non-empty balance recommendations")
	}
}

func doAdminRequest(
	t *testing.T,
	h *harness,
	method string,
	path string,
	body string,
	authHeader string,
) *http.Response {
	t.Helper()

	client := &http.Client{Timeout: 3 * time.Second}
	requestBody := io.Reader(http.NoBody)
	if body != "" {
		requestBody = bytes.NewBufferString(body)
	}
	request, err := http.NewRequest(method, h.httpServer.URL+path, requestBody)
	if err != nil {
		t.Fatalf("build admin request: %v", err)
	}
	if strings.TrimSpace(authHeader) != "" {
		request.Header.Set("Authorization", strings.TrimSpace(authHeader))
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("execute admin request: %v", err)
	}
	return response
}
