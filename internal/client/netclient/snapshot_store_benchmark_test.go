package netclient

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func BenchmarkSnapshotStoreApplyDelta(b *testing.B) {
	store := NewSnapshotStore()

	basePlayers := make([]model.PlayerState, 0, 12)
	for i := 0; i < 12; i++ {
		basePlayers = append(basePlayers, model.PlayerState{
			ID:            model.PlayerID("p" + string(rune('a'+i))),
			Alive:         true,
			Position:      model.Vector2{X: float32(i), Y: float32(i % 4)},
			CurrentRoomID: gamemap.RoomCorridorMain,
			HeartsHalf:    6,
		})
	}
	baseState := model.GameState{
		MatchID: "bench-snapshot-store",
		TickID:  1,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: basePlayers,
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 1,
		State:  &baseState,
	}) {
		b.Fatalf("failed to seed snapshot store baseline state")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		targetTick := uint64(i + 2)
		delta := model.Snapshot{
			Kind:   model.SnapshotKindDelta,
			TickID: targetTick,
			Delta: &model.GameDelta{
				ChangedPlayers: []model.PlayerState{
					{
						ID:            "pa",
						Alive:         true,
						Position:      model.Vector2{X: float32(i % 21), Y: float32((i / 3) % 11)},
						CurrentRoomID: gamemap.RoomCorridorMain,
						HeartsHalf:    6,
					},
				},
			},
		}
		if !store.ApplySnapshot(delta) {
			b.Fatalf("apply delta failed at iteration %d (tick %d)", i, targetTick)
		}
	}
}
