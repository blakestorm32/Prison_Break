package netclient

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestDialAndJoinConsumesJoinAcceptedAndInitialSnapshot(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		if envelope.Type != protocol.MsgJoinGame {
			t.Fatalf("expected join_game message, got %s", envelope.Type)
		}

		join, err := protocol.DecodePayload[protocol.JoinGameRequest](envelope)
		if err != nil {
			t.Fatalf("decode join_game payload: %v", err)
		}
		if join.PlayerName != "Alice" {
			t.Fatalf("expected player name Alice, got %q", join.PlayerName)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-1",
			PlayerID:        "alice-id",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		full := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-1",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "alice-id", Name: "Alice", Alive: true},
				},
			},
		}
		snapshot, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: full,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshot); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}

		time.Sleep(200 * time.Millisecond)
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Alice",
		PlayerID:   "alice-client",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	if session.LocalPlayerID() != "alice-id" {
		t.Fatalf("expected local player id alice-id, got %s", session.LocalPlayerID())
	}
	if session.SessionKind() != protocol.SessionKindPlayer {
		t.Fatalf("expected player session kind, got %s", session.SessionKind())
	}
	if session.MatchID() != "match-1" {
		t.Fatalf("expected match id match-1, got %s", session.MatchID())
	}
	if session.MinPlayers() != 1 || session.MaxPlayers() != 6 {
		t.Fatalf("expected min/max players 1/6, got %d/%d", session.MinPlayers(), session.MaxPlayers())
	}

	state, ok := session.Store().CurrentState()
	if !ok {
		t.Fatalf("expected snapshot state after join")
	}
	if state.MatchID != "match-1" || state.TickID != 1 {
		t.Fatalf("unexpected state after join: %+v", state)
	}
	if len(state.Players) != 1 || state.Players[0].ID != "alice-id" {
		t.Fatalf("unexpected players after join: %+v", state.Players)
	}
}

func TestDialAndJoinAppliesGameStartInitialSnapshot(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-2",
			PlayerID:        "p2",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		initial := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 7,
			State: &model.GameState{
				MatchID: "match-2",
				TickID:  7,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "p2", Name: "P2", Alive: true},
				},
			},
		}
		gameStart, err := protocol.NewEnvelope(protocol.MsgGameStart, protocol.GameStartMessage{
			MatchID:         "match-2",
			LocalPlayerID:   "p2",
			StartTickID:     7,
			InitialSnapshot: initial,
		})
		if err != nil {
			t.Fatalf("build game_start envelope: %v", err)
		}
		if err := conn.WriteJSON(gameStart); err != nil {
			t.Fatalf("write game_start envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Bob",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	state, ok := session.Store().CurrentState()
	if !ok {
		t.Fatalf("expected state after game_start handshake")
	}
	if state.TickID != 7 || state.MatchID != "match-2" {
		t.Fatalf("unexpected game_start state: %+v", state)
	}
}

func TestDialAndJoinAppliesPostHandshakeDeltaStreaming(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-stream",
			PlayerID:        "stream-p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		full := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 10,
			State: &model.GameState{
				MatchID: "match-stream",
				TickID:  10,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "stream-p1", Name: "Stream", HeartsHalf: 6, Alive: true},
				},
			},
		}
		fullEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: full})
		if err != nil {
			t.Fatalf("build full snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(fullEnvelope); err != nil {
			t.Fatalf("write full snapshot envelope: %v", err)
		}

		time.Sleep(150 * time.Millisecond)

		hearts := uint8(4)
		delta := model.Snapshot{
			Kind:       model.SnapshotKindDelta,
			TickID:     11,
			BaseTickID: 10,
			Delta: &model.GameDelta{
				ChangedPlayers: []model.PlayerState{
					{ID: "stream-p1", Name: "Stream", HeartsHalf: hearts, Alive: true},
				},
			},
		}
		deltaEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: delta})
		if err != nil {
			t.Fatalf("build delta snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(deltaEnvelope); err != nil {
			t.Fatalf("write delta snapshot envelope: %v", err)
		}

		time.Sleep(250 * time.Millisecond)
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Stream",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := session.Store().CurrentState()
		if ok && state.TickID >= 11 && len(state.Players) == 1 && state.Players[0].HeartsHalf == 4 {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	state, _ := session.Store().CurrentState()
	t.Fatalf("timed out waiting for streamed delta apply, latest state=%+v", state)
}

func TestDialAndJoinReturnsServerJoinError(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		errEnvelope, err := protocol.NewEnvelope(protocol.MsgError, protocol.ErrorMessage{
			Code:      protocol.ErrUnauthorized,
			Message:   "join denied",
			Retryable: false,
		})
		if err != nil {
			t.Fatalf("build error envelope: %v", err)
		}
		if err := conn.WriteJSON(errEnvelope); err != nil {
			t.Fatalf("write error envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Denied",
	})
	if err == nil {
		t.Fatalf("expected join error from server")
	}
	if !strings.Contains(err.Error(), "join denied") {
		t.Fatalf("expected server error detail in join error, got %v", err)
	}
}

func TestDialAndJoinSupportsSpectatorSessionKind(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		join, err := protocol.DecodePayload[protocol.JoinGameRequest](envelope)
		if err != nil {
			t.Fatalf("decode join request payload: %v", err)
		}
		if !join.Spectator {
			t.Fatalf("expected spectator join request")
		}
		if join.SpectatorFollowPlayerID != "p2" {
			t.Fatalf("expected spectator follow player id p2 in join request, got %s", join.SpectatorFollowPlayerID)
		}
		if join.SpectatorFollowSlot != 2 {
			t.Fatalf("expected spectator follow slot 2 in join request, got %d", join.SpectatorFollowSlot)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:                 "match-spec",
			PlayerID:                "",
			MinPlayers:              1,
			MaxPlayers:              6,
			TickRateHz:              30,
			SessionKind:             protocol.SessionKindSpectator,
			CanSendCommands:         false,
			SpectatorFollowPlayerID: "p2",
			SpectatorFollowSlot:     2,
			SpectatorSlotCount:      4,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 5,
			State: &model.GameState{
				MatchID: "match-spec",
				TickID:  5,
				Status:  model.MatchStatusRunning,
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:               serverURL,
		PlayerName:              "Observer",
		Spectator:               true,
		PreferredMatchID:        "match-spec",
		SpectatorFollowPlayerID: "p2",
		SpectatorFollowSlot:     2,
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	if session.SessionKind() != protocol.SessionKindSpectator {
		t.Fatalf("expected spectator session kind, got %s", session.SessionKind())
	}
	if session.LocalPlayerID() != "" {
		t.Fatalf("expected empty local player id for spectator, got %s", session.LocalPlayerID())
	}
	if session.MinPlayers() != 1 || session.MaxPlayers() != 6 {
		t.Fatalf("expected spectator session min/max players 1/6, got %d/%d", session.MinPlayers(), session.MaxPlayers())
	}
	if session.SpectatorFollowPlayerID() != "p2" {
		t.Fatalf("expected spectator follow player id p2, got %s", session.SpectatorFollowPlayerID())
	}
	if session.SpectatorFollowSlot() != 2 || session.SpectatorSlotCount() != 4 {
		t.Fatalf(
			"expected spectator follow slot/count 2/4, got %d/%d",
			session.SpectatorFollowSlot(),
			session.SpectatorSlotCount(),
		)
	}
}

func newWebSocketHarness(t *testing.T, handler func(conn *websocket.Conn)) (string, func()) {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade websocket connection: %v", err)
		}
		handler(conn)
	})

	httpServer := httptest.NewServer(mux)
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/ws"
	return wsURL, httpServer.Close
}

func TestSessionConfigNormalizationDefaults(t *testing.T) {
	config := SessionConfig{
		ServerURL:        "   ",
		PlayerName:       "   ",
		HandshakeTimeout: 0,
	}
	normalized := config.normalized()

	if normalized.ServerURL != "ws://127.0.0.1:8080/ws" {
		t.Fatalf("expected default server URL, got %q", normalized.ServerURL)
	}
	if normalized.PlayerName != "Player" {
		t.Fatalf("expected default player name, got %q", normalized.PlayerName)
	}
	if normalized.HandshakeTimeout <= 0 {
		t.Fatalf("expected positive default handshake timeout, got %s", normalized.HandshakeTimeout)
	}
	if normalized.WriteTimeout <= 0 {
		t.Fatalf("expected positive default write timeout, got %s", normalized.WriteTimeout)
	}
	if normalized.SendQueueDepth <= 0 {
		t.Fatalf("expected positive default send queue depth, got %d", normalized.SendQueueDepth)
	}

	spectatorNormalized := SessionConfig{
		Spectator:               true,
		PlayerName:              " ",
		SpectatorFollowPlayerID: " p2 ",
		SpectatorFollowSlot:     3,
	}.normalized()
	if spectatorNormalized.PlayerName != "Spectator" {
		t.Fatalf("expected spectator default name, got %q", spectatorNormalized.PlayerName)
	}
	if spectatorNormalized.SpectatorFollowPlayerID != "p2" {
		t.Fatalf(
			"expected spectator follow player id normalization to trim spaces, got %q",
			spectatorNormalized.SpectatorFollowPlayerID,
		)
	}
	if spectatorNormalized.SpectatorFollowSlot != 3 {
		t.Fatalf("expected spectator follow slot to remain 3, got %d", spectatorNormalized.SpectatorFollowSlot)
	}
}

func TestDialAndJoinRejectsInvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := DialAndJoin(ctx, SessionConfig{
		ServerURL: "://bad-url",
	})
	if err == nil {
		t.Fatalf("expected invalid URL error")
	}
}

func TestDialAndJoinRejectsNonWebSocketScheme(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_, err := DialAndJoin(ctx, SessionConfig{
		ServerURL: "http://localhost:8080/ws",
	})
	if err == nil {
		t.Fatalf("expected non-websocket scheme error")
	}
}

func TestSessionReadLoopCapturesServerErrorMessage(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-err",
			PlayerID:        "p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-err",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{{ID: "p1", Alive: true}},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: snapshot})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}

		time.Sleep(150 * time.Millisecond)

		errEnvelope, err := protocol.NewEnvelope(protocol.MsgError, protocol.ErrorMessage{
			Code:      protocol.ErrRateLimited,
			Message:   "slow down",
			Retryable: true,
		})
		if err != nil {
			t.Fatalf("build runtime error envelope: %v", err)
		}
		if err := conn.WriteJSON(errEnvelope); err != nil {
			t.Fatalf("write runtime error envelope: %v", err)
		}

		time.Sleep(250 * time.Millisecond)
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "P1",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if session.LastAsyncError() != nil {
			if !strings.Contains(session.LastAsyncError().Error(), "slow down") {
				t.Fatalf("expected runtime server error to be captured, got %v", session.LastAsyncError())
			}
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for runtime server error capture")
}

func TestDialAndJoinPreservesJoinEnvelopePlayerID(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		if envelope.PlayerID != "provided-player-id" {
			t.Fatalf("expected join envelope player id provided-player-id, got %s", envelope.PlayerID)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-player-id",
			PlayerID:        "provided-player-id",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-player-id",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "provided-player-id", Alive: true},
				},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: snapshot})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "P1",
		PlayerID:   "provided-player-id",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	if session.LocalPlayerID() != "provided-player-id" {
		t.Fatalf("expected provided player id to remain local player id, got %s", session.LocalPlayerID())
	}
}

func TestDialAndJoinIncludesSessionTokenInJoinPayload(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		joinRequest, err := protocol.DecodePayload[protocol.JoinGameRequest](envelope)
		if err != nil {
			t.Fatalf("decode join payload: %v", err)
		}
		if joinRequest.SessionToken != "session-token-1" {
			t.Fatalf("expected session token in join payload, got %q", joinRequest.SessionToken)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-auth",
			PlayerID:        "p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-auth",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "p1", Alive: true},
				},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "P1",
		AuthToken:  " session-token-1 ",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()
}

func TestDialAndJoinIncludesRegionPreferencesInJoinPayload(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		joinRequest, err := protocol.DecodePayload[protocol.JoinGameRequest](envelope)
		if err != nil {
			t.Fatalf("decode join payload: %v", err)
		}
		if joinRequest.PreferredRegion != "us-east" {
			t.Fatalf("expected preferred region us-east, got %q", joinRequest.PreferredRegion)
		}
		if len(joinRequest.RegionLatencyMS) != 2 {
			t.Fatalf("expected two region latencies, got %d", len(joinRequest.RegionLatencyMS))
		}
		if joinRequest.RegionLatencyMS["us-east"] != 45 {
			t.Fatalf("expected us-east latency 45, got %d", joinRequest.RegionLatencyMS["us-east"])
		}
		if joinRequest.RegionLatencyMS["us-west"] != 90 {
			t.Fatalf("expected us-west latency 90, got %d", joinRequest.RegionLatencyMS["us-west"])
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-region",
			PlayerID:        "p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-region",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "p1", Alive: true},
				},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:       serverURL,
		PlayerName:      "P1",
		PreferredRegion: "US-East",
		RegionLatencyMS: map[string]uint16{
			"US-East": 45,
			"us-west": 90,
		},
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()
}

func TestSessionReadLoopIgnoresInvalidSnapshotPayloadWithoutDisconnect(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-invalid",
			PlayerID:        "p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		full := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-invalid",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "p1", HeartsHalf: 6, Alive: true},
				},
			},
		}
		fullEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: full})
		if err != nil {
			t.Fatalf("build full envelope: %v", err)
		}
		if err := conn.WriteJSON(fullEnvelope); err != nil {
			t.Fatalf("write full envelope: %v", err)
		}

		invalid := protocol.Envelope{
			Type:    protocol.MsgSnapshot,
			Payload: json.RawMessage(`{"snapshot":{"kind":"delta","tick_id":2,"delta":"bad"}}`),
		}
		if err := conn.WriteJSON(invalid); err != nil {
			t.Fatalf("write invalid snapshot envelope: %v", err)
		}

		time.Sleep(100 * time.Millisecond)

		delta := model.Snapshot{
			Kind:       model.SnapshotKindDelta,
			TickID:     2,
			BaseTickID: 1,
			Delta: &model.GameDelta{
				ChangedPlayers: []model.PlayerState{
					{ID: "p1", HeartsHalf: 5, Alive: true},
				},
			},
		}
		deltaEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: delta})
		if err != nil {
			t.Fatalf("build delta envelope: %v", err)
		}
		if err := conn.WriteJSON(deltaEnvelope); err != nil {
			t.Fatalf("write delta envelope: %v", err)
		}

		time.Sleep(250 * time.Millisecond)
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "P1",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state, ok := session.Store().CurrentState()
		if ok && state.TickID >= 2 && len(state.Players) == 1 && state.Players[0].HeartsHalf == 5 {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	state, _ := session.Store().CurrentState()
	t.Fatalf("timed out waiting for post-invalid snapshot recovery, latest state=%+v", state)
}

func TestSessionSendInputCommandDispatchesPlayerInputToServer(t *testing.T) {
	receivedCommand := make(chan model.InputCommand, 1)

	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}
		if envelope.Type != protocol.MsgJoinGame {
			t.Fatalf("expected join_game message, got %s", envelope.Type)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-send",
			PlayerID:        "player-send",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-send",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "player-send", Alive: true},
				},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var inputEnvelope protocol.Envelope
		if err := conn.ReadJSON(&inputEnvelope); err != nil {
			t.Fatalf("read player input envelope: %v", err)
		}
		if inputEnvelope.Type != protocol.MsgPlayerInput {
			t.Fatalf("expected player_input message, got %s", inputEnvelope.Type)
		}

		inputMessage, err := protocol.DecodePayload[protocol.PlayerInputMessage](inputEnvelope)
		if err != nil {
			t.Fatalf("decode player_input payload: %v", err)
		}
		receivedCommand <- inputMessage.Command
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Sender",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	movePayload, err := json.Marshal(model.MovementInputPayload{
		MoveX:  1,
		MoveY:  0,
		Sprint: true,
	})
	if err != nil {
		t.Fatalf("marshal movement payload: %v", err)
	}

	sendErr := session.SendInputCommand(model.InputCommand{
		PlayerID:   "spoofed-player-id",
		ClientSeq:  42,
		TargetTick: 7,
		Type:       model.CmdMoveIntent,
		Payload:    movePayload,
	})
	if sendErr != nil {
		t.Fatalf("send input command failed: %v", sendErr)
	}

	select {
	case got := <-receivedCommand:
		if got.PlayerID != "player-send" {
			t.Fatalf("expected server-authoritative player id player-send, got %s", got.PlayerID)
		}
		if got.ClientSeq != 42 {
			t.Fatalf("expected client seq 42, got %d", got.ClientSeq)
		}
		if got.TargetTick != 7 {
			t.Fatalf("expected target tick 7, got %d", got.TargetTick)
		}
		if got.Type != model.CmdMoveIntent {
			t.Fatalf("expected move_intent command type, got %s", got.Type)
		}

		var move model.MovementInputPayload
		if err := json.Unmarshal(got.Payload, &move); err != nil {
			t.Fatalf("unmarshal movement payload from sent command: %v", err)
		}
		if move.MoveX != 1 || move.MoveY != 0 || !move.Sprint {
			t.Fatalf("unexpected movement payload: %+v", move)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for server to receive player_input command")
	}
}

func TestSessionSendInputCommandRejectsReadOnlySpectator(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-read-only",
			PlayerID:        "",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindSpectator,
			CanSendCommands: false,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-read-only",
				TickID:  1,
				Status:  model.MatchStatusRunning,
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:        serverURL,
		PlayerName:       "Observer",
		Spectator:        true,
		PreferredMatchID: "match-read-only",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	err = session.SendInputCommand(model.InputCommand{
		ClientSeq:  1,
		TargetTick: 2,
		Type:       model.CmdReload,
	})
	if !errors.Is(err, ErrReadOnlySession) {
		t.Fatalf("expected ErrReadOnlySession, got %v", err)
	}
}

func TestSessionSendInputCommandReturnsSessionClosedAfterClose(t *testing.T) {
	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var envelope protocol.Envelope
		if err := conn.ReadJSON(&envelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-close",
			PlayerID:        "closer",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		snapshot := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-close",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "closer", Alive: true},
				},
			},
		}
		snapshotEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{
			Snapshot: snapshot,
		})
		if err != nil {
			t.Fatalf("build snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(snapshotEnvelope); err != nil {
			t.Fatalf("write snapshot envelope: %v", err)
		}

		time.Sleep(500 * time.Millisecond)
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "Closer",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close session: %v", err)
	}
	err = session.SendInputCommand(model.InputCommand{
		ClientSeq:  2,
		TargetTick: 3,
		Type:       model.CmdReload,
	})
	if !errors.Is(err, ErrSessionClosed) {
		t.Fatalf("expected ErrSessionClosed after close, got %v", err)
	}
}

func TestSessionSendInputCommandReturnsQueueFullWhenBackedUp(t *testing.T) {
	done := make(chan struct{})
	session := &Session{
		localPlayerID: "player-queue",
		sessionKind:   protocol.SessionKindPlayer,
		sendQueue:     make(chan protocol.Envelope, 1),
		done:          done,
	}

	firstErr := session.SendInputCommand(model.InputCommand{
		ClientSeq:  1,
		TargetTick: 9,
		Type:       model.CmdReload,
	})
	if firstErr != nil {
		t.Fatalf("expected first send to enter queue, got %v", firstErr)
	}

	secondErr := session.SendInputCommand(model.InputCommand{
		ClientSeq:  2,
		TargetTick: 10,
		Type:       model.CmdReload,
	})
	if !errors.Is(secondErr, ErrSendQueueFull) {
		t.Fatalf("expected ErrSendQueueFull on second send, got %v", secondErr)
	}
}

func TestSessionReadLoopSendsSnapshotAckAndTracksServerAck(t *testing.T) {
	ackPayloads := make(chan protocol.SnapshotAckMessage, 2)

	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var joinEnvelope protocol.Envelope
		if err := conn.ReadJSON(&joinEnvelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-ack",
			PlayerID:        "p1",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindPlayer,
			CanSendCommands: true,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		full := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-ack",
				TickID:  1,
				Status:  model.MatchStatusRunning,
				Players: []model.PlayerState{
					{ID: "p1", Alive: true},
				},
			},
		}
		fullEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: full})
		if err != nil {
			t.Fatalf("build full snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(fullEnvelope); err != nil {
			t.Fatalf("write full snapshot envelope: %v", err)
		}

		time.Sleep(100 * time.Millisecond)
		delta1 := model.Snapshot{
			Kind:       model.SnapshotKindDelta,
			TickID:     2,
			BaseTickID: 1,
			Delta: &model.GameDelta{
				ChangedPlayers: []model.PlayerState{
					{ID: "p1", Alive: true},
				},
			},
			PlayerAcks: []model.PlayerAck{
				{PlayerID: "p1", LastProcessedClientSeq: 4},
			},
		}
		delta1Envelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: delta1})
		if err != nil {
			t.Fatalf("build first delta snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(delta1Envelope); err != nil {
			t.Fatalf("write first delta snapshot envelope: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var ackEnvelope1 protocol.Envelope
		if err := conn.ReadJSON(&ackEnvelope1); err != nil {
			t.Fatalf("read first ack_snapshot envelope: %v", err)
		}
		if ackEnvelope1.Type != protocol.MsgAckSnapshot {
			t.Fatalf("expected first outgoing message to be ack_snapshot, got %s", ackEnvelope1.Type)
		}
		ack1, err := protocol.DecodePayload[protocol.SnapshotAckMessage](ackEnvelope1)
		if err != nil {
			t.Fatalf("decode first ack_snapshot payload: %v", err)
		}
		ackPayloads <- ack1

		delta2 := model.Snapshot{
			Kind:       model.SnapshotKindDelta,
			TickID:     3,
			BaseTickID: 2,
			Delta: &model.GameDelta{
				ChangedPlayers: []model.PlayerState{
					{ID: "p1", Alive: true},
				},
			},
			PlayerAcks: []model.PlayerAck{
				{PlayerID: "p1", LastProcessedClientSeq: 5},
			},
		}
		delta2Envelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: delta2})
		if err != nil {
			t.Fatalf("build second delta snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(delta2Envelope); err != nil {
			t.Fatalf("write second delta snapshot envelope: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		var ackEnvelope2 protocol.Envelope
		if err := conn.ReadJSON(&ackEnvelope2); err != nil {
			t.Fatalf("read second ack_snapshot envelope: %v", err)
		}
		if ackEnvelope2.Type != protocol.MsgAckSnapshot {
			t.Fatalf("expected second outgoing message to be ack_snapshot, got %s", ackEnvelope2.Type)
		}
		ack2, err := protocol.DecodePayload[protocol.SnapshotAckMessage](ackEnvelope2)
		if err != nil {
			t.Fatalf("decode second ack_snapshot payload: %v", err)
		}
		ackPayloads <- ack2
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:  serverURL,
		PlayerName: "P1",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	select {
	case first := <-ackPayloads:
		if first.ClientTickID != 2 {
			t.Fatalf("expected first ack client tick 2, got %d", first.ClientTickID)
		}
		if first.LastProcessedClientSeq != 4 {
			t.Fatalf("expected first ack last processed seq 4, got %d", first.LastProcessedClientSeq)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first ack_snapshot payload")
	}

	select {
	case second := <-ackPayloads:
		if second.ClientTickID != 3 {
			t.Fatalf("expected second ack client tick 3, got %d", second.ClientTickID)
		}
		if second.LastProcessedClientSeq != 5 {
			t.Fatalf("expected second ack last processed seq 5, got %d", second.LastProcessedClientSeq)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second ack_snapshot payload")
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if session.LastSnapshotAckTick() >= 3 &&
			session.LastServerAckedClientSeq() >= 5 &&
			session.LastServerAckTick() >= 3 {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}

	t.Fatalf(
		"timed out waiting for session ack tracking state: last_snapshot_ack_tick=%d last_server_ack_seq=%d last_server_ack_tick=%d",
		session.LastSnapshotAckTick(),
		session.LastServerAckedClientSeq(),
		session.LastServerAckTick(),
	)
}

func TestSessionReadLoopDoesNotSendSnapshotAckForSpectator(t *testing.T) {
	readResult := make(chan error, 1)

	serverURL, closeServer := newWebSocketHarness(t, func(conn *websocket.Conn) {
		defer conn.Close()

		var joinEnvelope protocol.Envelope
		if err := conn.ReadJSON(&joinEnvelope); err != nil {
			t.Fatalf("read join envelope: %v", err)
		}

		accepted, err := protocol.NewEnvelope(protocol.MsgJoinAccepted, protocol.JoinGameAccepted{
			MatchID:         "match-spec-ack",
			PlayerID:        "",
			MinPlayers:      1,
			MaxPlayers:      6,
			TickRateHz:      30,
			SessionKind:     protocol.SessionKindSpectator,
			CanSendCommands: false,
		})
		if err != nil {
			t.Fatalf("build join accepted envelope: %v", err)
		}
		if err := conn.WriteJSON(accepted); err != nil {
			t.Fatalf("write join accepted envelope: %v", err)
		}

		full := model.Snapshot{
			Kind:   model.SnapshotKindFull,
			TickID: 1,
			State: &model.GameState{
				MatchID: "match-spec-ack",
				TickID:  1,
				Status:  model.MatchStatusRunning,
			},
		}
		fullEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: full})
		if err != nil {
			t.Fatalf("build full snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(fullEnvelope); err != nil {
			t.Fatalf("write full snapshot envelope: %v", err)
		}

		time.Sleep(100 * time.Millisecond)
		delta := model.Snapshot{
			Kind:       model.SnapshotKindDelta,
			TickID:     2,
			BaseTickID: 1,
			Delta:      &model.GameDelta{},
		}
		deltaEnvelope, err := protocol.NewEnvelope(protocol.MsgSnapshot, protocol.SnapshotMessage{Snapshot: delta})
		if err != nil {
			t.Fatalf("build delta snapshot envelope: %v", err)
		}
		if err := conn.WriteJSON(deltaEnvelope); err != nil {
			t.Fatalf("write delta snapshot envelope: %v", err)
		}

		_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		var outgoing protocol.Envelope
		readErr := conn.ReadJSON(&outgoing)
		readResult <- readErr
	})
	defer closeServer()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	session, err := DialAndJoin(ctx, SessionConfig{
		ServerURL:        serverURL,
		PlayerName:       "Observer",
		Spectator:        true,
		PreferredMatchID: "match-spec-ack",
	})
	if err != nil {
		t.Fatalf("dial and join failed: %v", err)
	}
	defer session.Close()

	select {
	case readErr := <-readResult:
		if readErr == nil {
			t.Fatalf("expected no outgoing spectator ack_snapshot message and read timeout")
		}
		netErr, ok := readErr.(net.Error)
		if !ok || !netErr.Timeout() {
			t.Fatalf("expected read timeout waiting for spectator outgoing message, got %v", readErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for spectator outgoing message check")
	}

	if session.LastSnapshotAckTick() != 0 {
		t.Fatalf("expected spectator session to keep snapshot ack tick at 0, got %d", session.LastSnapshotAckTick())
	}
}
