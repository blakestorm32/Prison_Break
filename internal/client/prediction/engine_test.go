package prediction

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"prison-break/internal/engine/physics"
	"prison-break/internal/shared/model"
)

func TestRenderStateInterpolatesBetweenTimedAuthoritativeFrames(t *testing.T) {
	engine := NewEngine("p1", Config{
		InterpolationBuffer: 0,
		CorrectionBlend:     0,
		SnapThresholdTiles:  0.2,
		HistoryLimit:        16,
	})

	t0 := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	stateA := model.GameState{
		TickID: 10,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}},
			{ID: "p2", Position: model.Vector2{X: 10, Y: 0}},
		},
	}
	stateB := model.GameState{
		TickID: 11,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 10, Y: 0}},
			{ID: "p2", Position: model.Vector2{X: 20, Y: 0}},
		},
	}

	engine.SeedAuthoritativeState(stateA, t0)
	engine.SeedAuthoritativeState(stateB, t0.Add(100*time.Millisecond))

	rendered, ok := engine.RenderState(t0.Add(50 * time.Millisecond))
	if !ok {
		t.Fatalf("expected render state")
	}

	p1 := findPlayer(rendered.Players, "p1")
	p2 := findPlayer(rendered.Players, "p2")
	assertNear(t, p1.Position.X, 5.0, 0.01)
	assertNear(t, p2.Position.X, 15.0, 0.01)
}

func TestRenderStateUsesInterpolationBuffer(t *testing.T) {
	engine := NewEngine("p1", Config{
		InterpolationBuffer: 100 * time.Millisecond,
		CorrectionBlend:     0,
		SnapThresholdTiles:  0.2,
		HistoryLimit:        16,
	})

	t0 := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	engine.SeedAuthoritativeState(model.GameState{
		TickID: 20,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}},
		},
	}, t0)
	engine.SeedAuthoritativeState(model.GameState{
		TickID: 21,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 20, Y: 0}},
		},
	}, t0.Add(200*time.Millisecond))

	// now=200ms with 100ms interpolation buffer => target=100ms => halfway.
	rendered, ok := engine.RenderState(t0.Add(200 * time.Millisecond))
	if !ok {
		t.Fatalf("expected render state")
	}
	p1 := findPlayer(rendered.Players, "p1")
	assertNear(t, p1.Position.X, 10.0, 0.01)
}

func TestReconciliationDropsAckedInputsAndSnapsLargeError(t *testing.T) {
	engine := NewEngine("p1", Config{
		InterpolationBuffer: 0,
		CorrectionBlend:     100 * time.Millisecond,
		SnapThresholdTiles:  0.2,
		HistoryLimit:        16,
	})

	t0 := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	engine.SeedAuthoritativeState(model.GameState{
		TickID: 30,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}},
		},
	}, t0)

	movePayload, err := json.Marshal(model.MovementInputPayload{
		MoveX: 1,
		MoveY: 0,
	})
	if err != nil {
		t.Fatalf("marshal move payload failed: %v", err)
	}

	engine.RecordLocalCommands([]model.InputCommand{
		{PlayerID: "p1", ClientSeq: 1, TargetTick: 31, Type: model.CmdMoveIntent, Payload: movePayload},
		{PlayerID: "p1", ClientSeq: 2, TargetTick: 32, Type: model.CmdMoveIntent, Payload: movePayload},
	})

	firstRender, ok := engine.RenderState(t0)
	if !ok {
		t.Fatalf("expected first render state")
	}
	assertNear(t, findPlayer(firstRender.Players, "p1").Position.X, 2*physics.BaseMoveStepPerTick, 0.01)

	auth := model.GameState{
		TickID: 31,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0.6, Y: 0}},
		},
	}
	engine.AcceptAuthoritativeSnapshot(model.Snapshot{
		Kind: model.SnapshotKindDelta,
		PlayerAcks: []model.PlayerAck{
			{PlayerID: "p1", LastProcessedClientSeq: 2},
		},
	}, auth, t0.Add(40*time.Millisecond))

	if pending := engine.PendingCommands(); len(pending) != 0 {
		t.Fatalf("expected acked pending inputs to be dropped, got %+v", pending)
	}

	// Large correction error (2.0 -> 0.6) exceeds threshold => immediate snap.
	rendered, ok := engine.RenderState(t0.Add(50 * time.Millisecond))
	if !ok {
		t.Fatalf("expected render state after authoritative snapshot")
	}
	assertNear(t, findPlayer(rendered.Players, "p1").Position.X, 0.6, 0.01)
}

func TestReconciliationBlendsSmallErrorOverCorrectionWindow(t *testing.T) {
	engine := NewEngine("p1", Config{
		InterpolationBuffer: 0,
		CorrectionBlend:     100 * time.Millisecond,
		SnapThresholdTiles:  2.0,
		HistoryLimit:        16,
	})

	t0 := time.Date(2026, 2, 23, 12, 0, 0, 0, time.UTC)
	engine.SeedAuthoritativeState(model.GameState{
		TickID: 40,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0, Y: 0}},
		},
	}, t0)

	movePayload, _ := json.Marshal(model.MovementInputPayload{
		MoveX: 1,
		MoveY: 0,
	})
	engine.RecordLocalCommands([]model.InputCommand{
		{PlayerID: "p1", ClientSeq: 1, TargetTick: 41, Type: model.CmdMoveIntent, Payload: movePayload},
	})

	rendered0, _ := engine.RenderState(t0)
	assertNear(t, findPlayer(rendered0.Players, "p1").Position.X, physics.BaseMoveStepPerTick, 0.01)

	engine.AcceptAuthoritativeSnapshot(model.Snapshot{
		Kind: model.SnapshotKindDelta,
		PlayerAcks: []model.PlayerAck{
			{PlayerID: "p1", LastProcessedClientSeq: 1},
		},
	}, model.GameState{
		TickID: 41,
		Players: []model.PlayerState{
			{ID: "p1", Position: model.Vector2{X: 0.5, Y: 0}},
		},
	}, t0.Add(20*time.Millisecond))

	rendered1, _ := engine.RenderState(t0.Add(60 * time.Millisecond))
	// dt=60ms over 100ms blend => from base step toward 0.5 by 60%.
	expectedBlend := physics.BaseMoveStepPerTick + ((0.5 - physics.BaseMoveStepPerTick) * 0.6)
	assertNear(t, findPlayer(rendered1.Players, "p1").Position.X, expectedBlend, 0.03)

	rendered2, _ := engine.RenderState(t0.Add(160 * time.Millisecond))
	// after additional 100ms => full settle to authoritative.
	assertNear(t, findPlayer(rendered2.Players, "p1").Position.X, 0.5, 0.02)
}

func TestRenderStateWithoutFramesReturnsFalse(t *testing.T) {
	engine := NewEngine("p1", DefaultConfig())
	if _, ok := engine.RenderState(time.Now()); ok {
		t.Fatalf("expected render state unavailable without any authoritative frames")
	}
}

func TestRecordLocalCommandsCapsPendingQueueToConfiguredLimit(t *testing.T) {
	engine := NewEngine("p1", Config{
		InterpolationBuffer: 0,
		CorrectionBlend:     0,
		SnapThresholdTiles:  0.2,
		HistoryLimit:        16,
		PendingLimit:        3,
	})

	movePayload, err := json.Marshal(model.MovementInputPayload{
		MoveX: 1,
		MoveY: 0,
	})
	if err != nil {
		t.Fatalf("marshal move payload failed: %v", err)
	}

	engine.RecordLocalCommands([]model.InputCommand{
		{PlayerID: "p1", ClientSeq: 1, TargetTick: 11, Type: model.CmdMoveIntent, Payload: movePayload},
		{PlayerID: "p1", ClientSeq: 2, TargetTick: 12, Type: model.CmdMoveIntent, Payload: movePayload},
		{PlayerID: "p1", ClientSeq: 3, TargetTick: 13, Type: model.CmdMoveIntent, Payload: movePayload},
		{PlayerID: "p1", ClientSeq: 4, TargetTick: 14, Type: model.CmdMoveIntent, Payload: movePayload},
		{PlayerID: "p1", ClientSeq: 5, TargetTick: 15, Type: model.CmdMoveIntent, Payload: movePayload},
	})

	pending := engine.PendingCommands()
	if len(pending) != 3 {
		t.Fatalf("expected pending queue to be capped at 3, got %d", len(pending))
	}
	if pending[0].ClientSeq != 3 || pending[1].ClientSeq != 4 || pending[2].ClientSeq != 5 {
		t.Fatalf("expected queue to retain newest commands [3,4,5], got %+v", pending)
	}
}

func findPlayer(players []model.PlayerState, playerID model.PlayerID) model.PlayerState {
	for _, player := range players {
		if player.ID == playerID {
			return player
		}
	}
	return model.PlayerState{}
}

func assertNear(t *testing.T, got float32, want float32, tolerance float64) {
	t.Helper()
	if math.Abs(float64(got-want)) > tolerance {
		t.Fatalf("value out of tolerance: got=%f want=%f tol=%f", got, want, tolerance)
	}
}
