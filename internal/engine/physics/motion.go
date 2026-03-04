package physics

import (
	"math"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

const (
	BaseMoveStepPerTick     float32 = 1.0
	SprintMoveStepPerTick   float32 = 1.35
	MaxKnockbackStepPerTick float32 = 1.50
)

type MotionResult struct {
	Position          model.Vector2  `json:"position"`
	Velocity          model.Vector2  `json:"velocity"`
	Moved             bool           `json:"moved"`
	BlockedByStun     bool           `json:"blocked_by_stun"`
	BlockedByMap      bool           `json:"blocked_by_map"`
	BlockedByDoorID   model.DoorID   `json:"blocked_by_door_id,omitempty"`
	BlockedByPlayerID model.PlayerID `json:"blocked_by_player_id,omitempty"`
	StunnedUntilTick  uint64         `json:"stunned_until_tick,omitempty"`
}

func TileFromPosition(position model.Vector2) gamemap.Point {
	return gamemap.Point{
		X: roundedInt(position.X),
		Y: roundedInt(position.Y),
	}
}

func BuildOccupiedTiles(players []model.PlayerState) map[gamemap.Point]model.PlayerID {
	occupied := make(map[gamemap.Point]model.PlayerID, len(players))
	for _, player := range players {
		tile := TileFromPosition(player.Position)
		existing, exists := occupied[tile]
		if !exists || player.ID < existing {
			occupied[tile] = player.ID
		}
	}
	return occupied
}

func ResolveMoveIntent(
	player model.PlayerState,
	input model.MovementInputPayload,
	layout gamemap.Layout,
	mapState model.MapState,
	occupied map[gamemap.Point]model.PlayerID,
	currentTick uint64,
) MotionResult {
	if player.SolitaryUntilTick != 0 && currentTick <= player.SolitaryUntilTick {
		return MotionResult{
			Position:      player.Position,
			Velocity:      model.Vector2{},
			BlockedByStun: true,
		}
	}
	if player.StunnedUntilTick != 0 && currentTick <= player.StunnedUntilTick {
		return MotionResult{
			Position:      player.Position,
			Velocity:      model.Vector2{},
			BlockedByStun: true,
		}
	}

	maxStep := BaseMoveStepPerTick
	if hasActiveEffect(player, model.EffectSpeedBoost, currentTick) {
		maxStep = SprintMoveStepPerTick
	}
	if input.Sprint {
		if SprintMoveStepPerTick > maxStep {
			maxStep = SprintMoveStepPerTick
		}
	}
	desired := clampVectorMagnitude(model.Vector2{
		X: input.MoveX,
		Y: input.MoveY,
	}, maxStep)

	return resolveVectorMotion(player, desired, layout, mapState, occupied)
}

func ApplyKnockback(
	player model.PlayerState,
	impulse model.Vector2,
	layout gamemap.Layout,
	mapState model.MapState,
	occupied map[gamemap.Point]model.PlayerID,
	currentTick uint64,
	stunDurationTicks uint64,
) (model.PlayerState, MotionResult) {
	desired := clampVectorMagnitude(impulse, MaxKnockbackStepPerTick)
	motion := resolveVectorMotion(player, desired, layout, mapState, occupied)

	next := player
	next.Position = motion.Position
	next.Velocity = motion.Velocity

	if stunDurationTicks > 0 {
		stunnedUntil := currentTick + stunDurationTicks
		if stunnedUntil > next.StunnedUntilTick {
			next.StunnedUntilTick = stunnedUntil
		}
	}
	motion.StunnedUntilTick = next.StunnedUntilTick

	return next, motion
}

func resolveVectorMotion(
	player model.PlayerState,
	desired model.Vector2,
	layout gamemap.Layout,
	mapState model.MapState,
	occupied map[gamemap.Point]model.PlayerID,
) MotionResult {
	result := MotionResult{
		Position: player.Position,
	}
	if desired.X == 0 && desired.Y == 0 {
		return result
	}

	current := player.Position
	originalTile := TileFromPosition(current)
	if occupied != nil {
		delete(occupied, originalTile)
	}

	applied := model.Vector2{}

	if desired.X != 0 {
		candidate := current
		candidate.X += desired.X
		if canOccupyPosition(player.ID, candidate, layout, mapState, occupied, &result) {
			applied.X = desired.X
			current = candidate
		}
	}

	if desired.Y != 0 {
		candidate := current
		candidate.Y += desired.Y
		if canOccupyPosition(player.ID, candidate, layout, mapState, occupied, &result) {
			applied.Y = desired.Y
			current = candidate
		}
	}

	if occupied != nil {
		occupied[TileFromPosition(current)] = player.ID
	}

	result.Position = current
	result.Velocity = applied
	result.Moved = applied.X != 0 || applied.Y != 0
	return result
}

func canOccupyPosition(
	playerID model.PlayerID,
	position model.Vector2,
	layout gamemap.Layout,
	mapState model.MapState,
	occupied map[gamemap.Point]model.PlayerID,
	result *MotionResult,
) bool {
	tilePoint := TileFromPosition(position)
	if !layout.InBounds(tilePoint) {
		result.BlockedByMap = true
		return false
	}

	tile, exists := layout.TileAt(tilePoint)
	if !exists || !tile.Walkable() {
		result.BlockedByMap = true
		return false
	}

	if doorLink, isDoorTile := layout.DoorLinkAt(tilePoint); isDoorTile {
		if !isDoorOpen(mapState, doorLink.ID) {
			result.BlockedByDoorID = doorLink.ID
			return false
		}
	}

	if occupied != nil {
		if occupant, occupiedByPlayer := occupied[tilePoint]; occupiedByPlayer && occupant != "" && occupant != playerID {
			result.BlockedByPlayerID = occupant
			return false
		}
	}

	return true
}

func isDoorOpen(mapState model.MapState, doorID model.DoorID) bool {
	for _, door := range mapState.Doors {
		if door.ID == doorID {
			return door.Open
		}
	}
	return true
}

func clampVectorMagnitude(vector model.Vector2, maxMagnitude float32) model.Vector2 {
	if maxMagnitude <= 0 {
		return model.Vector2{}
	}

	squared := vector.X*vector.X + vector.Y*vector.Y
	if squared == 0 {
		return vector
	}

	magnitude := float32(math.Sqrt(float64(squared)))
	if magnitude <= maxMagnitude {
		return vector
	}

	scale := maxMagnitude / magnitude
	return model.Vector2{
		X: vector.X * scale,
		Y: vector.Y * scale,
	}
}

func roundedInt(value float32) int {
	return int(math.Floor(float64(value) + 0.5))
}

func hasActiveEffect(player model.PlayerState, effect model.EffectType, currentTick uint64) bool {
	for _, candidate := range player.Effects {
		if candidate.Effect != effect {
			continue
		}
		if candidate.EndsTick != 0 && currentTick > candidate.EndsTick {
			continue
		}
		return true
	}
	return false
}
