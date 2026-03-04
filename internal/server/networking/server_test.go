package networking

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestWebSocketConnectionLifecycleConnectAndDisconnect(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "t",
	})
	defer h.Close(t)

	client := h.Dial(t)

	waitFor(t, "active connection", func() bool {
		return h.server.ConnectionCount() == 1
	})
	snapshots := h.server.ConnectionSnapshots()
	if len(snapshots) != 1 {
		t.Fatalf("expected one active connection snapshot, got %d", len(snapshots))
	}
	connectionID := snapshots[0].ConnectionID

	_ = client.Close()

	waitFor(t, "connection close", func() bool {
		return h.server.ConnectionCount() == 0
	})

	events := h.server.ConnectionEvents()
	var sawConnected, sawDisconnected bool
	for _, event := range events {
		if event.ConnectionID != connectionID {
			continue
		}
		if event.Type == ConnectionEventConnected {
			sawConnected = true
		}
		if event.Type == ConnectionEventDisconnected {
			sawDisconnected = true
		}
	}

	if !sawConnected || !sawDisconnected {
		t.Fatalf("expected connect/disconnect events for %s, got events=%#v", connectionID, events)
	}
}

func TestJoinGameReturnsJoinAcceptedAndBindsConnection(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "join",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName: "Alice",
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	joinEnvelope.PlayerID = "alice-player"
	writeEnvelope(t, client, joinEnvelope)

	response := readEnvelope(t, client, 2*time.Second)
	if response.Type != protocol.MsgJoinAccepted {
		t.Fatalf("expected join_accepted, got %s", response.Type)
	}

	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](response)
	if err != nil {
		t.Fatalf("decode join accepted payload: %v", err)
	}
	if accepted.PlayerID != "alice-player" {
		t.Fatalf("unexpected player id in join_accepted: got=%s want=%s", accepted.PlayerID, "alice-player")
	}
	if accepted.SessionKind != protocol.SessionKindPlayer {
		t.Fatalf("expected player session kind on normal join, got %s", accepted.SessionKind)
	}
	if !accepted.CanSendCommands {
		t.Fatalf("expected player join to allow gameplay commands")
	}
	if accepted.MatchID == "" {
		t.Fatalf("expected match id in join_accepted")
	}

	waitFor(t, "bound connection snapshot", func() bool {
		snapshots := h.server.ConnectionSnapshots()
		return len(snapshots) == 1 && snapshots[0].MatchID == accepted.MatchID && snapshots[0].PlayerID == "alice-player"
	})

	matchSnapshot, exists := h.manager.MatchSnapshot(accepted.MatchID)
	if !exists {
		t.Fatalf("expected match snapshot after successful join")
	}
	if len(matchSnapshot.Players) != 1 {
		t.Fatalf("expected one player in match snapshot, got %d", len(matchSnapshot.Players))
	}
}

func TestJoinGameSendsFullSnapshot(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "snap-join",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName: "SnapshotTester",
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	joinEnvelope.PlayerID = "snapshot-player"
	writeEnvelope(t, client, joinEnvelope)

	acceptedEnvelope := readUntilType(t, client, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode join accepted payload: %v", err)
	}

	snapshotEnvelope := readUntilType(t, client, protocol.MsgSnapshot, 2*time.Second)
	snapshotPayload, err := protocol.DecodePayload[protocol.SnapshotMessage](snapshotEnvelope)
	if err != nil {
		t.Fatalf("decode snapshot payload: %v", err)
	}

	if snapshotPayload.Snapshot.Kind != model.SnapshotKindFull {
		t.Fatalf("expected full snapshot on join, got kind=%s", snapshotPayload.Snapshot.Kind)
	}
	if snapshotPayload.Snapshot.TickID != 0 {
		t.Fatalf("expected full snapshot tick 0 on join, got %d", snapshotPayload.Snapshot.TickID)
	}
	if snapshotPayload.Snapshot.State == nil {
		t.Fatalf("expected full snapshot state on join")
	}
	if snapshotPayload.Snapshot.State.MatchID != accepted.MatchID {
		t.Fatalf(
			"expected full snapshot match id %s, got %s",
			accepted.MatchID,
			snapshotPayload.Snapshot.State.MatchID,
		)
	}
	if snapshotPayload.Snapshot.State.Status != model.MatchStatusLobby {
		t.Fatalf(
			"expected lobby status on join full snapshot, got %s",
			snapshotPayload.Snapshot.State.Status,
		)
	}
	if len(snapshotPayload.Snapshot.State.Players) != 1 {
		t.Fatalf(
			"expected one player in join full snapshot, got %d",
			len(snapshotPayload.Snapshot.State.Players),
		)
	}
	if snapshotPayload.Snapshot.State.Players[0].ID != "snapshot-player" {
		t.Fatalf(
			"expected joined player in full snapshot, got %s",
			snapshotPayload.Snapshot.State.Players[0].ID,
		)
	}
}

func TestListLobbiesReturnsSortedJoinableLobbySummaries(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "list",
	})
	defer h.Close(t)

	matchA := h.manager.CreateMatch()
	matchB := h.manager.CreateMatch()

	if _, err := h.manager.JoinMatch(matchA.MatchID, "a1", "A1"); err != nil {
		t.Fatalf("join A1 failed: %v", err)
	}
	if _, err := h.manager.JoinMatch(matchA.MatchID, "a2", "A2"); err != nil {
		t.Fatalf("join A2 failed: %v", err)
	}
	if _, err := h.manager.JoinMatch(matchB.MatchID, "b1", "B1"); err != nil {
		t.Fatalf("join B1 failed: %v", err)
	}

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgListLobbies, protocol.ListLobbiesRequest{})
	if err != nil {
		t.Fatalf("build list_lobbies envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgLobbyList, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.LobbyListMessage](response)
	if err != nil {
		t.Fatalf("decode lobby_list payload: %v", err)
	}

	if len(payload.Lobbies) != 2 {
		t.Fatalf("expected 2 lobby entries, got %d", len(payload.Lobbies))
	}
	if payload.Lobbies[0].MatchID != matchA.MatchID {
		t.Fatalf("expected highest population lobby first, got %s", payload.Lobbies[0].MatchID)
	}
	if payload.Lobbies[0].PlayerCount != 2 {
		t.Fatalf("expected lobby player count 2, got %d", payload.Lobbies[0].PlayerCount)
	}
	if payload.Lobbies[0].OpenSlots != 4 {
		t.Fatalf("expected lobby open slots 4, got %d", payload.Lobbies[0].OpenSlots)
	}
	if !payload.Lobbies[0].Joinable {
		t.Fatalf("expected lobby to be joinable")
	}
}

func TestJoinGameWithoutPreferredReusesOpenLobby(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "reuse",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()

	matchA := joinClient(t, clientA, "p1", "")
	matchB := joinClient(t, clientB, "p2", "")

	if matchA != matchB {
		t.Fatalf("expected empty preferred match join to reuse open lobby, got %s and %s", matchA, matchB)
	}

	snapshot, exists := h.manager.MatchSnapshot(matchA)
	if !exists {
		t.Fatalf("expected reused lobby snapshot")
	}
	if len(snapshot.Players) != 2 {
		t.Fatalf("expected two players in reused lobby, got %d", len(snapshot.Players))
	}
}

func TestJoinGameWithoutPreferredCreatesLobbyWhenOnlyRunningMatchesExist(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "fallback",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()
	clientC := h.Dial(t)
	defer clientC.Close()

	runningMatchID := joinClient(t, clientA, "p1", "")
	_ = joinClient(t, clientB, "p2", runningMatchID)
	_ = readUntilType(t, clientA, protocol.MsgGameStart, 2*time.Second)
	_ = readUntilType(t, clientB, protocol.MsgGameStart, 2*time.Second)

	waitFor(t, "running match status", func() bool {
		snapshot, exists := h.manager.MatchSnapshot(runningMatchID)
		return exists && snapshot.Status == model.MatchStatusRunning
	})

	createdMatchID := joinClient(t, clientC, "p3", "")
	if createdMatchID == runningMatchID {
		t.Fatalf("expected new lobby creation when existing match is running")
	}
}

func TestSpectatorJoinRequiresPreferredMatchID(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		Spectator: true,
	})
	if err != nil {
		t.Fatalf("build spectator join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode spectator join error payload: %v", err)
	}
	if payload.Code != protocol.ErrInvalidPayload {
		t.Fatalf("expected invalid payload code for spectator join without match id, got %s", payload.Code)
	}
}

func TestSpectatorJoinRejectsUnknownMatch(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		Spectator:        true,
		PreferredMatchID: "missing-match",
	})
	if err != nil {
		t.Fatalf("build spectator join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode spectator join error payload: %v", err)
	}
	if payload.Code != protocol.ErrMatchNotFound {
		t.Fatalf("expected match_not_found code for unknown spectator target, got %s", payload.Code)
	}
}

func TestSpectatorJoinRejectsNonRunningMatch(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec",
	})
	defer h.Close(t)

	match := h.manager.CreateMatch()

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		Spectator:        true,
		PreferredMatchID: match.MatchID,
	})
	if err != nil {
		t.Fatalf("build spectator join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode spectator join non-running error payload: %v", err)
	}
	if payload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized code for non-running spectator target, got %s", payload.Code)
	}
}

func TestSpectatorCanJoinRunningMatchWithReadOnlyVisibility(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec",
	})
	defer h.Close(t)

	playerA := h.Dial(t)
	defer playerA.Close()
	playerB := h.Dial(t)
	defer playerB.Close()
	spectator := h.Dial(t)
	defer spectator.Close()

	matchID := joinClient(t, playerA, "p1", "")
	_ = joinClient(t, playerB, "p2", matchID)
	_ = readUntilType(t, playerA, protocol.MsgGameStart, 2*time.Second)
	_ = readUntilType(t, playerB, protocol.MsgGameStart, 2*time.Second)

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		Spectator:        true,
		PreferredMatchID: matchID,
	})
	if err != nil {
		t.Fatalf("build spectator join envelope: %v", err)
	}
	writeEnvelope(t, spectator, joinEnvelope)

	acceptedEnvelope := readUntilType(t, spectator, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode spectator join accepted: %v", err)
	}
	if accepted.MatchID != matchID {
		t.Fatalf("expected spectator to join match %s, got %s", matchID, accepted.MatchID)
	}
	if accepted.SessionKind != protocol.SessionKindSpectator {
		t.Fatalf("expected spectator session kind, got %s", accepted.SessionKind)
	}
	if accepted.CanSendCommands {
		t.Fatalf("expected spectator can_send_commands to be false")
	}
	if accepted.PlayerID != "" {
		t.Fatalf("expected spectator player_id to be empty, got %s", accepted.PlayerID)
	}
	if accepted.SpectatorFollowPlayerID == "" {
		t.Fatalf("expected spectator follow player id in join accepted payload")
	}
	if accepted.SpectatorFollowSlot == 0 || accepted.SpectatorSlotCount < 2 {
		t.Fatalf(
			"expected spectator follow slot metadata in join accepted payload, got slot=%d count=%d",
			accepted.SpectatorFollowSlot,
			accepted.SpectatorSlotCount,
		)
	}

	gameStartEnvelope := readUntilType(t, spectator, protocol.MsgGameStart, 2*time.Second)
	gameStartPayload, err := protocol.DecodePayload[protocol.GameStartMessage](gameStartEnvelope)
	if err != nil {
		t.Fatalf("decode spectator game_start payload: %v", err)
	}
	if gameStartPayload.LocalPlayerID != "" {
		t.Fatalf("expected spectator local player id to be empty, got %s", gameStartPayload.LocalPlayerID)
	}
	if gameStartPayload.InitialSnapshot.State == nil {
		t.Fatalf("expected spectator initial snapshot state")
	}
	if len(gameStartPayload.InitialSnapshot.State.Players) < 2 {
		t.Fatalf("expected spectator to receive running match players, got %d", len(gameStartPayload.InitialSnapshot.State.Players))
	}

	var sawWarden bool
	var sawHiddenNonWarden bool
	for _, player := range gameStartPayload.InitialSnapshot.State.Players {
		if player.Role == model.RoleWarden {
			sawWarden = true
			if player.Faction != model.FactionAuthority {
				t.Fatalf("expected spectator to see public warden faction authority, got %+v", player)
			}
			if player.Alignment != "" {
				t.Fatalf("expected spectator to have warden alignment hidden, got %+v", player)
			}
			continue
		}

		if player.Role != "" || player.Faction != "" || player.Alignment != "" {
			t.Fatalf("expected non-warden roles hidden from spectator, got %+v", player)
		}
		sawHiddenNonWarden = true
	}
	if !sawWarden || !sawHiddenNonWarden {
		t.Fatalf("expected spectator visibility mix (public warden + hidden others)")
	}

	playerCommandEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdMoveIntent,
			Payload: mustRawJSON(t, model.MovementInputPayload{
				MoveX: 1,
				MoveY: 0,
			}),
		},
	})
	if err != nil {
		t.Fatalf("build player movement command envelope: %v", err)
	}
	writeEnvelope(t, playerA, playerCommandEnvelope)

	deadline := time.Now().Add(3 * time.Second)
	var sawDelta bool
	for time.Now().Before(deadline) {
		envelope := readEnvelope(t, spectator, time.Until(deadline))
		if envelope.Type != protocol.MsgSnapshot {
			continue
		}
		payload, decodeErr := protocol.DecodePayload[protocol.SnapshotMessage](envelope)
		if decodeErr != nil {
			t.Fatalf("decode spectator streamed snapshot: %v", decodeErr)
		}
		if payload.Snapshot.Kind == model.SnapshotKindDelta {
			sawDelta = true
			break
		}
	}
	if !sawDelta {
		t.Fatalf("expected spectator to receive streamed delta snapshot")
	}

	inputEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build spectator gameplay command envelope: %v", err)
	}
	writeEnvelope(t, spectator, inputEnvelope)
	errorEnvelope := readUntilType(t, spectator, protocol.MsgError, 2*time.Second)
	errorPayload, err := protocol.DecodePayload[protocol.ErrorMessage](errorEnvelope)
	if err != nil {
		t.Fatalf("decode spectator command error payload: %v", err)
	}
	if errorPayload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized for spectator gameplay command, got %s", errorPayload.Code)
	}
}

func TestSpectatorJoinResolvesRequestedFollowTarget(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec",
	})
	defer h.Close(t)

	playerA := h.Dial(t)
	defer playerA.Close()
	playerB := h.Dial(t)
	defer playerB.Close()
	spectator := h.Dial(t)
	defer spectator.Close()

	matchID := joinClient(t, playerA, "p1", "")
	_ = joinClient(t, playerB, "p2", matchID)
	_ = readUntilType(t, playerA, protocol.MsgGameStart, 2*time.Second)
	_ = readUntilType(t, playerB, protocol.MsgGameStart, 2*time.Second)

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		Spectator:               true,
		PreferredMatchID:        matchID,
		SpectatorFollowPlayerID: "p2",
		SpectatorFollowSlot:     1,
	})
	if err != nil {
		t.Fatalf("build spectator join envelope: %v", err)
	}
	writeEnvelope(t, spectator, joinEnvelope)

	acceptedEnvelope := readUntilType(t, spectator, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode spectator join accepted: %v", err)
	}
	if accepted.SpectatorFollowPlayerID != "p2" {
		t.Fatalf("expected requested spectator follow player p2, got %s", accepted.SpectatorFollowPlayerID)
	}
	if accepted.SpectatorFollowSlot != 2 {
		t.Fatalf("expected spectator follow slot 2 for p2, got %d", accepted.SpectatorFollowSlot)
	}
	if accepted.SpectatorSlotCount < 2 {
		t.Fatalf("expected spectator slot count to include both running players, got %d", accepted.SpectatorSlotCount)
	}
}

func TestJoinGameRejectsSpectatorFollowOptionsForPlayerJoin(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "spec-validate",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:              "player",
		Spectator:               false,
		SpectatorFollowSlot:     1,
		SpectatorFollowPlayerID: "p2",
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	writeEnvelope(t, client, envelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode join error payload: %v", err)
	}
	if payload.Code != protocol.ErrInvalidPayload {
		t.Fatalf("expected invalid payload code for spectator follow options on player join, got %s", payload.Code)
	}
}

func TestRequestReplayRequiresJoinedMatchContext(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "replay-auth",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	requestEnvelope, err := protocol.NewEnvelope(protocol.MsgRequestReplay, protocol.ReplayLogRequest{})
	if err != nil {
		t.Fatalf("build request_replay envelope: %v", err)
	}
	writeEnvelope(t, client, requestEnvelope)

	response := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	errPayload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode request_replay error payload: %v", err)
	}
	if errPayload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized response for replay without join, got %s", errPayload.Code)
	}
}

func TestRequestReplayReturnsAcceptedInputEntries(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "replay",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	matchID := joinClient(t, client, "replay-p1", "")
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

	waitFor(t, "replay log entry", func() bool {
		replay, replayErr := h.manager.ReplayLog(matchID)
		return replayErr == nil && len(replay.Entries) >= 1
	})

	requestEnvelope, err := protocol.NewEnvelope(protocol.MsgRequestReplay, protocol.ReplayLogRequest{})
	if err != nil {
		t.Fatalf("build request_replay envelope: %v", err)
	}
	writeEnvelope(t, client, requestEnvelope)

	response := readUntilType(t, client, protocol.MsgReplayLog, 2*time.Second)
	replayPayload, err := protocol.DecodePayload[protocol.ReplayLogMessage](response)
	if err != nil {
		t.Fatalf("decode replay_log payload: %v", err)
	}

	if replayPayload.MatchID != matchID {
		t.Fatalf("expected replay match id %s, got %s", matchID, replayPayload.MatchID)
	}
	if replayPayload.TickRateHz != 30 {
		t.Fatalf("expected replay tick rate 30, got %d", replayPayload.TickRateHz)
	}
	if len(replayPayload.Entries) == 0 {
		t.Fatalf("expected at least one replay entry")
	}

	first := replayPayload.Entries[0]
	if first.Command.PlayerID != "replay-p1" {
		t.Fatalf("expected replay entry player replay-p1, got %s", first.Command.PlayerID)
	}
	if first.Command.ClientSeq != 1 {
		t.Fatalf("expected replay entry client_seq 1, got %d", first.Command.ClientSeq)
	}
	if first.Command.Type != model.CmdReload {
		t.Fatalf("expected replay entry type reload, got %s", first.Command.Type)
	}
	if first.IngressSeq == 0 {
		t.Fatalf("expected replay entry ingress seq > 0")
	}
	if first.AcceptedTick == 0 {
		t.Fatalf("expected replay entry accepted tick > 0")
	}
}

func TestPingReturnsPong(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "ping",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	pingEnvelope, err := protocol.NewEnvelope(protocol.MsgPing, protocol.PingMessage{
		ClientSendUnixMS: 123456789,
	})
	if err != nil {
		t.Fatalf("build ping envelope: %v", err)
	}
	writeEnvelope(t, client, pingEnvelope)

	response := readEnvelope(t, client, 2*time.Second)
	if response.Type != protocol.MsgPong {
		t.Fatalf("expected pong response, got %s", response.Type)
	}

	pong, err := protocol.DecodePayload[protocol.PongMessage](response)
	if err != nil {
		t.Fatalf("decode pong payload: %v", err)
	}
	if pong.ClientSendUnixMS != 123456789 {
		t.Fatalf("unexpected echoed client timestamp: got=%d", pong.ClientSendUnixMS)
	}
	if pong.ServerSendUnixMS <= 0 {
		t.Fatalf("expected positive server timestamp, got=%d", pong.ServerSendUnixMS)
	}
}

func TestInvalidEnvelopeReturnsProtocolError(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "err",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	if err := client.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if err := client.WriteMessage(websocket.TextMessage, []byte("{not-json")); err != nil {
		t.Fatalf("write invalid json envelope: %v", err)
	}

	response := readEnvelope(t, client, 2*time.Second)
	if response.Type != protocol.MsgError {
		t.Fatalf("expected error response, got %s", response.Type)
	}

	payload, err := protocol.DecodePayload[protocol.ErrorMessage](response)
	if err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload.Code != protocol.ErrInvalidPayload {
		t.Fatalf("expected invalid_payload code, got %s", payload.Code)
	}
}

func TestBroadcastToMatchDeliversServerEventsToAllParticipants(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    10,
		MaxPlayers:    12,
		TickRateHz:    30,
		MatchIDPrefix: "broadcast",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()

	matchID := joinClient(t, clientA, "p1", "")
	_ = joinClient(t, clientB, "p2", matchID)

	phasePayload := protocol.PhaseChangeMessage{
		TickID:     15,
		CycleCount: 1,
		Phase: model.PhaseState{
			Current:     model.PhaseNight,
			StartedTick: 15,
			EndsTick:    300,
		},
	}
	if err := h.server.BroadcastToMatch(matchID, protocol.MsgPhaseChange, phasePayload); err != nil {
		t.Fatalf("broadcast phase change: %v", err)
	}

	aEvent := readUntilType(t, clientA, protocol.MsgPhaseChange, 2*time.Second)
	bEvent := readUntilType(t, clientB, protocol.MsgPhaseChange, 2*time.Second)
	if aEvent.Type != protocol.MsgPhaseChange {
		t.Fatalf("expected phase_change for client A, got %s", aEvent.Type)
	}
	if bEvent.Type != protocol.MsgPhaseChange {
		t.Fatalf("expected phase_change for client B, got %s", bEvent.Type)
	}
}

func TestRunningMatchStreamsDeltaSnapshots(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "snap-delta",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	_ = joinClient(t, client, "runner", "")
	gameStartEnvelope := readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)
	gameStartPayload, err := protocol.DecodePayload[protocol.GameStartMessage](gameStartEnvelope)
	if err != nil {
		t.Fatalf("decode game_start payload: %v", err)
	}

	commandEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdMoveIntent,
			Payload: mustRawJSON(t, model.MovementInputPayload{
				MoveX: 1,
				MoveY: 0,
			}),
		},
	})
	if err != nil {
		t.Fatalf("build player input envelope: %v", err)
	}
	writeEnvelope(t, client, commandEnvelope)

	deadline := time.Now().Add(3 * time.Second)
	sawDelta := false
	for time.Now().Before(deadline) {
		envelope := readEnvelope(t, client, time.Until(deadline))
		if envelope.Type != protocol.MsgSnapshot {
			continue
		}

		payload, err := protocol.DecodePayload[protocol.SnapshotMessage](envelope)
		if err != nil {
			t.Fatalf("decode streamed snapshot payload: %v", err)
		}
		snapshot := payload.Snapshot
		if snapshot.Kind != model.SnapshotKindDelta {
			continue
		}

		sawDelta = true
		if snapshot.TickID <= gameStartPayload.StartTickID {
			t.Fatalf("expected streamed delta tick > start tick, got %d", snapshot.TickID)
		}
		if !hasPlayerAckAtLeast(snapshot.PlayerAcks, "runner", 1) {
			continue
		}
		if snapshot.Delta == nil {
			t.Fatalf("expected delta payload for streamed delta snapshot")
		}
		if !containsChangedPlayer(snapshot.Delta.ChangedPlayers, "runner") {
			continue
		}
		return
	}

	if !sawDelta {
		t.Fatalf("expected at least one streamed delta snapshot")
	}
	t.Fatalf("timed out waiting for delta snapshot with acked player command")
}

func TestGameplayCommandRequiresJoinAndIsAcceptedAfterJoin(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "cmd",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	envelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdMoveIntent,
			Payload: mustRawJSON(t, model.MovementInputPayload{
				MoveX: 1,
				MoveY: 0,
			}),
		},
	})
	if err != nil {
		t.Fatalf("build player_input envelope: %v", err)
	}

	writeEnvelope(t, client, envelope)
	unauthorized := readEnvelope(t, client, 2*time.Second)
	if unauthorized.Type != protocol.MsgError {
		t.Fatalf("expected error response before join, got %s", unauthorized.Type)
	}
	errPayload, err := protocol.DecodePayload[protocol.ErrorMessage](unauthorized)
	if err != nil {
		t.Fatalf("decode unauthorized payload: %v", err)
	}
	if errPayload.Code != protocol.ErrUnauthorized {
		t.Fatalf("expected unauthorized error code, got %s", errPayload.Code)
	}

	matchID := joinClient(t, client, "player-a", "")

	// Match auto-start emits game_start; consume it so subsequent reads are deterministic.
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	writeEnvelope(t, client, envelope)
	waitFor(t, "queued player input after join", func() bool {
		counts, err := h.manager.PendingInputCounts(matchID)
		if err != nil {
			return false
		}
		total := 0
		for _, count := range counts {
			total += count
		}
		return total >= 1
	})
}

func TestGameplayInputDuplicateAndRateLimitErrors(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    1,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "cmd-err",
	})
	defer h.Close(t)

	client := h.Dial(t)
	defer client.Close()

	_ = joinClient(t, client, "player-z", "")
	_ = readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)

	firstEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build first command envelope: %v", err)
	}
	writeEnvelope(t, client, firstEnvelope)

	duplicateEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 1,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build duplicate command envelope: %v", err)
	}
	writeEnvelope(t, client, duplicateEnvelope)
	duplicateError := readUntilType(t, client, protocol.MsgError, 2*time.Second)
	duplicatePayload, err := protocol.DecodePayload[protocol.ErrorMessage](duplicateError)
	if err != nil {
		t.Fatalf("decode duplicate error payload: %v", err)
	}
	if duplicatePayload.Code != protocol.ErrOutOfDateCommand {
		t.Fatalf("expected duplicate command error code %s, got %s", protocol.ErrOutOfDateCommand, duplicatePayload.Code)
	}

	for seq := uint64(2); seq <= 10; seq++ {
		envelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
			Command: model.InputCommand{
				ClientSeq: seq,
				Type:      model.CmdReload,
			},
		})
		if err != nil {
			t.Fatalf("build envelope for seq %d: %v", seq, err)
		}
		writeEnvelope(t, client, envelope)
		if seq == 10 {
			rateLimitError := readUntilType(t, client, protocol.MsgError, 2*time.Second)
			rateLimitPayload, err := protocol.DecodePayload[protocol.ErrorMessage](rateLimitError)
			if err != nil {
				t.Fatalf("decode rate limit payload: %v", err)
			}
			if rateLimitPayload.Code != protocol.ErrRateLimited {
				t.Fatalf("expected rate limited code %s, got %s", protocol.ErrRateLimited, rateLimitPayload.Code)
			}
		}
	}
}

func TestAutoStartSendsGameStartWhenMinPlayersReached(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "auto",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()

	matchID := joinClient(t, clientA, "p1", "")
	_ = joinClient(t, clientB, "p2", matchID)

	eventA := readUntilType(t, clientA, protocol.MsgGameStart, 2*time.Second)
	eventB := readUntilType(t, clientB, protocol.MsgGameStart, 2*time.Second)

	startA, err := protocol.DecodePayload[protocol.GameStartMessage](eventA)
	if err != nil {
		t.Fatalf("decode game_start A payload: %v", err)
	}
	startB, err := protocol.DecodePayload[protocol.GameStartMessage](eventB)
	if err != nil {
		t.Fatalf("decode game_start B payload: %v", err)
	}

	if startA.MatchID != matchID || startB.MatchID != matchID {
		t.Fatalf("expected game_start match id %s, got A=%s B=%s", matchID, startA.MatchID, startB.MatchID)
	}
	if startA.LocalPlayerID != "p1" {
		t.Fatalf("expected local player id p1 for client A, got %s", startA.LocalPlayerID)
	}
	if startB.LocalPlayerID != "p2" {
		t.Fatalf("expected local player id p2 for client B, got %s", startB.LocalPlayerID)
	}
}

func TestGameStartSnapshotHidesNonPublicRolesFromOtherPlayers(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    6,
		MaxPlayers:    12,
		TickRateHz:    30,
		DaySeconds:    300,
		NightSeconds:  120,
		MaxCycles:     6,
		MatchIDPrefix: "roles-vis",
	})
	defer h.Close(t)

	clients := make([]*websocket.Conn, 0, 6)
	for idx := 0; idx < 6; idx++ {
		client := h.Dial(t)
		clients = append(clients, client)
		defer client.Close()
	}

	matchID := joinClient(t, clients[0], "p1", "")
	for idx := 1; idx < len(clients); idx++ {
		playerID := model.PlayerID(fmt.Sprintf("p%d", idx+1))
		joinedMatchID := joinClient(t, clients[idx], playerID, matchID)
		if joinedMatchID != matchID {
			t.Fatalf("expected all players in same match, got %s and %s", joinedMatchID, matchID)
		}
	}

	for _, client := range clients {
		gameStartEnvelope := readUntilType(t, client, protocol.MsgGameStart, 2*time.Second)
		gameStart, err := protocol.DecodePayload[protocol.GameStartMessage](gameStartEnvelope)
		if err != nil {
			t.Fatalf("decode game_start payload: %v", err)
		}
		if gameStart.InitialSnapshot.State == nil {
			t.Fatalf("expected game_start initial snapshot state")
		}

		var sawSelf bool
		var sawHiddenOther bool
		for _, player := range gameStart.InitialSnapshot.State.Players {
			if player.ID == gameStart.LocalPlayerID {
				sawSelf = true
				if player.Role == "" || player.Faction == "" || player.Alignment == "" {
					t.Fatalf("expected local player role/faction/alignment visible, got %+v", player)
				}
				continue
			}

			if player.Role == model.RoleWarden {
				if player.Faction != model.FactionAuthority {
					t.Fatalf("expected public warden faction authority, got %+v", player)
				}
				if player.Alignment != "" {
					t.Fatalf("expected warden alignment hidden from others, got %+v", player)
				}
				continue
			}

			if player.Role != "" || player.Faction != "" || player.Alignment != "" {
				t.Fatalf("expected hidden role/faction/alignment for non-self non-warden, got %+v", player)
			}
			sawHiddenOther = true
		}

		if !sawSelf {
			t.Fatalf("expected to find local player %s in snapshot", gameStart.LocalPlayerID)
		}
		if !sawHiddenOther {
			t.Fatalf("expected at least one hidden other player in snapshot")
		}
	}
}

func TestReconnectJoinResumesRunningPlayerSession(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    30,
		MatchIDPrefix: "reconnect",
	})
	defer h.Close(t)

	clientA := h.Dial(t)
	defer clientA.Close()
	clientB := h.Dial(t)
	defer clientB.Close()

	matchID := joinClient(t, clientA, "p1", "")
	_ = joinClient(t, clientB, "p2", matchID)

	_ = readUntilType(t, clientA, protocol.MsgGameStart, 2*time.Second)
	_ = readUntilType(t, clientB, protocol.MsgGameStart, 2*time.Second)

	_ = clientA.Close()
	waitFor(t, "original player disconnect", func() bool {
		snapshots := h.server.ConnectionSnapshots()
		if len(snapshots) != 1 {
			return false
		}
		return snapshots[0].PlayerID == "p2"
	})

	reconnected := h.Dial(t)
	defer reconnected.Close()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:       "p1-rejoined",
		PreferredMatchID: matchID,
	})
	if err != nil {
		t.Fatalf("build reconnect join envelope: %v", err)
	}
	joinEnvelope.PlayerID = "p1"
	writeEnvelope(t, reconnected, joinEnvelope)

	acceptedEnvelope := readUntilType(t, reconnected, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode reconnect join accepted payload: %v", err)
	}
	if accepted.MatchID != matchID {
		t.Fatalf("expected reconnect to rebind match %s, got %s", matchID, accepted.MatchID)
	}
	if accepted.PlayerID != "p1" {
		t.Fatalf("expected reconnect to rebind player p1, got %s", accepted.PlayerID)
	}
	if accepted.SessionKind != protocol.SessionKindPlayer || !accepted.CanSendCommands {
		t.Fatalf("expected reconnect player session with command authority, got %+v", accepted)
	}

	snapshotEnvelope := readUntilType(t, reconnected, protocol.MsgSnapshot, 2*time.Second)
	snapshotPayload, err := protocol.DecodePayload[protocol.SnapshotMessage](snapshotEnvelope)
	if err != nil {
		t.Fatalf("decode reconnect snapshot payload: %v", err)
	}
	if snapshotPayload.Snapshot.State == nil {
		t.Fatalf("expected reconnect snapshot state")
	}
	if snapshotPayload.Snapshot.State.Status != model.MatchStatusRunning {
		t.Fatalf("expected reconnect snapshot to keep running status, got %s", snapshotPayload.Snapshot.State.Status)
	}
	var sawConnectedP1 bool
	for _, player := range snapshotPayload.Snapshot.State.Players {
		if player.ID == "p1" {
			sawConnectedP1 = player.Connected
			break
		}
	}
	if !sawConnectedP1 {
		t.Fatalf("expected reconnect snapshot to mark p1 as connected")
	}

	inputEnvelope, err := protocol.NewEnvelope(protocol.MsgPlayerInput, protocol.PlayerInputMessage{
		Command: model.InputCommand{
			ClientSeq: 17,
			Type:      model.CmdReload,
		},
	})
	if err != nil {
		t.Fatalf("build reconnect gameplay command envelope: %v", err)
	}
	writeEnvelope(t, reconnected, inputEnvelope)
	waitFor(t, "reconnected command accepted", func() bool {
		counts, pendingErr := h.manager.PendingInputCounts(matchID)
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

func TestRebindingSamePlayerClosesPreviousConnection(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "rebind",
	})
	defer h.Close(t)

	original := h.Dial(t)
	defer original.Close()

	matchID := joinClient(t, original, "p1", "")
	_ = readUntilType(t, original, protocol.MsgSnapshot, 2*time.Second)

	initialSnapshots := h.server.ConnectionSnapshots()
	if len(initialSnapshots) != 1 {
		t.Fatalf("expected one connection snapshot after original join, got %d", len(initialSnapshots))
	}
	originalConnectionID := initialSnapshots[0].ConnectionID

	rebind := h.Dial(t)
	defer rebind.Close()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:       "p1-rebind",
		PreferredMatchID: matchID,
	})
	if err != nil {
		t.Fatalf("build rebind join envelope: %v", err)
	}
	joinEnvelope.PlayerID = "p1"
	writeEnvelope(t, rebind, joinEnvelope)

	acceptedEnvelope := readUntilType(t, rebind, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](acceptedEnvelope)
	if err != nil {
		t.Fatalf("decode rebind join accepted payload: %v", err)
	}
	if accepted.PlayerID != "p1" || accepted.MatchID != matchID {
		t.Fatalf("expected rebind to keep same player/match identity, got %+v", accepted)
	}
	_ = readUntilType(t, rebind, protocol.MsgSnapshot, 2*time.Second)

	waitFor(t, "single active connection after rebind", func() bool {
		snapshots := h.server.ConnectionSnapshots()
		if len(snapshots) != 1 {
			return false
		}
		return snapshots[0].PlayerID == "p1" && snapshots[0].ConnectionID != originalConnectionID
	})

	waitFor(t, "original connection disconnected event", func() bool {
		events := h.server.ConnectionEvents()
		for _, event := range events {
			if event.ConnectionID == originalConnectionID && event.Type == ConnectionEventDisconnected {
				return true
			}
		}
		return false
	})
}

type harness struct {
	manager    *game.Manager
	server     *Server
	httpServer *httptest.Server
	wsURL      string
}

func newHarness(t *testing.T, gameConfig game.Config) *harness {
	t.Helper()

	manager := game.NewManager(gameConfig)
	netConfig := DefaultConfig()
	netConfig.PingInterval = 10 * time.Second
	netConfig.PongTimeout = 30 * time.Second
	netConfig.WriteTimeout = 3 * time.Second
	server := NewServer(manager, netConfig)

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

func (h *harness) Close(t *testing.T) {
	t.Helper()
	_ = h.server.Shutdown(context.Background())
	h.manager.Close()
	h.httpServer.Close()
}

func (h *harness) Dial(t *testing.T) *websocket.Conn {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(h.wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}

	return conn
}

func joinClient(t *testing.T, client *websocket.Conn, playerID model.PlayerID, preferredMatchID model.MatchID) model.MatchID {
	t.Helper()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:       string(playerID),
		PreferredMatchID: preferredMatchID,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	joinEnvelope.PlayerID = playerID

	writeEnvelope(t, client, joinEnvelope)
	response := readUntilType(t, client, protocol.MsgJoinAccepted, 2*time.Second)

	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](response)
	if err != nil {
		t.Fatalf("decode join_accepted payload: %v", err)
	}
	return accepted.MatchID
}

func writeEnvelope(t *testing.T, client *websocket.Conn, envelope protocol.Envelope) {
	t.Helper()

	if err := client.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set write deadline: %v", err)
	}
	if err := client.WriteJSON(envelope); err != nil {
		t.Fatalf("write envelope: %v", err)
	}
}

func readEnvelope(t *testing.T, client *websocket.Conn, timeout time.Duration) protocol.Envelope {
	t.Helper()

	if err := client.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}

	var envelope protocol.Envelope
	if err := client.ReadJSON(&envelope); err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	return envelope
}

func readUntilType(t *testing.T, client *websocket.Conn, messageType protocol.MessageType, timeout time.Duration) protocol.Envelope {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		envelope := readEnvelope(t, client, time.Until(deadline))
		if envelope.Type == messageType {
			return envelope
		}
	}

	t.Fatalf("did not receive message type %s before timeout", messageType)
	return protocol.Envelope{}
}

func waitFor(t *testing.T, description string, predicate func() bool) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for %s", description)
}

func mustRawJSON(t *testing.T, payload any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func hasPlayerAckAtLeast(acks []model.PlayerAck, playerID model.PlayerID, minimumSeq uint64) bool {
	for _, ack := range acks {
		if ack.PlayerID == playerID && ack.LastProcessedClientSeq >= minimumSeq {
			return true
		}
	}
	return false
}

func containsChangedPlayer(players []model.PlayerState, playerID model.PlayerID) bool {
	for _, player := range players {
		if player.ID == playerID {
			return true
		}
	}
	return false
}
