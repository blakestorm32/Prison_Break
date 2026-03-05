package game

import (
	"encoding/json"
	"testing"
	"time"

	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestDropItemCreatesDroppedEntityAndEntityDelta(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "itm-drop",
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

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 2},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitDropItem(t, manager, match.MatchID, "p1", 1, model.ItemShiv, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "p1")
	if !items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected player inventory to keep one shiv after dropping one")
	}

	entities := entitiesForTest(manager, match.MatchID)
	dropped, found := firstEntityByKindForTest(entities, model.EntityKindDroppedItem)
	if !found {
		t.Fatalf("expected one dropped-item entity in state, got %+v", entities)
	}
	if dropped.Kind != model.EntityKindDroppedItem {
		t.Fatalf("expected dropped item entity kind, got %s", dropped.Kind)
	}
	if dropped.OwnerPlayerID != "p1" {
		t.Fatalf("expected dropped item owner p1, got %s", dropped.OwnerPlayerID)
	}
	if dropped.RoomID != gamemap.RoomCorridorMain {
		t.Fatalf("expected dropped item room corridor_main, got %s", dropped.RoomID)
	}

	item, quantity, ok := items.ParseDroppedItem(dropped)
	if !ok {
		t.Fatalf("expected dropped item tags to parse")
	}
	if item != model.ItemShiv || quantity != 1 {
		t.Fatalf("unexpected dropped item payload: item=%s quantity=%d", item, quantity)
	}

	snapshots, err := manager.SnapshotsSince(match.MatchID, 0)
	if err != nil {
		t.Fatalf("snapshots since failed: %v", err)
	}
	if len(snapshots) == 0 || snapshots[0].Delta == nil {
		t.Fatalf("expected first tick delta snapshot")
	}
	if countEntitiesByKindForTest(snapshots[0].Delta.ChangedEntities, model.EntityKindDroppedItem) != 1 {
		t.Fatalf("expected changed entity delta after drop, got %#v", snapshots[0].Delta.ChangedEntities)
	}
}

func TestPickupDroppedItemRequiresRoomAndCapacity(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "itm-pick",
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

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCellBlockA)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitDropItem(t, manager, match.MatchID, "p1", 1, model.ItemShiv, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	entities := entitiesForTest(manager, match.MatchID)
	dropped, found := firstEntityByKindForTest(entities, model.EntityKindDroppedItem)
	if !found {
		t.Fatalf("expected one dropped-item entity, got %+v", entities)
	}
	entityID := dropped.ID

	// Different room cannot pick up.
	mustSubmitInteract(t, manager, match.MatchID, "p2", 1, model.InteractPayload{
		TargetEntityID: entityID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if countEntitiesByKindForTest(entitiesForTest(manager, match.MatchID), model.EntityKindDroppedItem) != 1 {
		t.Fatalf("expected dropped entity to remain when picker is in another room")
	}

	// Same room but full inventory cannot pick up new stack.
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "p2", []model.ItemStack{
		{Item: model.ItemBadge, Quantity: 1},
		{Item: model.ItemBullet, Quantity: 1},
		{Item: model.ItemDoorStop, Quantity: 1},
	})

	mustSubmitInteract(t, manager, match.MatchID, "p2", 2, model.InteractPayload{
		TargetEntityID: entityID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if countEntitiesByKindForTest(entitiesForTest(manager, match.MatchID), model.EntityKindDroppedItem) != 1 {
		t.Fatalf("expected dropped entity to remain when picker has no capacity")
	}

	// Free capacity then pickup succeeds.
	setPlayerInventoryForTest(manager, match.MatchID, "p2", []model.ItemStack{
		{Item: model.ItemBadge, Quantity: 1},
		{Item: model.ItemBullet, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p2", 3, model.InteractPayload{
		TargetEntityID: entityID,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)

	if countEntitiesByKindForTest(entitiesForTest(manager, match.MatchID), model.EntityKindDroppedItem) != 0 {
		t.Fatalf("expected dropped entity to be removed after successful pickup")
	}

	p2Inventory := playerInventoryForTest(manager, match.MatchID, "p2")
	if !items.HasItem(model.PlayerState{Inventory: p2Inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected p2 to receive picked-up shiv")
	}
	if !items.HasContraband(model.PlayerState{Inventory: p2Inventory}) {
		t.Fatalf("expected picked-up shiv to register as contraband")
	}

	recentSnapshots, err := manager.SnapshotsSince(match.MatchID, 3)
	if err != nil {
		t.Fatalf("snapshots since tick 3 failed: %v", err)
	}
	if len(recentSnapshots) == 0 || recentSnapshots[0].Delta == nil {
		t.Fatalf("expected delta snapshot for pickup tick")
	}
	if len(recentSnapshots[0].Delta.RemovedEntityIDs) != 1 || recentSnapshots[0].Delta.RemovedEntityIDs[0] != entityID {
		t.Fatalf("expected removed entity id %d in pickup delta, got %#v", entityID, recentSnapshots[0].Delta.RemovedEntityIDs)
	}
}

func TestUseItemTransfersOnlyWhenValid(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "itm-xfer",
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

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemBullet, Quantity: 2},
		{Item: model.ItemShiv, Quantity: 1},
	})
	setPlayerInventoryForTest(manager, match.MatchID, "p2", nil)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseItemTransfer(t, manager, match.MatchID, "p1", 1, model.ItemBullet, 2, "p2")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	p1Inventory := playerInventoryForTest(manager, match.MatchID, "p1")
	p2Inventory := playerInventoryForTest(manager, match.MatchID, "p2")
	if items.HasItem(model.PlayerState{Inventory: p1Inventory}, model.ItemBullet, 1) {
		t.Fatalf("expected p1 bullets to transfer out")
	}
	if !items.HasItem(model.PlayerState{Inventory: p2Inventory}, model.ItemBullet, 2) {
		t.Fatalf("expected p2 bullets to transfer in")
	}

	// Full destination capacity blocks transfer.
	setPlayerInventoryForTest(manager, match.MatchID, "p2", []model.ItemStack{
		{Item: model.ItemBadge, Quantity: 1},
		{Item: model.ItemBullet, Quantity: 2},
		{Item: model.ItemDoorStop, Quantity: 1},
		{Item: model.ItemKeys, Quantity: 1},
		{Item: model.ItemMetalSlab, Quantity: 1},
		{Item: model.ItemWood, Quantity: 1},
	})

	mustSubmitUseItemTransfer(t, manager, match.MatchID, "p1", 2, model.ItemShiv, 1, "p2")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	p1Inventory = playerInventoryForTest(manager, match.MatchID, "p1")
	p2Inventory = playerInventoryForTest(manager, match.MatchID, "p2")
	if !items.HasItem(model.PlayerState{Inventory: p1Inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected p1 to retain shiv when destination capacity blocks transfer")
	}
	if items.HasItem(model.PlayerState{Inventory: p2Inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected p2 not to receive shiv when transfer is invalid")
	}

	// Different room blocks transfer.
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCellBlockA)
	mustSubmitUseItemTransfer(t, manager, match.MatchID, "p1", 3, model.ItemShiv, 1, "p2")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	p2Inventory = playerInventoryForTest(manager, match.MatchID, "p2")
	if items.HasItem(model.PlayerState{Inventory: p2Inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected cross-room transfer to remain blocked")
	}
}

func TestCraftItemConsumesInputsAndFlagsContrabandOutput(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "itm-craft",
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

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemMetalSlab, Quantity: 1},
		{Item: model.ItemWood, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitCraftItem(t, manager, match.MatchID, "p1", 1, model.ItemShiv)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "p1")
	player := model.PlayerState{Inventory: inventory}
	if !items.HasItem(player, model.ItemShiv, 1) {
		t.Fatalf("expected crafted shiv output in inventory")
	}
	if items.HasItem(player, model.ItemWood, 1) || items.HasItem(player, model.ItemMetalSlab, 1) {
		t.Fatalf("expected craft input materials consumed")
	}
	if !items.HasContraband(player) {
		t.Fatalf("expected crafted shiv to be detected as contraband")
	}

	// No materials left; craft should no-op and preserve current inventory.
	before := append([]model.ItemStack(nil), inventory...)
	mustSubmitCraftItem(t, manager, match.MatchID, "p1", 2, model.ItemShiv)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	after := playerInventoryForTest(manager, match.MatchID, "p1")
	if len(after) != len(before) {
		t.Fatalf("expected failed craft to preserve inventory size, got before=%v after=%v", before, after)
	}
	if !items.HasItem(model.PlayerState{Inventory: after}, model.ItemShiv, 1) {
		t.Fatalf("expected existing crafted shiv to remain after failed craft attempt")
	}
}

func mustSubmitDropItem(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	item model.ItemType,
	amount uint8,
) {
	t.Helper()

	raw, err := json.Marshal(model.DropItemPayload{
		Item:   item,
		Amount: amount,
	})
	if err != nil {
		t.Fatalf("marshal drop payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdDropItem,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit drop item input failed: %v", err)
	}
}

func mustSubmitUseItemTransfer(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	item model.ItemType,
	amount uint8,
	target model.PlayerID,
) {
	t.Helper()

	raw, err := json.Marshal(model.ItemUsePayload{
		Item:           item,
		Amount:         amount,
		TargetPlayerID: target,
	})
	if err != nil {
		t.Fatalf("marshal use-item payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseItem,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit use item input failed: %v", err)
	}
}

func mustSubmitCraftItem(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	item model.ItemType,
) {
	t.Helper()

	raw, err := json.Marshal(model.CraftItemPayload{
		Item: item,
	})
	if err != nil {
		t.Fatalf("marshal craft payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdCraftItem,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit craft input failed: %v", err)
	}
}

func setPlayerInventoryForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	inventory []model.ItemStack,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].Inventory = append([]model.ItemStack(nil), inventory...)
		return
	}
}

func playerInventoryForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
) []model.ItemStack {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		return append([]model.ItemStack(nil), session.gameState.Players[idx].Inventory...)
	}
	return nil
}

func entitiesForTest(manager *Manager, matchID model.MatchID) []model.EntityState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	out := make([]model.EntityState, len(session.gameState.Entities))
	copy(out, session.gameState.Entities)
	for idx := range out {
		out[idx].Tags = append([]string(nil), out[idx].Tags...)
	}
	return out
}

func firstEntityByKindForTest(
	entities []model.EntityState,
	kind model.EntityKind,
) (model.EntityState, bool) {
	for _, entity := range entities {
		if entity.Kind == kind {
			return entity, true
		}
	}
	return model.EntityState{}, false
}

func countEntitiesByKindForTest(entities []model.EntityState, kind model.EntityKind) int {
	count := 0
	for _, entity := range entities {
		if entity.Kind == kind {
			count++
		}
	}
	return count
}
