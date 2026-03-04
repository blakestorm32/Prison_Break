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
	cellBlock, exists := layout.Room(gamemap.RoomCellBlockA)
	if !exists {
		t.Fatalf("expected cell block room in default layout")
	}
	wallApproach := edgeApproachPositionForStep(
		gamemap.Point{X: cellBlock.Min.X, Y: cellBlock.Min.Y + 1},
		-1,
		0,
		BaseMoveStepPerTick,
	)

	player := model.PlayerState{
		ID:       "p1",
		Position: wallApproach,
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

	player.Position = edgeApproachPositionForStep(gamemap.Point{X: 1, Y: 1}, -1, 0, BaseMoveStepPerTick)
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

	playerPosition, moveX, moveY := approachDoorForTest(t, layout, 7, gamemap.RoomCorridorMain)
	player := model.PlayerState{
		ID:            "p1",
		CurrentRoomID: gamemap.RoomCorridorMain,
		Position:      playerPosition,
	}
	occupied := BuildOccupiedTiles([]model.PlayerState{player})

	setDoorOpen(&mapState, 7, false)
	closedResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if closedResult.Moved {
		t.Fatalf("expected closed door to block movement")
	}
	if closedResult.BlockedByDoorID != 7 {
		t.Fatalf("expected door id 7 block, got %#v", closedResult)
	}

	setDoorOpen(&mapState, 7, true)
	occupied = BuildOccupiedTiles([]model.PlayerState{player})
	openResult := ResolveMoveIntent(
		player,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
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

func TestResolveMoveIntentBlocksRestrictedRoomEntryByRole(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	setDoorOpen(&mapState, 1, true)
	playerPosition, moveX, moveY := approachDoorForTest(t, layout, 1, gamemap.RoomCorridorMain)

	prisoner := model.PlayerState{
		ID:            "prisoner",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Position:      playerPosition,
	}

	occupied := BuildOccupiedTiles([]model.PlayerState{prisoner})
	blocked := ResolveMoveIntent(
		prisoner,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if blocked.Moved {
		t.Fatalf("expected prisoner blocked from entering warden_hq")
	}
	if !blocked.BlockedByMap {
		t.Fatalf("expected access-denied move to mark blocked-by-map")
	}

	warden := model.PlayerState{
		ID:            "warden",
		Role:          model.RoleWarden,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Position:      playerPosition,
	}
	occupied = BuildOccupiedTiles([]model.PlayerState{warden})
	allowed := ResolveMoveIntent(
		warden,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if !allowed.Moved {
		t.Fatalf("expected warden to enter warden_hq")
	}
}

func TestResolveMoveIntentAllowsExitFromCameraRoomWhenPowerOff(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	mapState.PowerOn = false
	setDoorOpen(&mapState, 2, true)
	playerPosition, moveX, moveY := approachDoorForTest(t, layout, 2, gamemap.RoomCameraRoom)

	deputy := model.PlayerState{
		ID:            "deputy",
		Role:          model.RoleDeputy,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCameraRoom,
		Position:      playerPosition,
	}

	occupied := BuildOccupiedTiles([]model.PlayerState{deputy})
	result := ResolveMoveIntent(
		deputy,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if !result.Moved {
		t.Fatalf("expected movement out of camera room toward corridor to remain allowed when power off")
	}
}

func TestResolveMoveIntentRespectsBlackMarketPrisonerOnlyRule(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()
	setDoorOpen(&mapState, 9, true)
	playerPosition, moveX, moveY := approachDoorForTest(t, layout, 9, gamemap.RoomCorridorMain)

	authority := model.PlayerState{
		ID:            "deputy",
		Role:          model.RoleDeputy,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Position:      playerPosition,
	}
	occupied := BuildOccupiedTiles([]model.PlayerState{authority})
	authorityResult := ResolveMoveIntent(
		authority,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if authorityResult.Moved {
		t.Fatalf("expected authority blocked from entering black market")
	}

	prisoner := model.PlayerState{
		ID:            "gang",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Position:      playerPosition,
	}
	occupied = BuildOccupiedTiles([]model.PlayerState{prisoner})
	prisonerResult := ResolveMoveIntent(
		prisoner,
		model.MovementInputPayload{MoveX: moveX, MoveY: moveY},
		layout,
		mapState,
		occupied,
		10,
	)
	if !prisonerResult.Moved {
		t.Fatalf("expected prisoner allowed to enter black market")
	}
}

func TestResolveMoveIntentBlocksOnOccupiedTile(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	mover := model.PlayerState{
		ID:       "p1",
		Position: edgeApproachPositionForStep(gamemap.Point{X: 3, Y: 14}, 1, 0, BaseMoveStepPerTick),
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

func approachDoorForTest(
	t *testing.T,
	layout gamemap.Layout,
	doorID model.DoorID,
	fromRoomID model.RoomID,
) (model.Vector2, float32, float32) {
	t.Helper()

	var link gamemap.DoorLink
	found := false
	for _, candidate := range layout.DoorLinks() {
		if candidate.ID == doorID {
			link = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("door %d not found in layout", doorID)
	}

	directions := []gamemap.Point{
		{X: 0, Y: -1},
		{X: 0, Y: 1},
		{X: -1, Y: 0},
		{X: 1, Y: 0},
	}
	doorRoomID, doorRoomExists := layout.RoomAt(link.Position)
	if doorRoomExists && doorRoomID == fromRoomID {
		for _, direction := range directions {
			neighbor := gamemap.Point{
				X: link.Position.X + direction.X,
				Y: link.Position.Y + direction.Y,
			}
			roomID, exists := layout.RoomAt(neighbor)
			if !exists || roomID == "" || roomID == fromRoomID {
				continue
			}
			moveX := float32(neighbor.X - link.Position.X)
			moveY := float32(neighbor.Y - link.Position.Y)
			position := edgeApproachPositionForStep(link.Position, moveX, moveY, BaseMoveStepPerTick)
			return position, moveX, moveY
		}
	}

	for _, direction := range directions {
		neighbor := gamemap.Point{
			X: link.Position.X + direction.X,
			Y: link.Position.Y + direction.Y,
		}
		roomID, exists := layout.RoomAt(neighbor)
		if !exists || roomID != fromRoomID {
			continue
		}
		moveX := float32(link.Position.X - neighbor.X)
		moveY := float32(link.Position.Y - neighbor.Y)
		position := edgeApproachPositionForStep(neighbor, moveX, moveY, BaseMoveStepPerTick)
		return position, moveX, moveY
	}

	t.Fatalf("door %d has no adjacent tile in room %s", doorID, fromRoomID)
	return model.Vector2{}, 0, 0
}

func edgeApproachPositionForStep(
	fromTile gamemap.Point,
	moveX float32,
	moveY float32,
	step float32,
) model.Vector2 {
	shift := float32(0.01)
	if step > 0 && step < 0.49 {
		shift = 0.5 - step + 0.01
	}

	x := float32(fromTile.X)
	y := float32(fromTile.Y)
	if moveX > 0 {
		x += shift
	} else if moveX < 0 {
		x -= shift
	}
	if moveY > 0 {
		y += shift
	} else if moveY < 0 {
		y -= shift
	}
	return model.Vector2{X: x, Y: y}
}
