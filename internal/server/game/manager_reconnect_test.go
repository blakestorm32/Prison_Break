package game

import (
	"errors"
	"testing"
	"time"

	"prison-break/internal/shared/model"
)

func TestSetPlayerConnectedAndResumePlayerInRunningMatch(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "resume-running",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "PlayerOne"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "PlayerTwo"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start match failed: %v", err)
	}

	setPlayerPositionForTest(manager, match.MatchID, "p1", model.Vector2{X: 8, Y: 9})
	setPlayerHeartsForTest(manager, match.MatchID, "p1", 5)

	if err := manager.SetPlayerConnected(match.MatchID, "p1", false); err != nil {
		t.Fatalf("set player connected false: %v", err)
	}
	if connected := playerConnectedForTest(manager, match.MatchID, "p1"); connected {
		t.Fatalf("expected p1 to be disconnected")
	}

	if _, err := manager.ResumePlayer(match.MatchID, "p1", "PlayerOne-Rejoined"); err != nil {
		t.Fatalf("resume player failed: %v", err)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot after resume: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state after resume")
	}
	p1, found := playerFromStateForTest(*full.State, "p1")
	if !found {
		t.Fatalf("expected resumed player in snapshot")
	}
	if !p1.Connected {
		t.Fatalf("expected resumed player connected flag true, got %+v", p1)
	}
	if p1.Name != "PlayerOne-Rejoined" {
		t.Fatalf("expected resume to update player name, got %q", p1.Name)
	}
	if p1.HeartsHalf != 5 {
		t.Fatalf("expected resume to preserve hearts half=5, got %d", p1.HeartsHalf)
	}
	if p1.Position != (model.Vector2{X: 8, Y: 9}) {
		t.Fatalf("expected resume to preserve position, got %+v", p1.Position)
	}
}

func TestResumePlayerValidationAndMatchLookup(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "resume-validate",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	matchA := manager.CreateMatch()
	matchB := manager.CreateMatch()

	if _, err := manager.JoinMatch(matchA.MatchID, "p1", "PlayerOne"); err != nil {
		t.Fatalf("join p1 matchA failed: %v", err)
	}
	if _, err := manager.JoinMatch(matchB.MatchID, "p2", "PlayerTwo"); err != nil {
		t.Fatalf("join p2 matchB failed: %v", err)
	}

	if matchID, exists := manager.MatchIDForPlayer("p1"); !exists || matchID != matchA.MatchID {
		t.Fatalf("expected player-to-match mapping for p1 => %s, got %s exists=%t", matchA.MatchID, matchID, exists)
	}
	if _, exists := manager.MatchIDForPlayer("missing"); exists {
		t.Fatalf("expected missing player mapping to not exist")
	}

	if _, err := manager.ResumePlayer(matchB.MatchID, "p1", "WrongMatch"); !errors.Is(err, ErrPlayerAlreadyInMatch) {
		t.Fatalf("expected ErrPlayerAlreadyInMatch when resuming in wrong match, got %v", err)
	}
	if _, err := manager.ResumePlayer(matchA.MatchID, "missing", "Missing"); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound when resuming missing player, got %v", err)
	}

	if err := manager.SetPlayerConnected(matchA.MatchID, "missing", false); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound for missing player connected toggle, got %v", err)
	}
	if err := manager.SetPlayerConnected("missing-match", "p1", false); !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("expected ErrMatchNotFound for missing match connected toggle, got %v", err)
	}
}

func TestDisconnectedLobbyPlayerStatePersistsAcrossJoinSync(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "resume-sync",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "PlayerOne"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if err := manager.SetPlayerConnected(match.MatchID, "p1", false); err != nil {
		t.Fatalf("set p1 disconnected failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "PlayerTwo"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}

	p1, found := playerFromStateForTest(*full.State, "p1")
	if !found {
		t.Fatalf("expected p1 in full snapshot")
	}
	if p1.Connected {
		t.Fatalf("expected disconnected state to survive sync, got %+v", p1)
	}
}

func playerConnectedForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.Connected
		}
	}
	return false
}

func playerFromStateForTest(state model.GameState, playerID model.PlayerID) (model.PlayerState, bool) {
	for _, player := range state.Players {
		if player.ID == playerID {
			return player, true
		}
	}
	return model.PlayerState{}, false
}
