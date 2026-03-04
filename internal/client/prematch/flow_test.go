package prematch

import (
	"errors"
	"testing"

	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestMainMenuSelectionTransitionsToConnectingForQuickPlay(t *testing.T) {
	flow := NewFlow()
	if flow.Stage() != StageMainMenu {
		t.Fatalf("expected initial stage main_menu, got %s", flow.Stage())
	}

	intent, shouldConnect := flow.ActivateMenuSelection()
	if !shouldConnect {
		t.Fatalf("expected quick play activation to request connect")
	}
	if intent.PreferredMatchID != "" {
		t.Fatalf("expected quick play to have empty preferred match id, got %s", intent.PreferredMatchID)
	}
	if intent.Spectator {
		t.Fatalf("expected quick play to create player join intent")
	}
	if flow.Stage() != StageConnecting {
		t.Fatalf("expected stage to transition to connecting, got %s", flow.Stage())
	}
}

func TestMainMenuSelectionTransitionsToConnectingForSpectator(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(2) // Spectate Match

	intent, shouldConnect := flow.ActivateMenuSelection()
	if !shouldConnect {
		t.Fatalf("expected spectate selection to request connect")
	}
	if !intent.Spectator {
		t.Fatalf("expected spectate selection to set spectator intent")
	}
	if flow.Stage() != StageConnecting {
		t.Fatalf("expected stage to transition to connecting, got %s", flow.Stage())
	}
}

func TestMainMenuSelectionTransitionsToTutorialCodex(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(3) // Tutorial / Rules Codex

	intent, shouldConnect := flow.ActivateMenuSelection()
	if shouldConnect {
		t.Fatalf("expected tutorial codex selection to not start connection")
	}
	if intent.Spectator || intent.PreferredMatchID != "" {
		t.Fatalf("expected tutorial codex selection to keep empty connect intent, got %+v", intent)
	}
	if flow.Stage() != StageTutorial {
		t.Fatalf("expected stage tutorial, got %s", flow.Stage())
	}
}

func TestBrowseLobbyFlowSelectsAndJoinsLobby(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(1) // Browse Lobbies
	_, shouldConnect := flow.ActivateMenuSelection()
	if shouldConnect {
		t.Fatalf("expected browsing lobbies to not immediately connect")
	}
	if flow.Stage() != StageLobbyList {
		t.Fatalf("expected stage lobby_list, got %s", flow.Stage())
	}

	flow.SetLobbies([]protocol.LobbySummary{
		{MatchID: "m-older", Joinable: true, PlayerCount: 2, ReadyToStart: true},
		{MatchID: "m-fuller", Joinable: true, PlayerCount: 4, ReadyToStart: true},
		{MatchID: "m-closed", Joinable: false, PlayerCount: 6, ReadyToStart: true},
	})

	selected, ok := flow.SelectedLobby()
	if !ok {
		t.Fatalf("expected selected lobby after loading list")
	}
	if selected.MatchID != "m-fuller" {
		t.Fatalf("expected joinable fullest lobby to be selected first, got %s", selected.MatchID)
	}

	flow.MoveLobbySelection(1)
	selected, _ = flow.SelectedLobby()
	if selected.MatchID != "m-older" {
		t.Fatalf("expected selection rotation to move to m-older, got %s", selected.MatchID)
	}

	intent, err := flow.BeginJoinSelectedLobby()
	if err != nil {
		t.Fatalf("join selected lobby returned error: %v", err)
	}
	if intent.PreferredMatchID != "m-older" {
		t.Fatalf("expected connect intent to selected match, got %s", intent.PreferredMatchID)
	}
	if flow.Stage() != StageConnecting {
		t.Fatalf("expected stage connecting after begin join, got %s", flow.Stage())
	}
}

func TestJoinSelectedLobbyReturnsErrorWhenNoLobbiesAvailable(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(1)
	_, _ = flow.ActivateMenuSelection()

	if _, err := flow.BeginJoinSelectedLobby(); err == nil {
		t.Fatalf("expected join selected lobby error for empty list")
	}
	if flow.Stage() != StageLobbyList {
		t.Fatalf("expected stage to remain lobby_list after error, got %s", flow.Stage())
	}
}

func TestBeginSpectateSelectedLobbyBuildsSpectatorIntent(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(1)
	_, _ = flow.ActivateMenuSelection()
	flow.SetLobbies([]protocol.LobbySummary{
		{MatchID: "m-123", Joinable: true, PlayerCount: 3, ReadyToStart: true},
	})

	intent, err := flow.BeginSpectateSelectedLobby()
	if err != nil {
		t.Fatalf("expected spectator lobby join intent, got error: %v", err)
	}
	if !intent.Spectator {
		t.Fatalf("expected spectator intent for spectator lobby join")
	}
	if intent.PreferredMatchID != "m-123" {
		t.Fatalf("expected spectator preferred match id m-123, got %s", intent.PreferredMatchID)
	}
	if flow.Stage() != StageConnecting {
		t.Fatalf("expected stage connecting after spectator lobby intent, got %s", flow.Stage())
	}
}

func TestOnJoinedAndLobbySnapshotTransitionsToInMatchWhenRunning(t *testing.T) {
	flow := NewFlow()
	flow.OnJoined(LobbyStatus{
		MatchID:     "m-lobby",
		Status:      model.MatchStatusLobby,
		PlayerCount: 1,
		MinPlayers:  2,
		MaxPlayers:  6,
	})
	if flow.Stage() != StageLobbyWait {
		t.Fatalf("expected lobby_wait stage after joining lobby, got %s", flow.Stage())
	}
	if flow.LobbyStatus().ReadyToStart() {
		t.Fatalf("expected ready_to_start false with only one player")
	}

	flow.OnLobbySnapshot(model.MatchStatusLobby, 2)
	if !flow.LobbyStatus().ReadyToStart() {
		t.Fatalf("expected ready_to_start true after second player arrives")
	}
	if flow.Stage() != StageLobbyWait {
		t.Fatalf("expected to remain in lobby_wait until running, got %s", flow.Stage())
	}

	flow.OnLobbySnapshot(model.MatchStatusRunning, 2)
	if flow.Stage() != StageInMatch {
		t.Fatalf("expected stage transition to in_match when status running, got %s", flow.Stage())
	}
}

func TestConnectErrorMovesToErrorStageAndCanReturnToMenu(t *testing.T) {
	flow := NewFlow()
	flow.OnConnectError(errors.New("network timeout"))
	if flow.Stage() != StageErrorNotice {
		t.Fatalf("expected error_notice stage, got %s", flow.Stage())
	}
	if flow.LastError() != "network timeout" {
		t.Fatalf("expected propagated error message, got %q", flow.LastError())
	}

	flow.BackToMainMenu()
	if flow.Stage() != StageMainMenu {
		t.Fatalf("expected back to main menu, got %s", flow.Stage())
	}
}

func TestTutorialPageNavigationWraps(t *testing.T) {
	flow := NewFlow()
	flow.MoveMenuSelection(3)
	_, _ = flow.ActivateMenuSelection()
	if flow.Stage() != StageTutorial {
		t.Fatalf("expected stage tutorial before page navigation, got %s", flow.Stage())
	}

	flow.MoveTutorialPage(1, 5)
	if flow.TutorialPage() != 1 {
		t.Fatalf("expected tutorial page 1 after forward navigation, got %d", flow.TutorialPage())
	}

	flow.MoveTutorialPage(-2, 5)
	if flow.TutorialPage() != 4 {
		t.Fatalf("expected wrapped tutorial page 4 after reverse navigation, got %d", flow.TutorialPage())
	}
}
