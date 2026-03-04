package networking

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"prison-break/internal/server/auth"
	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestJoinGameRejectsUnsupportedProtocolVersion(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "proto",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName: "Alice",
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	envelope.Version++
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode protocol error payload: %v", err)
	}
	if payload.Code != protocol.ErrUnsupportedVersion {
		t.Fatalf("expected unsupported_version code, got %s", payload.Code)
	}
}

func TestJoinGameAuthRequiredRejectsMissingSessionToken(t *testing.T) {
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auth-required",
	}, Config{
		RequireAuth: true,
		AuthSecret:  "test-secret",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName: "Alice",
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	envelope.PlayerID = "p1"
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode join rejection payload: %v", err)
	}
	if payload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized code for missing token, got %s", payload.Code)
	}
}

func TestJoinGameAuthRequiredAcceptsValidPlayerToken(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auth-ok",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "token-player",
		SessionKind: "player",
		Scope:       auth.ScopeGameplay,
	})

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:   "Alice",
		SessionToken: token,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	acceptedEnvelope := readUntilType(t, client, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode join_accepted payload: %v", err)
	}
	if accepted.PlayerID != "token-player" {
		t.Fatalf("expected token-derived player id token-player, got %s", accepted.PlayerID)
	}

	waitFor(t, "auth context bound", func() bool {
		snapshots := h.server.ConnectionSnapshots()
		return len(snapshots) == 1 &&
			snapshots[0].PlayerID == "token-player" &&
			snapshots[0].AuthSubject == "token-player" &&
			snapshots[0].AuthScope == auth.ScopeGameplay
	})

	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	commandEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build player_input envelope: %v", err)
	}
	writeEnvelope(t, client, commandEnvelope)

	waitFor(t, "player_input accepted for authenticated connection", func() bool {
		counts, pendingErr := h.manager.PendingInputCounts(accepted.MatchID)
		if pendingErr != nil {
			return false
		}
		total := 0
		for _, count := range counts {
			total += count
		}
		return total >= 1
	})
}

func TestJoinGameAuthRequiredRejectsMismatchedEnvelopePlayerID(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auth-mismatch",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "token-player",
		SessionKind: "player",
		Scope:       auth.ScopeGameplay,
	})

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:   "Alice",
		SessionToken: token,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	envelope.PlayerID = "spoofed-player"
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode join mismatch payload: %v", err)
	}
	if payload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized code, got %s", payload.Code)
	}
}

func TestJoinGameAuthRequiredRejectsInsufficientScope(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auth-scope",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "token-player",
		SessionKind: "player",
		Scope:       auth.ScopeLobby,
	})

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:   "Alice",
		SessionToken: token,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode join scope payload: %v", err)
	}
	if payload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized code for insufficient scope, got %s", payload.Code)
	}
}

func TestListLobbiesWSRequiresTokenWhenAuthEnabled(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "auth-list",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	_ = h.manager.CreateMatch()
	client := h.Dial(t)
	defer client.Close()

	listEnvelope, err := protocol.NewEnvelope(protocol.MsgListLobbies, protocol.ListLobbiesRequest{})
	if err != nil {
		t.Fatalf("build list_lobbies envelope: %v", err)
	}
	writeEnvelope(t, client, listEnvelope)

	rejected := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	rejectedPayload, err := protocol.DecodePayload[protocol.ErrorMessage](rejected)
	if err != nil {
		t.Fatalf("decode list_lobbies rejection payload: %v", err)
	}
	if rejectedPayload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized on list_lobbies without token, got %s", rejectedPayload.Code)
	}

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "lobby-reader",
		SessionKind: "player",
		Scope:       auth.ScopeLobby,
	})
	authorizedEnvelope, err := protocol.NewEnvelope(protocol.MsgListLobbies, protocol.ListLobbiesRequest{
		SessionToken: token,
	})
	if err != nil {
		t.Fatalf("build authorized list_lobbies envelope: %v", err)
	}
	writeEnvelope(t, client, authorizedEnvelope)

	authorized := readUntilType(t, client, protocol.MsgLobbyList, 2*time.Second)
	authorizedPayload, err := protocol.DecodePayload[protocol.LobbyListMessage](authorized)
	if err != nil {
		t.Fatalf("decode authorized list_lobbies payload: %v", err)
	}
	if len(authorizedPayload.Lobbies) == 0 {
		t.Fatalf("expected at least one lobby in authorized response")
	}
}

func TestLobbiesHTTPRequiresBearerTokenWhenAuthEnabled(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "auth-http",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	_ = h.manager.CreateMatch()
	client := &http.Client{Timeout: 2 * time.Second}

	unauthorizedRequest, err := http.NewRequest(http.MethodGet, h.httpServer.URL+"/lobbies", nil)
	if err != nil {
		t.Fatalf("build unauthorized lobbies request: %v", err)
	}
	unauthorizedResponse, err := client.Do(unauthorizedRequest)
	if err != nil {
		t.Fatalf("execute unauthorized lobbies request: %v", err)
	}
	defer unauthorizedResponse.Body.Close()
	if unauthorizedResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing bearer token, got %d", unauthorizedResponse.StatusCode)
	}

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "game-client",
		SessionKind: "player",
		Scope:       auth.ScopeGameplay,
	})
	authorizedRequest, err := http.NewRequest(http.MethodGet, h.httpServer.URL+"/lobbies", nil)
	if err != nil {
		t.Fatalf("build authorized lobbies request: %v", err)
	}
	authorizedRequest.Header.Set("Authorization", "Bearer "+token)

	authorizedResponse, err := client.Do(authorizedRequest)
	if err != nil {
		t.Fatalf("execute authorized lobbies request: %v", err)
	}
	defer authorizedResponse.Body.Close()
	if authorizedResponse.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for authorized lobbies request, got %d", authorizedResponse.StatusCode)
	}

	var payload protocol.LobbyListMessage
	if decodeErr := json.NewDecoder(authorizedResponse.Body).Decode(&payload); decodeErr != nil {
		t.Fatalf("decode authorized lobbies response: %v", decodeErr)
	}
	if len(payload.Lobbies) == 0 {
		t.Fatalf("expected lobby payload in authorized response")
	}
}

func newHarnessWithNetworkingConfig(t *testing.T, gameConfig game.Config, netConfig Config) *harness {
	t.Helper()

	manager := game.NewManager(gameConfig)
	config := netConfig.normalized()
	if config.PingInterval == 0 {
		config.PingInterval = 10 * time.Second
	}
	if config.PongTimeout == 0 {
		config.PongTimeout = 30 * time.Second
	}
	if config.WriteTimeout == 0 {
		config.WriteTimeout = 3 * time.Second
	}
	server := NewServer(manager, config)

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", server.HandleWebSocket)
	mux.HandleFunc("/lobbies", server.HandleLobbiesHTTP)
	mux.HandleFunc("/admin", server.HandleAdminHTTP)
	mux.HandleFunc("/admin/", server.HandleAdminHTTP)

	httpServer := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"

	return &harness{
		manager:    manager,
		server:     server,
		httpServer: httpServer,
		wsURL:      wsURL,
	}
}

func mustIssueToken(t *testing.T, secret string, claims auth.Claims) string {
	t.Helper()

	service, err := auth.NewTokenService(secret, 2*time.Second)
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}

	now := time.Now().UTC()
	normalized := claims
	if normalized.IssuedAt == 0 {
		normalized.IssuedAt = now.Add(-10 * time.Second).Unix()
	}
	if normalized.ExpiresAt == 0 {
		normalized.ExpiresAt = now.Add(time.Hour).Unix()
	}

	token, err := service.Sign(normalized)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestLobbiesHTTPReturnsForbiddenForInsufficientScope(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "auth-http-scope",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	client := &http.Client{Timeout: 2 * time.Second}
	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "spectator",
		SessionKind: "spectator",
		Scope:       "unknown_scope",
	})

	request, err := http.NewRequest(http.MethodGet, h.httpServer.URL+"/lobbies", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("execute request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for insufficient scope, got %d", response.StatusCode)
	}
}

func TestAuthBoundConnectionRejectsGameplayEnvelopePlayerMismatch(t *testing.T) {
	secret := "test-secret"
	h := newHarnessWithNetworkingConfig(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auth-cmd",
	}, Config{
		RequireAuth: true,
		AuthSecret:  secret,
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	token := mustIssueToken(t, secret, auth.Claims{
		Subject:     "token-player",
		SessionKind: "player",
		Scope:       auth.ScopeGameplay,
	})
	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:   "Alice",
		SessionToken: token,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	writeEnvelope(t, client, joinEnvelope)

	_ = readUntilType(t, client, protocol.MsgJoinAccepted, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	commandEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build player_input envelope: %v", err)
	}
	commandEnvelope.PlayerID = "spoofed-player"
	writeEnvelope(t, client, commandEnvelope)

	errorEnvelope := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	errorPayload, err := protocol.DecodePayload[protocol.ErrorMessage](errorEnvelope)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if errorPayload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized for envelope player_id mismatch, got %s", errorPayload.Code)
	}
}
