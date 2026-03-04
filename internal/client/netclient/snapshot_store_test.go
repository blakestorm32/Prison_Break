package netclient

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestSnapshotStoreApplyFullAndCurrentStateDeepCopy(t *testing.T) {
	store := NewSnapshotStore()

	fullState := model.GameState{
		MatchID: "m-1",
		TickID:  5,
		Status:  model.MatchStatusRunning,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    10,
		},
		Map: model.MapState{
			PowerOn:           true,
			BlackMarketRoomID: "black_market",
			Doors: []model.DoorState{
				{ID: 1, Open: true},
			},
		},
		Players: []model.PlayerState{
			{
				ID:         "p1",
				HeartsHalf: 6,
				Bullets:    2,
				Effects: []model.EffectState{
					{Effect: model.EffectSpeedBoost, EndsTick: 20},
				},
			},
		},
	}

	applied := store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 5,
		State:  &fullState,
	})
	if !applied {
		t.Fatalf("expected full snapshot to be applied")
	}

	current, ok := store.CurrentState()
	if !ok {
		t.Fatalf("expected current state after full snapshot")
	}
	if current.TickID != 5 || len(current.Players) != 1 || current.Players[0].HeartsHalf != 6 {
		t.Fatalf("unexpected current state after full snapshot: %+v", current)
	}

	current.Players[0].HeartsHalf = 1
	current.Map.Doors[0].Open = false

	reloaded, ok := store.CurrentState()
	if !ok {
		t.Fatalf("expected reloaded current state")
	}
	if reloaded.Players[0].HeartsHalf != 6 {
		t.Fatalf("expected stored state to remain immutable from caller mutation")
	}
	if !reloaded.Map.Doors[0].Open {
		t.Fatalf("expected stored door state to remain immutable from caller mutation")
	}

	meta, ok := store.LatestSnapshotMeta()
	if !ok {
		t.Fatalf("expected snapshot meta after full snapshot apply")
	}
	if meta.TickID != 5 {
		t.Fatalf("expected full snapshot meta tick=5, got %d", meta.TickID)
	}
}

func TestSnapshotStoreApplyDeltaUpdatesAuthoritativeState(t *testing.T) {
	store := NewSnapshotStore()

	base := model.GameState{
		MatchID:    "m-1",
		TickID:     10,
		Status:     model.MatchStatusRunning,
		CycleCount: 1,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    30,
		},
		Map: model.MapState{
			PowerOn:           true,
			BlackMarketRoomID: "black_market",
			Doors: []model.DoorState{
				{ID: 1, Open: true},
				{ID: 2, Open: true},
			},
			Cells: []model.CellState{
				{ID: 1, OccupantPlayerIDs: []model.PlayerID{"p1"}},
			},
		},
		Players: []model.PlayerState{
			{ID: "p1", HeartsHalf: 6, Bullets: 1},
			{ID: "p2", HeartsHalf: 6, Bullets: 3},
		},
		Entities: []model.EntityState{
			{ID: 100, Kind: model.EntityKindDroppedItem, Active: true},
		},
	}
	if !store.ApplySnapshot(model.Snapshot{Kind: model.SnapshotKindFull, TickID: 10, State: &base}) {
		t.Fatalf("expected base full snapshot apply")
	}

	nextPhase := model.PhaseState{
		Current:     model.PhaseNight,
		StartedTick: 30,
		EndsTick:    50,
	}
	nextCycle := uint8(2)
	nextPower := false
	nextMarket := model.RoomID("courtyard")
	gameOver := model.GameOverState{
		Reason:          model.WinReasonMaxCyclesReached,
		EndedTick:       11,
		WinnerPlayerIDs: []model.PlayerID{"p2"},
	}

	applied := store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindDelta,
		TickID: 11,
		Delta: &model.GameDelta{
			ChangedPlayers: []model.PlayerState{
				{
					ID:         "p1",
					HeartsHalf: 4,
					Bullets:    0,
					Effects: []model.EffectState{
						{Effect: model.EffectStunned, EndsTick: 20},
					},
				},
			},
			RemovedPlayerIDs: []model.PlayerID{"p2"},
			ChangedDoors: []model.DoorState{
				{ID: 1, Open: false},
			},
			ChangedCells: []model.CellState{
				{ID: 1, OccupantPlayerIDs: []model.PlayerID{"p1", "p3"}},
			},
			ChangedEntities: []model.EntityState{
				{ID: 101, Kind: model.EntityKindNPCGuard, Active: true},
			},
			RemovedEntityIDs:  []model.EntityID{100},
			Phase:             &nextPhase,
			CycleCount:        &nextCycle,
			PowerOn:           &nextPower,
			BlackMarketRoomID: &nextMarket,
			GameOver:          &gameOver,
		},
	})
	if !applied {
		t.Fatalf("expected delta snapshot to be applied")
	}

	current, ok := store.CurrentState()
	if !ok {
		t.Fatalf("expected current state")
	}

	if current.TickID != 11 {
		t.Fatalf("expected tick to advance to 11, got %d", current.TickID)
	}
	if current.Phase.Current != model.PhaseNight || current.CycleCount != 2 {
		t.Fatalf("expected phase/cycle updates from delta, got phase=%s cycle=%d", current.Phase.Current, current.CycleCount)
	}
	if current.Map.PowerOn {
		t.Fatalf("expected map power off from delta")
	}
	if current.Map.BlackMarketRoomID != "courtyard" {
		t.Fatalf("expected black market room to be courtyard, got %s", current.Map.BlackMarketRoomID)
	}
	if len(current.Players) != 1 || current.Players[0].ID != "p1" {
		t.Fatalf("expected p2 removal and p1 update, got players=%+v", current.Players)
	}
	if current.Players[0].HeartsHalf != 4 || current.Players[0].Bullets != 0 {
		t.Fatalf("expected p1 heart/ammo update from delta, got %+v", current.Players[0])
	}
	if len(current.Entities) != 1 || current.Entities[0].ID != 101 {
		t.Fatalf("expected entity remove/upsert from delta, got entities=%+v", current.Entities)
	}
	if len(current.Map.Cells) != 1 || len(current.Map.Cells[0].OccupantPlayerIDs) != 2 {
		t.Fatalf("expected cell occupants update from delta, got cells=%+v", current.Map.Cells)
	}
	if current.GameOver == nil || current.GameOver.Reason != model.WinReasonMaxCyclesReached {
		t.Fatalf("expected game-over payload update from delta, got %+v", current.GameOver)
	}

	meta, ok := store.LatestSnapshotMeta()
	if !ok {
		t.Fatalf("expected snapshot meta after delta apply")
	}
	if meta.TickID != 11 {
		t.Fatalf("expected delta snapshot meta tick=11, got %d", meta.TickID)
	}
}

func TestSnapshotStoreRejectsInvalidOrStaleDelta(t *testing.T) {
	store := NewSnapshotStore()

	if store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindDelta,
		TickID: 1,
		Delta:  &model.GameDelta{},
	}) {
		t.Fatalf("expected delta apply to fail without baseline full state")
	}

	base := model.GameState{
		MatchID: "m-1",
		TickID:  5,
		Status:  model.MatchStatusRunning,
		Map:     model.MapState{PowerOn: true},
	}
	if !store.ApplySnapshot(model.Snapshot{Kind: model.SnapshotKindFull, TickID: 5, State: &base}) {
		t.Fatalf("expected full baseline apply")
	}

	if store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindDelta,
		TickID: 5,
		Delta: &model.GameDelta{
			ChangedPlayers: []model.PlayerState{{ID: "p1", HeartsHalf: 1}},
		},
	}) {
		t.Fatalf("expected stale delta with equal tick to be rejected")
	}

	current, ok := store.CurrentState()
	if !ok {
		t.Fatalf("expected current state")
	}
	if current.TickID != 5 || len(current.Players) != 0 {
		t.Fatalf("expected stale delta rejection to preserve state, got %+v", current)
	}

	meta, ok := store.LatestSnapshotMeta()
	if !ok {
		t.Fatalf("expected snapshot meta from baseline full state")
	}
	if meta.TickID != 5 {
		t.Fatalf("expected stale delta rejection to preserve meta tick 5, got %d", meta.TickID)
	}
}

func TestSnapshotStoreLatestSnapshotMetaCopiesPlayerAcks(t *testing.T) {
	store := NewSnapshotStore()
	fullState := model.GameState{
		MatchID: "m-ack",
		TickID:  9,
		Status:  model.MatchStatusRunning,
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 9,
		State:  &fullState,
		PlayerAcks: []model.PlayerAck{
			{PlayerID: "p1", LastProcessedClientSeq: 7},
		},
	}) {
		t.Fatalf("expected full snapshot apply")
	}

	meta, ok := store.LatestSnapshotMeta()
	if !ok {
		t.Fatalf("expected snapshot meta")
	}
	if len(meta.PlayerAcks) != 1 || meta.PlayerAcks[0].LastProcessedClientSeq != 7 {
		t.Fatalf("unexpected player acks in meta: %+v", meta.PlayerAcks)
	}

	meta.PlayerAcks[0].LastProcessedClientSeq = 999
	meta2, ok := store.LatestSnapshotMeta()
	if !ok {
		t.Fatalf("expected second snapshot meta read")
	}
	if meta2.PlayerAcks[0].LastProcessedClientSeq != 7 {
		t.Fatalf("expected snapshot meta to remain immutable from caller mutation, got %+v", meta2.PlayerAcks)
	}
}
