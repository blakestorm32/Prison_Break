package render

import (
	"encoding/json"
	"testing"
	"time"

	"prison-break/internal/client/input"
	"prison-break/internal/client/netclient"
	"prison-break/internal/client/prediction"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestShellResolveRenderStateInterpolatesAuthoritativeFrames(t *testing.T) {
	now := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	store := netclient.NewSnapshotStore()
	shell := NewShell(ShellConfig{
		ScreenWidth:   800,
		ScreenHeight:  600,
		LocalPlayerID: "local-not-present",
		Store:         store,
		Layout:        gamemap.DefaultPrisonLayout(),
		Now:           nowFn,
		PredictionEngine: prediction.NewEngine("local-not-present", prediction.Config{
			InterpolationBuffer: 100 * time.Millisecond,
			CorrectionBlend:     0,
			SnapThresholdTiles:  0.2,
			HistoryLimit:        32,
		}),
	})

	first := model.GameState{
		MatchID: "m-1",
		TickID:  10,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}, Alive: true},
		},
	}
	if !shell.ApplyAuthoritativeSnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: first.TickID,
		State:  &first,
	}) {
		t.Fatalf("expected first authoritative snapshot apply")
	}

	now = now.Add(200 * time.Millisecond)
	second := cloneStateForShellPredictionTest(first)
	second.TickID = 11
	second.Players[0].Position = model.Vector2{X: 20, Y: 0}

	if !shell.ApplyAuthoritativeSnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: second.TickID,
		State:  &second,
	}) {
		t.Fatalf("expected second authoritative snapshot apply")
	}

	// now=200ms and interpolation buffer=100ms => render target at 100ms, halfway between frames.
	rendered, ok := shell.resolveRenderState()
	if !ok {
		t.Fatalf("expected resolved render state")
	}
	position := findPlayerInStateForShellTest(rendered, "p1").Position
	assertNearValue(t, position.X, 10.0, 0.05)
}

func TestShellPredictionReconcilesLocalMovement(t *testing.T) {
	now := time.Date(2026, 2, 23, 12, 10, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	store := netclient.NewSnapshotStore()
	initial := model.GameState{
		MatchID: "m-2",
		TickID:  30,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}, Alive: true},
		},
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: initial.TickID,
		State:  &initial,
	}) {
		t.Fatalf("expected initial store snapshot apply")
	}

	fireInput := false
	shell := NewShell(ShellConfig{
		ScreenWidth:   800,
		ScreenHeight:  600,
		LocalPlayerID: "p1",
		Store:         store,
		Layout:        gamemap.DefaultPrisonLayout(),
		Now:           nowFn,
		PredictionEngine: prediction.NewEngine("p1", prediction.Config{
			InterpolationBuffer: 0,
			CorrectionBlend:     100 * time.Millisecond,
			SnapThresholdTiles:  2.0,
			HistoryLimit:        32,
		}),
		InputController: input.NewController(input.ControllerConfig{
			PlayerID: "p1",
		}),
		InputSnapshotProvider: func() input.InputSnapshot {
			return input.InputSnapshot{
				MoveRight:   true,
				FirePressed: fireInput,
				HasAim:      true,
				AimWorldX:   3,
				AimWorldY:   0,
			}
		},
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	commands := shell.DrainOutgoingCommands()
	if len(commands) == 0 {
		t.Fatalf("expected at least move command")
	}

	rendered0, ok := shell.resolveRenderState()
	if !ok {
		t.Fatalf("expected render state after local predicted input")
	}
	assertNearValue(t, findPlayerInStateForShellTest(rendered0, "p1").Position.X, 1.0, 0.05)

	now = now.Add(20 * time.Millisecond)
	delta, err := json.Marshal(model.GameDelta{
		ChangedPlayers: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0.5, Y: 0}, Alive: true},
		},
	})
	if err != nil {
		t.Fatalf("marshal delta failed: %v", err)
	}
	_ = delta // keep explicit compile-time payload creation pattern consistent with command payload tests.

	if !shell.ApplyAuthoritativeSnapshot(model.Snapshot{
		Kind:   model.SnapshotKindDelta,
		TickID: 31,
		Delta: &model.GameDelta{
			ChangedPlayers: []model.PlayerState{
				{ID: "p1", Position: model.Vector2{X: 0.5, Y: 0}, Alive: true},
			},
		},
		PlayerAcks: []model.PlayerAck{
			{PlayerID: "p1", LastProcessedClientSeq: commands[0].ClientSeq},
		},
	}) {
		t.Fatalf("expected authoritative delta apply")
	}

	now = now.Add(40 * time.Millisecond)
	rendered1, ok := shell.resolveRenderState()
	if !ok {
		t.Fatalf("expected render state after correction blend start")
	}
	assertNearValue(t, findPlayerInStateForShellTest(rendered1, "p1").Position.X, 0.8, 0.12)

	now = now.Add(100 * time.Millisecond)
	rendered2, ok := shell.resolveRenderState()
	if !ok {
		t.Fatalf("expected render state after correction blend end")
	}
	assertNearValue(t, findPlayerInStateForShellTest(rendered2, "p1").Position.X, 0.5, 0.06)
}

func TestShellUpdateReconcilesFromStoreSnapshotsAndDropsAckedPendingCommands(t *testing.T) {
	now := time.Date(2026, 2, 23, 13, 0, 0, 0, time.UTC)
	nowFn := func() time.Time { return now }

	store := netclient.NewSnapshotStore()
	initial := model.GameState{
		MatchID: "m-store-reconcile",
		TickID:  55,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}, Alive: true},
		},
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: initial.TickID,
		State:  &initial,
	}) {
		t.Fatalf("expected initial snapshot apply")
	}

	predictionEngine := prediction.NewEngine("p1", prediction.Config{
		InterpolationBuffer: 0,
		CorrectionBlend:     0,
		SnapThresholdTiles:  0.2,
		HistoryLimit:        32,
		PendingLimit:        64,
	})

	moveRight := true
	shell := NewShell(ShellConfig{
		ScreenWidth:      800,
		ScreenHeight:     600,
		LocalPlayerID:    "p1",
		Store:            store,
		Layout:           gamemap.DefaultPrisonLayout(),
		Now:              nowFn,
		PredictionEngine: predictionEngine,
		InputController: input.NewController(input.ControllerConfig{
			PlayerID: "p1",
		}),
		InputSnapshotProvider: func() input.InputSnapshot {
			return input.InputSnapshot{
				MoveRight: moveRight,
				HasAim:    true,
				AimWorldX: 2,
				AimWorldY: 0,
			}
		},
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	commands := shell.DrainOutgoingCommands()
	if len(commands) == 0 {
		t.Fatalf("expected local command generation")
	}
	lastFirstFrameSeq := commands[len(commands)-1].ClientSeq
	if pending := predictionEngine.PendingCommands(); len(pending) == 0 {
		t.Fatalf("expected pending prediction commands after local input")
	}

	now = now.Add(30 * time.Millisecond)
	moveRight = false
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindDelta,
		TickID: 56,
		Delta: &model.GameDelta{
			ChangedPlayers: []model.PlayerState{
				{ID: "p1", Position: model.Vector2{X: 1, Y: 0}, Alive: true},
			},
		},
		PlayerAcks: []model.PlayerAck{
			{PlayerID: "p1", LastProcessedClientSeq: lastFirstFrameSeq},
		},
	}) {
		t.Fatalf("expected authoritative delta apply to store")
	}

	if _, ok := shell.resolveRenderState(); !ok {
		t.Fatalf("expected resolved render state for reconciliation")
	}
	if pending := predictionEngine.PendingCommands(); len(pending) != 0 {
		t.Fatalf("expected acked pending commands to be dropped by reconciliation loop, got %+v", pending)
	}
}

func cloneStateForShellPredictionTest(in model.GameState) model.GameState {
	out := in
	out.Players = append([]model.PlayerState(nil), in.Players...)
	out.Entities = append([]model.EntityState(nil), in.Entities...)
	out.Map.Doors = append([]model.DoorState(nil), in.Map.Doors...)
	out.Map.Cells = append([]model.CellState(nil), in.Map.Cells...)
	out.Map.RestrictedZones = append([]model.ZoneState(nil), in.Map.RestrictedZones...)
	return out
}

func findPlayerInStateForShellTest(state model.GameState, playerID model.PlayerID) model.PlayerState {
	for _, player := range state.Players {
		if player.ID == playerID {
			return player
		}
	}
	return model.PlayerState{}
}

func assertNearValue(t *testing.T, got float32, want float32, tolerance float64) {
	t.Helper()
	diff := float64(got - want)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("value out of tolerance: got=%f want=%f tolerance=%f", got, want, tolerance)
	}
}
