package game

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestNightCardOfferAndSelectionFlow(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "night-card",
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
	setPlayerCardsForTest(manager, match.MatchID, "p1", nil)
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
	choices := playerNightCardChoicesForTest(manager, match.MatchID, "p1")
	if len(choices) != 3 {
		t.Fatalf("expected 3 night card choices, got %+v", choices)
	}

	selected := choices[0]
	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		NightCardChoice: selected,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "p1"), selected); got != 1 {
		t.Fatalf("expected selected night card to be granted once, card=%s cards=%+v", selected, playerCardsForTest(manager, match.MatchID, "p1"))
	}
	if remaining := playerNightCardChoicesForTest(manager, match.MatchID, "p1"); len(remaining) != 0 {
		t.Fatalf("expected night card choices to clear after selection, got %+v", remaining)
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		NightCardChoice: model.CardMoney,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "p1")
	if !strings.Contains(strings.ToLower(feedback.Message), "no night card selection is pending") {
		t.Fatalf("expected follow-up selection attempt to be denied, got %+v", feedback)
	}
}

func TestCellStashDepositAndWithdrawInAssignedCell(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "stash-flow",
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
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemWood, Quantity: 2},
	})
	assignedCell := playerAssignedCellForTest(manager, match.MatchID, "p1")
	if assignedCell == 0 {
		t.Fatalf("expected assigned cell for stash owner")
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		StashAction: "deposit",
		StashItem:   model.ItemWood,
		StashAmount: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	playerInventory := playerInventoryForTest(manager, match.MatchID, "p1")
	if !items.HasItem(model.PlayerState{Inventory: playerInventory}, model.ItemWood, 1) {
		t.Fatalf("expected deposit to leave one wood in inventory, got %+v", playerInventory)
	}
	stash := cellStashForPlayerForTest(manager, match.MatchID, "p1")
	if !items.HasItem(model.PlayerState{Inventory: stash}, model.ItemWood, 1) {
		t.Fatalf("expected cell stash to receive deposited wood, got %+v", stash)
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		StashAction: "withdraw",
		StashItem:   model.ItemWood,
		StashAmount: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	playerInventory = playerInventoryForTest(manager, match.MatchID, "p1")
	if !items.HasItem(model.PlayerState{Inventory: playerInventory}, model.ItemWood, 2) {
		t.Fatalf("expected withdraw to restore original inventory count, got %+v", playerInventory)
	}
	stash = cellStashForPlayerForTest(manager, match.MatchID, "p1")
	if items.HasItem(model.PlayerState{Inventory: stash}, model.ItemWood, 1) {
		t.Fatalf("expected stash wood to be removed after withdraw, got %+v", stash)
	}

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 3, model.InteractPayload{
		StashAction: "deposit",
		StashItem:   model.ItemWood,
		StashAmount: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "p1")
	if !strings.Contains(strings.ToLower(feedback.Message), "go to your cell block") {
		t.Fatalf("expected stash use outside cell block to be denied with guidance, got %+v", feedback)
	}
}

func TestNPCPrisonerTaskAssignAndCompletionRewardsMoneyCards(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "npc-task",
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
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p1", npc.RoomID)
	setPlayerCardsForTest(manager, match.MatchID, "p1", nil)
	setPhaseAndMarketForTest(manager, match.MatchID, model.PhaseDay, gamemap.RoomCourtyard)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		TargetEntityID: npc.ID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "p1")
	if !strings.Contains(feedback.Message, "Task assigned:") {
		t.Fatalf("expected first npc task interact to assign task, got %+v", feedback)
	}

	phase := playerPhaseForTest(manager, match.MatchID)
	setNPCTaskForPlayerForTest(manager, match.MatchID, "p1", npcTaskState{
		DayStartTick: phase.StartedTick,
		Type:         npcTaskHoldItem,
		TargetItem:   model.ItemWood,
		RewardCards:  2,
		AssignedBy:   npc.ID,
	})
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemWood, Quantity: 1},
	})

	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		TargetEntityID: npc.ID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if got := countCardForTest(playerCardsForTest(manager, match.MatchID, "p1"), model.CardMoney); got != 2 {
		t.Fatalf("expected completed npc task to grant 2 money cards, got %+v", playerCardsForTest(manager, match.MatchID, "p1"))
	}
	task, exists := npcTaskForPlayerForTest(manager, match.MatchID, "p1")
	if !exists || task.Type != "" {
		t.Fatalf("expected npc task state to reset after completion, got exists=%t task=%+v", exists, task)
	}
}

func TestDeterministicNPCTaskVisitRoomsAlwaysReachableByNPC(t *testing.T) {
	allowed := make(map[model.RoomID]struct{}, len(npcPrisonerTaskVisitRooms))
	for _, roomID := range npcPrisonerTaskVisitRooms {
		allowed[roomID] = struct{}{}
	}
	if len(allowed) == 0 {
		allowed[gamemap.RoomCorridorMain] = struct{}{}
	}

	for index := 0; index < 128; index++ {
		task := deterministicNPCTaskForDay(
			"task-reachability",
			model.PlayerID(fmt.Sprintf("p%d", index)),
			uint64(index+1),
			model.EntityID((index%7)+1),
		)
		if task.Type != npcTaskVisitRoom {
			continue
		}
		if _, ok := allowed[task.TargetRoomID]; !ok {
			t.Fatalf("expected visit-room task target to map to npc room, got %s", task.TargetRoomID)
		}
	}
}

func playerNightCardChoicesForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) []model.CardType {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return append([]model.CardType(nil), player.NightCardChoices...)
		}
	}
	return nil
}

func cellStashForPlayerForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) []model.ItemStack {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	var assignedCellID model.CellID
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			assignedCellID = player.AssignedCell
			break
		}
	}
	if assignedCellID == 0 {
		return nil
	}
	for _, cell := range session.gameState.Map.Cells {
		if cell.ID == assignedCellID {
			return append([]model.ItemStack(nil), cell.Stash...)
		}
	}
	return nil
}

func setNPCTaskForPlayerForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	task npcTaskState,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	if session.npcTaskByPlayer == nil {
		session.npcTaskByPlayer = make(map[model.PlayerID]npcTaskState)
	}
	session.npcTaskByPlayer[playerID] = task
}

func npcTaskForPlayerForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
) (npcTaskState, bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	task, exists := session.npcTaskByPlayer[playerID]
	return task, exists
}
