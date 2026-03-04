package physics

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestResolveMoveIntentMovesOnWalkableTiles(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	player := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 3, Y: 14},
	}
	occupied := BuildOccupiedTiles([]model.PlayerState{player})

	result := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 1, MoveY: 0},
		layout,
		mapState,
		occupied,
		10,
	)

	if !result.Moved {
		t.Fatalf("expected player to move on walkable tile")
	}
	if result.Position.X <= player.Position.X {
		t.Fatalf("expected x to increase, before=%f after=%f", player.Position.X, result.Position.X)
	}
	if result.BlockedByMap || result.BlockedByDoorID != 0 || result.BlockedByPlayerID != "" {
		t.Fatalf("unexpected block state: %#v", result)
	}
}

func TestResolveMoveIntentBlocksOnWallAndOutOfBounds(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	player := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 2, Y: 13},
	}

	occupied := BuildOccupiedTiles([]model.PlayerState{player})
	wallResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: -1, MoveY: 0},
		layout,
		mapState,
		occupied,
		10,
	)
	if wallResult.Moved {
		t.Fatalf("expected movement blocked by wall")
	}
	if !wallResult.BlockedByMap {
		t.Fatalf("expected wall collision to set BlockedByMap")
	}

	player.Position = model.Vector2{X: 1, Y: 1}
	occupied = BuildOccupiedTiles([]model.PlayerState{player})
	outResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: -3, MoveY: 0},
		layout,
		mapState,
		occupied,
		10,
	)
	if outResult.Moved {
		t.Fatalf("expected movement blocked by out-of-bounds")
	}
	if !outResult.BlockedByMap {
		t.Fatalf("expected out-of-bounds to set BlockedByMap")
	}
}

func TestResolveMoveIntentBlocksOnClosedDoorAndAllowsOpenDoor(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	player := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 4, Y: 10},
	}
	occupied := BuildOccupiedTiles([]model.PlayerState{player})

	setDoorOpen(&mapState, 1, false)
	closedResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 0, MoveY: -1},
		layout,
		mapState,
		occupied,
		10,
	)
	if closedResult.Moved {
		t.Fatalf("expected closed door to block movement")
	}
	if closedResult.BlockedByDoorID != 1 {
		t.Fatalf("expected door id 1 block, got %#v", closedResult)
	}

	setDoorOpen(&mapState, 1, true)
	occupied = BuildOccupiedTiles([]model.PlayerState{player})
	openResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 0, MoveY: -1},
		layout,
		mapState,
		occupied,
		10,
	)
	if !openResult.Moved {
		t.Fatalf("expected open door to allow movement")
	}
	if openResult.BlockedByDoorID != 0 {
		t.Fatalf("expected no blocking door when open, got %#v", openResult)
	}
}

func TestResolveMoveIntentBlocksOnOccupiedTile(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	mover := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 3, Y: 14},
	}
	blocker := model.PlayerState{
		ID:       "p2",
		Position: model.Vector2{X: 4, Y: 14},
	}

	occupied := BuildOccupiedTiles([]model.PlayerState{mover, blocker})
	result := ResolveMoveIntent(
		mover,
		model.MovementInputPayload{MoveX: 1, MoveY: 0},
		layout,
		mapState,
		occupied,
		10,
	)

	if result.Moved {
		t.Fatalf("expected movement blocked by occupied tile")
	}
	if result.BlockedByPlayerID != "p2" {
		t.Fatalf("expected blocker p2, got %#v", result)
	}
}

func TestResolveMoveIntentBlocksWhenStunned(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	player := model.PlayerState{
		ID:               "p1",
		Position:         model.Vector2{X: 3, Y: 14},
		StunnedUntilTick: 5,
	}

	result := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 1, MoveY: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		4,
	)
	if result.Moved {
		t.Fatalf("expected stunned player to not move")
	}
	if !result.BlockedByStun {
		t.Fatalf("expected blocked-by-stun result")
	}
}

func TestResolveMoveIntentBlocksWhenSolitaryPenaltyActive(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	player := model.PlayerState{
		ID:                "p1",
		Position:          model.Vector2{X: 3, Y: 14},
		SolitaryUntilTick: 8,
	}

	result := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 1, MoveY: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		5,
	)
	if result.Moved {
		t.Fatalf("expected solitary-penalized player to not move")
	}
	if !result.BlockedByStun {
		t.Fatalf("expected solitary lock to use blocked-by-stun motion gate")
	}
}

func TestApplyKnockbackMovesAndExtendsStun(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	player := model.PlayerState{
		ID:               "p1",
		Position:         model.Vector2{X: 3, Y: 14},
		StunnedUntilTick: 4,
	}

	next, result := ApplyKnockback(
		player,
		model.Vector2{X: 1, Y: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		10,
		3,
	)

	if !result.Moved {
		t.Fatalf("expected knockback to move player")
	}
	if next.Position.X <= player.Position.X {
		t.Fatalf("expected knockback to push player on x axis")
	}
	if next.StunnedUntilTick != 13 {
		t.Fatalf("expected stun extension to tick 13, got %d", next.StunnedUntilTick)
	}
}

func TestResolveMoveIntentClampsExtremeInputMagnitude(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	player := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 3, Y: 14},
	}

	result := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 50, MoveY: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		1,
	)
	if result.Velocity.X > BaseMoveStepPerTick+0.001 {
		t.Fatalf("expected clamped velocity <= %f, got %f", BaseMoveStepPerTick, result.Velocity.X)
	}
}

func TestResolveMoveIntentUsesSpeedBoostEffect(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	player := model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 3, Y: 14},
		Effects: []model.EffectState{
			{
				Effect:   model.EffectSpeedBoost,
				EndsTick: 10,
			},
		},
	}

	result := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 1.35, MoveY: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		5,
	)
	if result.Velocity.X < SprintMoveStepPerTick-0.001 {
		t.Fatalf("expected speed-boost move velocity to reach sprint step, got %f", result.Velocity.X)
	}

	player.Effects[0].EndsTick = 2
	result = ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: 1.35, MoveY: 0},
		layout,
		mapState,
		BuildOccupiedTiles([]model.PlayerState{player}),
		5,
	)
	if result.Velocity.X > BaseMoveStepPerTick+0.001 {
		t.Fatalf("expected expired speed boost to fall back to base step, got %f", result.Velocity.X)
	}
}

func setDoorOpen(mapState *model.MapState, doorID model.DoorID, open bool) {
	for idx := range mapState.Doors {
		if mapState.Doors[idx].ID == doorID {
			mapState.Doors[idx].Open = open
			return
		}
	}
}
