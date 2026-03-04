package game

import (
	"encoding/json"
	"testing"
	"time"

	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestNPCPrisonerEntitiesSpawnWithBlackMarketLinkedOffers(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "npc-spawn",
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

	entities := entitiesForTest(manager, match.MatchID)
	npcCount := 0
	for _, entity := range entities {
		if entity.Kind != model.EntityKindNPCPrisoner {
			continue
		}
		npcCount++

		state := npcPrisonerBribeStateForTest(manager, match.MatchID, entity.ID)
		if state.OfferItem == "" || state.OfferCost == 0 {
			t.Fatalf("expected npc prisoner %d offer state, got %+v", entity.ID, state)
		}
		offer, exists := items.BlackMarketOfferForItem(state.OfferItem)
		if !exists {
			t.Fatalf("expected npc offer item to exist in market catalog, got %+v", state)
		}
		if offer.MoneyCardCost != state.OfferCost {
			t.Fatalf("expected npc offer cost to mirror market cost for %s: market=%d npc=%d", state.OfferItem, offer.MoneyCardCost, state.OfferCost)
		}
		if state.Stock != 1 {
			t.Fatalf("expected npc offer stock to start at 1, got %+v", state)
		}
	}
	if npcCount != len(npcPrisonerRooms) {
		t.Fatalf("expected %d npc prisoner entities, got %d", len(npcPrisonerRooms), npcCount)
	}
}

func TestMoneyCardBribeProgressesAndDispensesItemDeterministically(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "npc-bribe",
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

	npc := firstNPCPrisonerEntityForTest(manager, match.MatchID)
	setNPCPrisonerBribeStateForTest(manager, match.MatchID, npc.ID, npcPrisonerBribeState{
		OfferItem: model.ItemWireCutters,
		OfferCost: 2,
		Stock:     1,
	})

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p1", npc.RoomID)
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, npc.RoomID)
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardMoney, model.CardMoney})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "p1", 1, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "p1"), model.CardMoney); got != 1 {
		t.Fatalf("expected first bribe payment to consume one money card, got %+v", playerCardsForTest(manager, match.MatchID, "p1"))
	}
	if items.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p1")}, model.ItemWireCutters, 1) {
		t.Fatalf("expected first partial payment not to grant item yet")
	}
	state := npcPrisonerBribeStateForTest(manager, match.MatchID, npc.ID)
	if state.PayerID != "p1" || state.PaidCards != 1 || state.Stock != 1 {
		t.Fatalf("expected partial bribe progress state, got %+v", state)
	}

	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "p1", 2, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "p1"), model.CardMoney); got != 0 {
		t.Fatalf("expected second payment to consume remaining money card, got %+v", playerCardsForTest(manager, match.MatchID, "p1"))
	}
	if !items.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "p1")}, model.ItemWireCutters, 1) {
		t.Fatalf("expected completed bribe to grant configured item")
	}
	state = npcPrisonerBribeStateForTest(manager, match.MatchID, npc.ID)
	if state.Stock != 0 || state.PaidCards != 0 || state.PayerID != "" {
		t.Fatalf("expected completed bribe to consume stock and reset progress, got %+v", state)
	}

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardMoney})
	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "p1", 3, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "p1"), model.CardMoney); got != 1 {
		t.Fatalf("expected sold-out npc bribe to preserve money card, got %+v", playerCardsForTest(manager, match.MatchID, "p1"))
	}
}

func TestMoneyCardBribeGatesByPhaseRoomAndFaction(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "npc-gate",
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

	npc := firstNPCPrisonerEntityForTest(manager, match.MatchID)
	setNPCPrisonerBribeStateForTest(manager, match.MatchID, npc.ID, npcPrisonerBribeState{
		OfferItem: model.ItemWood,
		OfferCost: 1,
		Stock:     1,
	})

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerCardsForTest(manager, match.MatchID, "pris", []model.CardType{model.CardMoney})
	setPlayerCardsForTest(manager, match.MatchID, "auth", []model.CardType{model.CardMoney})
	setPlayerRoomForTest(manager, match.MatchID, "pris", npc.RoomID)
	setPlayerRoomForTest(manager, match.MatchID, "auth", npc.RoomID)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// Day: blocked.
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseDay, gamemap.RoomCourtyard)
	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "pris", 1, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "pris"), model.CardMoney); got != 1 {
		t.Fatalf("expected day-phase bribe attempt to preserve money card, got %+v", playerCardsForTest(manager, match.MatchID, "pris"))
	}

	// Night but cross-room: blocked.
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseNight, gamemap.RoomCourtyard)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCellBlockA)
	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "pris", 2, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "pris"), model.CardMoney); got != 1 {
		t.Fatalf("expected cross-room bribe attempt to preserve money card, got %+v", playerCardsForTest(manager, match.MatchID, "pris"))
	}

	// Authority cannot bribe NPC prisoner.
	mustSubmitUseMoneyCardOnEntityForTest(t, manager, match.MatchID, "auth", 1, npc.ID)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "auth"), model.CardMoney); got != 1 {
		t.Fatalf("expected authority bribe attempt to preserve money card, got %+v", playerCardsForTest(manager, match.MatchID, "auth"))
	}
}

func TestNPCPrisonerBribeOffersRefreshOnNightStart(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "npc-refresh",
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

	npc := firstNPCPrisonerEntityForTest(manager, match.MatchID)
	setNPCPrisonerBribeStateForTest(manager, match.MatchID, npc.ID, npcPrisonerBribeState{
		OfferItem: model.ItemWood,
		OfferCost: 1,
		Stock:     0,
		PayerID:   "p1",
		PaidCards: 1,
	})

	setPhaseEndTickForTest(manager, match.MatchID, 1)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	phase := playerPhaseForTest(manager, match.MatchID)
	if phase.Current != model.PhaseNight {
		t.Fatalf("expected phase transition to night at tick 1, got %+v", phase)
	}

	expectedItem, expectedCost, ok := deterministicNPCPrisonerOffer(match.MatchID, npc.RoomID, npc.ID, 0)
	if !ok {
		t.Fatalf("expected deterministic npc offer to resolve")
	}
	state := npcPrisonerBribeStateForTest(manager, match.MatchID, npc.ID)
	if state.OfferItem != expectedItem || state.OfferCost != expectedCost || state.Stock != 1 || state.PayerID != "" || state.PaidCards != 0 {
		t.Fatalf("expected night refresh to reset npc state to deterministic offer, got %+v expected item=%s cost=%d", state, expectedItem, expectedCost)
	}

	snapshots, err := manager.SnapshotsSince(match.MatchID, 0)
	if err != nil {
		t.Fatalf("snapshots since failed: %v", err)
	}
	if len(snapshots) == 0 || snapshots[0].Delta == nil {
		t.Fatalf("expected delta snapshot at night transition")
	}
	if !deltaContainsEntityIDForTest(snapshots[0].Delta.ChangedEntities, npc.ID) {
		t.Fatalf("expected night refresh delta to include changed npc entity id %d, got %+v", npc.ID, snapshots[0].Delta.ChangedEntities)
	}
}

func mustSubmitUseMoneyCardOnEntityForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	targetEntityID model.EntityID,
) {
	t.Helper()

	raw, err := json.Marshal(model.CardUsePayload{
		Card:           model.CardMoney,
		TargetEntityID: targetEntityID,
	})
	if err != nil {
		t.Fatalf("marshal money-card payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseCard,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit money-card input failed: %v", err)
	}
}

func firstNPCPrisonerEntityForTest(manager *Manager, matchID model.MatchID) model.EntityState {
	entities := entitiesForTest(manager, matchID)
	for _, entity := range entities {
		if entity.Kind == model.EntityKindNPCPrisoner {
			return entity
		}
	}
	return model.EntityState{}
}

func setNPCPrisonerBribeStateForTest(
	manager *Manager,
	matchID model.MatchID,
	entityID model.EntityID,
	state npcPrisonerBribeState,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	session.npcPrisonerBribeState[entityID] = state
	for index := range session.gameState.Entities {
		if session.gameState.Entities[index].ID != entityID {
			continue
		}
		session.gameState.Entities[index].Tags = npcPrisonerTags(state)
		return
	}
}

func npcPrisonerBribeStateForTest(
	manager *Manager,
	matchID model.MatchID,
	entityID model.EntityID,
) npcPrisonerBribeState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	return session.npcPrisonerBribeState[entityID]
}

func deltaContainsEntityIDForTest(entities []model.EntityState, entityID model.EntityID) bool {
	for _, entity := range entities {
		if entity.ID == entityID {
			return true
		}
	}
	return false
}
