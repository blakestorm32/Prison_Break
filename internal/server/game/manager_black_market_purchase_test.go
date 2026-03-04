package game

import (
	"encoding/json"
	"testing"
	"time"

	gameitems "prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestBlackMarketPurchaseConsumesMoneyCardsAndGrantsItem(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "market-buy",
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
		model.CardSpeed,
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 1, model.ItemShiv)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "buyer")
	if !gameitems.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected purchased shiv in buyer inventory, got %+v", inventory)
	}
	cards := playerCardsForTest(manager, match.MatchID, "buyer")
	if countCardForTest(cards, model.CardMoney) != 0 {
		t.Fatalf("expected 2 money cards to be consumed, got %+v", cards)
	}
	if countCardForTest(cards, model.CardSpeed) != 1 {
		t.Fatalf("expected non-currency cards to remain, got %+v", cards)
	}
}

func TestBlackMarketPurchaseRequiresNightAndMatchingMarketRoom(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "market-gate",
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
	setPlayerCardsForTest(manager, match.MatchID, "buyer", []model.CardType{
		model.CardMoney,
		model.CardMoney,
		model.CardMoney,
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// Day-phase attempt should fail.
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseDay, gamemap.RoomCourtyard)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 1, model.ItemPistol)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "buyer")}, model.ItemPistol, 1) {
		t.Fatalf("expected day-phase market purchase to be rejected")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "buyer"), model.CardMoney) != 3 {
		t.Fatalf("expected day-phase rejection to preserve money cards")
	}

	// Night-phase in a non-market room should fail.
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomMailRoom)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 2, model.ItemPistol)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "buyer")}, model.ItemPistol, 1) {
		t.Fatalf("expected off-location market purchase to be rejected")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "buyer"), model.CardMoney) != 3 {
		t.Fatalf("expected off-location rejection to preserve money cards")
	}

	// Correct phase and room should succeed.
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomCourtyard)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "buyer", 3, model.ItemPistol)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	if !gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "buyer")}, model.ItemPistol, 1) {
		t.Fatalf("expected valid market purchase to grant pistol")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "buyer"), model.CardMoney) != 0 {
		t.Fatalf("expected successful purchase to consume money cards")
	}
}

func TestBlackMarketPurchaseFailsForInsufficientFundsAndAuthorityPlayers(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "market-auth",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join prisoner failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join authority failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCourtyard)
	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomCourtyard)
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomCourtyard)
	setPlayerCardsForTest(manager, match.MatchID, "pris", []model.CardType{
		model.CardMoney,
	})
	setPlayerCardsForTest(manager, match.MatchID, "auth", []model.CardType{
		model.CardMoney,
		model.CardMoney,
		model.CardMoney,
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "pris", 1, model.ItemPistol)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "auth", 1, model.ItemShiv)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "pris")}, model.ItemPistol, 1) {
		t.Fatalf("expected insufficient-funds purchase to be rejected")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "pris"), model.CardMoney) != 1 {
		t.Fatalf("expected insufficient-funds rejection to preserve money cards")
	}

	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "auth")}, model.ItemShiv, 1) {
		t.Fatalf("expected authority purchase to be rejected")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "auth"), model.CardMoney) != 3 {
		t.Fatalf("expected authority rejection to preserve money cards")
	}
}

func TestBlackMarketGoldenBulletIsOnePerMatch(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    10,
			MatchIDPrefix: "market-gold",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	for _, playerID := range []model.PlayerID{"p1", "p2", "p3"} {
		if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
			t.Fatalf("join %s failed: %v", playerID, err)
		}
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p2", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p3", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCourtyard)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCourtyard)
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomCourtyard)
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{
		model.CardMoney,
		model.CardMoney,
		model.CardMoney,
	})
	setPlayerCardsForTest(manager, match.MatchID, "p2", []model.CardType{
		model.CardMoney,
		model.CardMoney,
		model.CardMoney,
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "p1", 1, model.ItemGoldenBullet)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "p2", 1, model.ItemGoldenBullet)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if !gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p1")}, model.ItemGoldenBullet, 1) {
		t.Fatalf("expected first purchase to grant golden bullet")
	}
	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p2")}, model.ItemGoldenBullet, 1) {
		t.Fatalf("expected second same-tick purchase to be rejected")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "p2"), model.CardMoney) != 3 {
		t.Fatalf("expected rejected golden-bullet purchase to preserve p2 money cards")
	}

	// Simulate the first bullet leaving inventory; one-per-match lock should still hold.
	setPlayerInventoryForTest(manager, match.MatchID, "p1", nil)
	mustSubmitBlackMarketBuy(t, manager, match.MatchID, "p2", 2, model.ItemGoldenBullet)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p2")}, model.ItemGoldenBullet, 1) {
		t.Fatalf("expected one-per-match golden bullet rule to persist after first holder loses item")
	}
	if countCardForTest(playerCardsForTest(manager, match.MatchID, "p2"), model.CardMoney) != 3 {
		t.Fatalf("expected second rejected golden-bullet purchase to preserve p2 money cards")
	}
}

func TestMoneyCardCannotBeConsumedThroughUseCardCommand(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "market-money",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardMoney})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardMoney)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	cards := playerCardsForTest(manager, match.MatchID, "p1")
	if countCardForTest(cards, model.CardMoney) != 1 {
		t.Fatalf("expected money card to remain when used outside market purchase flow, got %+v", cards)
	}
	if gameitems.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p1")}, model.ItemWood, 1) {
		t.Fatalf("expected money card use to no longer convert into wood inventory")
	}
}

func mustSubmitBlackMarketBuy(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	item model.ItemType,
) {
	t.Helper()

	raw, err := json.Marshal(model.BlackMarketPurchasePayload{
		Item: item,
	})
	if err != nil {
		t.Fatalf("marshal black-market payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdBlackMarketBuy,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit black-market input failed: %v", err)
	}
}

func setPhaseAndMarketForTest(
	manager *Manager,
	matchID model.MatchID,
	phaseType model.PhaseType,
	marketRoom model.RoomID,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	session.gameState.Phase.Current = phaseType
	session.gameState.Map.BlackMarketRoomID = marketRoom
}

func countCardForTest(cards []model.CardType, target model.CardType) int {
	count := 0
	for _, card := range cards {
		if card == target {
			count++
		}
	}
	return count
}
