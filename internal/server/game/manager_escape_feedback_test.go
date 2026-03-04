package game

import (
	"strings"
	"testing"
	"time"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/gamecore/winconditions"
	"prison-break/internal/shared/model"
)

func TestEscapeRouteAttemptsEmitAuthoritativeFeedback(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "esc-fb",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "P2"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCourtyard)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", nil)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	feedback := playerLastEscapeAttemptForTest(manager, match.MatchID, "p1")
	if feedback.Route != model.EscapeRouteCourtyardDig {
		t.Fatalf("expected feedback route courtyard_dig, got %+v", feedback)
	}
	if feedback.Status != model.EscapeAttemptStatusFailed {
		t.Fatalf("expected failed feedback status for unmet route, got %+v", feedback)
	}
	if !strings.Contains(feedback.Reason, "Item: shovel x1") {
		t.Fatalf("expected feedback reason to include missing shovel requirement, got %+v", feedback)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemShovel, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	feedback = playerLastEscapeAttemptForTest(manager, match.MatchID, "p1")
	if feedback.Status != model.EscapeAttemptStatusSuccess {
		t.Fatalf("expected success feedback after valid escape attempt, got %+v", feedback)
	}
	if feedback.Reason != "Escape route successful." {
		t.Fatalf("expected success reason text, got %+v", feedback)
	}
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected player room escaped after successful route, got %s", room)
	}
}

func playerLastEscapeAttemptForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
) model.EscapeAttemptFeedback {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.LastEscapeAttempt
		}
	}
	return model.EscapeAttemptFeedback{}
}
