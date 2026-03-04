package game

import (
	"strings"
	"testing"
	"time"

	"prison-break/internal/gamecore/combat"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestCombatActionFeedbackForHitAndStun(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "fb-combat",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "a", "Shooter"); err != nil {
		t.Fatalf("join shooter failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "b", "Target"); err != nil {
		t.Fatalf("join target failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
	setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
	setPlayerBulletsForTest(manager, match.MatchID, "a", 3)
	setMapPowerForTest(manager, match.MatchID, false) // Avoid unjust-shot penalty feedback overriding hit feedback.

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
		Weapon:  model.ItemPistol,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	shooterFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "a")
	if shooterFeedback.Kind != model.ActionFeedbackKindCombat || shooterFeedback.Level != model.ActionFeedbackLevelSuccess {
		t.Fatalf("expected shooter combat success feedback, got %+v", shooterFeedback)
	}
	if !strings.Contains(shooterFeedback.Message, "Hit Target") {
		t.Fatalf("expected shooter hit feedback message, got %+v", shooterFeedback)
	}

	targetFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "b")
	if targetFeedback.Kind != model.ActionFeedbackKindCombat || targetFeedback.Level != model.ActionFeedbackLevelWarning {
		t.Fatalf("expected target combat warning feedback, got %+v", targetFeedback)
	}
	if !strings.Contains(targetFeedback.Message, "Hit by Shooter") {
		t.Fatalf("expected target hit-by feedback message, got %+v", targetFeedback)
	}

	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 2, model.FireWeaponPayload{
		Weapon:  combat.WeaponBaton,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	shooterFeedback = playerLastActionFeedbackForTest(manager, match.MatchID, "a")
	if shooterFeedback.Kind != model.ActionFeedbackKindStun || shooterFeedback.Level != model.ActionFeedbackLevelSuccess {
		t.Fatalf("expected shooter stun success feedback after baton, got %+v", shooterFeedback)
	}
	if !strings.Contains(shooterFeedback.Message, "Baton stunned") {
		t.Fatalf("expected baton stun feedback message for shooter, got %+v", shooterFeedback)
	}

	targetFeedback = playerLastActionFeedbackForTest(manager, match.MatchID, "b")
	if targetFeedback.Kind != model.ActionFeedbackKindStun || targetFeedback.Level != model.ActionFeedbackLevelWarning {
		t.Fatalf("expected target stun warning feedback after baton, got %+v", targetFeedback)
	}
	if !strings.Contains(targetFeedback.Message, "Stunned by Shooter") {
		t.Fatalf("expected baton stun feedback message for target, got %+v", targetFeedback)
	}
}

func TestAlarmDoorAndEscapeActionFeedback(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "fb-flow",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "gang", "Gang"); err != nil {
		t.Fatalf("join gang failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "gang", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "warden", gamemap.RoomPowerRoom)
	setPlayerRoomForTest(manager, match.MatchID, "gang", gamemap.RoomCourtyard)
	setPlayerInventoryForTest(manager, match.MatchID, "gang", nil)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "warden", 1, model.InteractPayload{})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	doorFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "warden")
	if doorFeedback.Kind != model.ActionFeedbackKindDoor {
		t.Fatalf("expected door/power feedback kind, got %+v", doorFeedback)
	}
	if !strings.Contains(doorFeedback.Message, "Power switched OFF") {
		t.Fatalf("expected power toggle feedback, got %+v", doorFeedback)
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "warden", 2, model.AbilityAlarm)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	alarmFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "warden")
	if alarmFeedback.Kind != model.ActionFeedbackKindAlarm || alarmFeedback.Level != model.ActionFeedbackLevelSuccess {
		t.Fatalf("expected alarm success feedback, got %+v", alarmFeedback)
	}
	if !strings.Contains(alarmFeedback.Message, "Alarm triggered") {
		t.Fatalf("expected alarm feedback message, got %+v", alarmFeedback)
	}

	mustSubmitInteract(t, manager, match.MatchID, "gang", 1, model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	escapeFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "gang")
	if escapeFeedback.Kind != model.ActionFeedbackKindEscape || escapeFeedback.Level != model.ActionFeedbackLevelWarning {
		t.Fatalf("expected escape warning feedback for failed route, got %+v", escapeFeedback)
	}
	if !strings.Contains(escapeFeedback.Message, string(model.EscapeRouteCourtyardDig)) {
		t.Fatalf("expected escape feedback to include attempted route, got %+v", escapeFeedback)
	}
}

func TestDoorActionFeedbackForCellDoorToggle(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "fb-door",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCellBlockA)
	cellID := playerAssignedCellForTest(manager, match.MatchID, "p1")
	if cellID == 0 {
		t.Fatalf("expected assigned cell")
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		TargetCellID: cellID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "p1")
	if feedback.Kind != model.ActionFeedbackKindDoor {
		t.Fatalf("expected door feedback kind for cell door toggle, got %+v", feedback)
	}
	if !strings.Contains(feedback.Message, "Cell door") {
		t.Fatalf("expected cell door feedback message, got %+v", feedback)
	}
}

func TestPurchaseActionFeedbackForMarketAndNPCBribe(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "fb-buy",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "buyer", "Buyer"); err != nil {
		t.Fatalf("join buyer failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "other", "Other"); err != nil {
		t.Fatalf("join other failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "buyer", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "buyer", gamemap.RoomCourtyard)
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomCourtyard)
	setPlayerCardsForTest(manager, match.MatchID, "buyer", []model.CardType{
		model.CardMoney,
		model.CardMoney,
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 1, model.ItemShiv)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	purchaseFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "buyer")
	if purchaseFeedback.Kind != model.ActionFeedbackKindPurchase || purchaseFeedback.Level != model.ActionFeedbackLevelSuccess {
		t.Fatalf("expected purchase success feedback, got %+v", purchaseFeedback)
	}
	if !strings.Contains(purchaseFeedback.Message, "Purchased shiv") {
		t.Fatalf("expected market purchase message, got %+v", purchaseFeedback)
	}

	setPlayerCardsForTest(manager, match.MatchID, "buyer", []model.CardType{
		model.CardMoney,
	})
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 2, model.ItemPistol)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	purchaseFeedback = playerLastActionFeedbackForTest(manager, match.MatchID, "buyer")
	if purchaseFeedback.Kind != model.ActionFeedbackKindPurchase || purchaseFeedback.Level != model.ActionFeedbackLevelWarning {
		t.Fatalf("expected purchase warning feedback for insufficient funds, got %+v", purchaseFeedback)
	}
	if !strings.Contains(purchaseFeedback.Message, "Need 3 money cards") {
		t.Fatalf("expected insufficient-funds feedback message, got %+v", purchaseFeedback)
	}

	npc := firstNPCPrisonerEntityForTest(manager, match.MatchID)
	setNPCPrisonerBribeStateForTest(manager, match.MatchID, npc.ID, npcPrisonerBribeState{
		OfferItem: model.ItemWood,
		OfferCost: 2,
		Stock:     1,
	})
	setPlayerRoomForTest(manager, match.MatchID, "buyer", npc.RoomID)
	setPlayerCardsForTest(manager, match.MatchID, "buyer", []model.CardType{
		model.CardMoney,
		model.CardMoney,
	})

	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "buyer", 3, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	bribeFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "buyer")
	if bribeFeedback.Kind != model.ActionFeedbackKindPurchase || bribeFeedback.Level != model.ActionFeedbackLevelInfo {
		t.Fatalf("expected bribe progress feedback, got %+v", bribeFeedback)
	}
	if !strings.Contains(bribeFeedback.Message, "Bribe progress 1/2") {
		t.Fatalf("expected bribe progress message, got %+v", bribeFeedback)
	}

	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "buyer", 4, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)

	bribeFeedback = playerLastActionFeedbackForTest(manager, match.MatchID, "buyer")
	if bribeFeedback.Kind != model.ActionFeedbackKindPurchase || bribeFeedback.Level != model.ActionFeedbackLevelSuccess {
		t.Fatalf("expected completed bribe success feedback, got %+v", bribeFeedback)
	}
	if !strings.Contains(bribeFeedback.Message, "NPC deal complete") {
		t.Fatalf("expected completed bribe message, got %+v", bribeFeedback)
	}
}

func playerLastActionFeedbackForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
) model.ActionFeedback {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.LastActionFeedback
		}
	}
	return model.ActionFeedback{}
}
