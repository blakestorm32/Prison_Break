package prediction

import (
	"encoding/json"
	"math"
	"sort"
	"sync"
	"time"

	"prison-break/internal/engine/physics"
	"prison-break/internal/gamecore/determinism"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type Config struct {
	InterpolationBuffer time.Duration
	CorrectionBlend     time.Duration
	SnapThresholdTiles  float32
	HistoryLimit        int
	PendingLimit        int
}

func DefaultConfig() Config {
	return Config{
		InterpolationBuffer: time.Duration(constants.DefaultInterpolationBufferMS) * time.Millisecond,
		CorrectionBlend:     time.Duration(constants.DefaultCorrectionBlendMS) * time.Millisecond,
		SnapThresholdTiles:  constants.PositionSnapThresholdTiles,
		HistoryLimit:        64,
		PendingLimit:        256,
	}
}

func (c Config) normalized() Config {
	if c.InterpolationBuffer < 0 {
		c.InterpolationBuffer = 0
	}
	if c.CorrectionBlend < 0 {
		c.CorrectionBlend = 0
	}
	if c.SnapThresholdTiles <= 0 {
		c.SnapThresholdTiles = constants.PositionSnapThresholdTiles
	}
	if c.HistoryLimit <= 1 {
		c.HistoryLimit = 64
	}
	if c.PendingLimit <= 0 {
		c.PendingLimit = 256
	}
	return c
}

type Engine struct {
	mu sync.RWMutex

	localPlayerID model.PlayerID
	config        Config

	history []timedState
	pending []model.InputCommand

	hasSmoothedLocal bool
	smoothedLocal    model.Vector2
	lastRenderAt     time.Time
}

type timedState struct {
	receivedAt time.Time
	state      model.GameState
}

func NewEngine(localPlayerID model.PlayerID, config Config) *Engine {
	return &Engine{
		localPlayerID: localPlayerID,
		config:        config.normalized(),
		history:       make([]timedState, 0, 64),
		pending:       make([]model.InputCommand, 0, 64),
	}
}

func (e *Engine) SeedAuthoritativeState(state model.GameState, receivedAt time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.insertFrameLocked(state, receivedAt)
}

func (e *Engine) AcceptAuthoritativeSnapshot(
	snapshot model.Snapshot,
	state model.GameState,
	receivedAt time.Time,
) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.insertFrameLocked(state, receivedAt)

	ackSeq := ackedClientSeq(snapshot.PlayerAcks, e.localPlayerID)
	if ackSeq != 0 {
		e.pending = determinism.DropAckedInputs(e.pending, ackSeq)
	}
	e.trimPendingLocked()
}

func (e *Engine) RecordLocalCommands(commands []model.InputCommand) {
	if len(commands) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for _, command := range commands {
		if command.PlayerID != e.localPlayerID || command.ClientSeq == 0 {
			continue
		}
		if command.Type != model.CmdMoveIntent && command.Type != model.CmdAimIntent {
			continue
		}
		if hasPendingClientSeq(e.pending, command.ClientSeq) {
			continue
		}

		cloned := command
		if len(command.Payload) > 0 {
			cloned.Payload = append([]byte(nil), command.Payload...)
		}
		e.pending = append(e.pending, cloned)
	}

	e.trimPendingLocked()
}

func (e *Engine) PendingCommands() []model.InputCommand {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]model.InputCommand, len(e.pending))
	copy(out, e.pending)
	return out
}

func (e *Engine) RenderState(now time.Time) (model.GameState, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.history) == 0 {
		return model.GameState{}, false
	}

	base, ok := e.interpolateLocked(now)
	if !ok {
		return model.GameState{}, false
	}

	localIndex := findPlayerIndex(base.Players, e.localPlayerID)
	if localIndex < 0 {
		e.hasSmoothedLocal = false
		e.lastRenderAt = now
		return base, true
	}

	desired := replayPendingMovement(base.Players[localIndex], base.TickID, e.pending)

	if !e.hasSmoothedLocal {
		e.smoothedLocal = desired
		e.hasSmoothedLocal = true
		e.lastRenderAt = now
		base.Players[localIndex].Position = desired
		return base, true
	}

	errorDistance := vectorDistance(e.smoothedLocal, desired)
	if errorDistance > e.config.SnapThresholdTiles {
		e.smoothedLocal = desired
		e.lastRenderAt = now
		base.Players[localIndex].Position = desired
		return base, true
	}

	blendFactor := float32(1.0)
	if e.config.CorrectionBlend > 0 && !e.lastRenderAt.IsZero() && now.After(e.lastRenderAt) {
		dt := now.Sub(e.lastRenderAt)
		blendFactor = float32(dt.Seconds() / e.config.CorrectionBlend.Seconds())
		if blendFactor > 1 {
			blendFactor = 1
		}
		if blendFactor < 0 {
			blendFactor = 0
		}
	}

	e.smoothedLocal = lerpVector(e.smoothedLocal, desired, blendFactor)
	e.lastRenderAt = now
	base.Players[localIndex].Position = e.smoothedLocal

	return base, true
}

func (e *Engine) insertFrameLocked(state model.GameState, receivedAt time.Time) {
	if receivedAt.IsZero() {
		receivedAt = time.Now().UTC()
	}

	frame := timedState{
		receivedAt: receivedAt.UTC(),
		state:      cloneGameState(state),
	}

	e.history = append(e.history, frame)
	sort.Slice(e.history, func(i int, j int) bool {
		return e.history[i].receivedAt.Before(e.history[j].receivedAt)
	})

	if len(e.history) > e.config.HistoryLimit {
		cut := len(e.history) - e.config.HistoryLimit
		e.history = append([]timedState(nil), e.history[cut:]...)
	}
}

func (e *Engine) interpolateLocked(now time.Time) (model.GameState, bool) {
	if len(e.history) == 0 {
		return model.GameState{}, false
	}
	if len(e.history) == 1 {
		return cloneGameState(e.history[0].state), true
	}

	target := now.UTC().Add(-e.config.InterpolationBuffer)

	first := e.history[0]
	last := e.history[len(e.history)-1]
	if target.Before(first.receivedAt) || target.Equal(first.receivedAt) {
		return cloneGameState(first.state), true
	}
	if target.After(last.receivedAt) || target.Equal(last.receivedAt) {
		return cloneGameState(last.state), true
	}

	for index := 1; index < len(e.history); index++ {
		prev := e.history[index-1]
		next := e.history[index]
		if target.Before(prev.receivedAt) || target.After(next.receivedAt) {
			continue
		}

		window := next.receivedAt.Sub(prev.receivedAt)
		if window <= 0 {
			return cloneGameState(next.state), true
		}

		alpha := float32(target.Sub(prev.receivedAt).Seconds() / window.Seconds())
		if alpha < 0 {
			alpha = 0
		}
		if alpha > 1 {
			alpha = 1
		}

		return interpolateState(prev.state, next.state, alpha), true
	}

	return cloneGameState(last.state), true
}

func ackedClientSeq(acks []model.PlayerAck, playerID model.PlayerID) uint64 {
	for _, ack := range acks {
		if ack.PlayerID == playerID {
			return ack.LastProcessedClientSeq
		}
	}
	return 0
}

func hasPendingClientSeq(pending []model.InputCommand, clientSeq uint64) bool {
	for _, command := range pending {
		if command.ClientSeq == clientSeq {
			return true
		}
	}
	return false
}

func (e *Engine) trimPendingLocked() {
	if e.config.PendingLimit <= 0 || len(e.pending) <= e.config.PendingLimit {
		return
	}

	cut := len(e.pending) - e.config.PendingLimit
	e.pending = append([]model.InputCommand(nil), e.pending[cut:]...)
}

func replayPendingMovement(base model.PlayerState, authoritativeTick uint64, pending []model.InputCommand) model.Vector2 {
	position := base.Position

	candidate := make([]model.InputCommand, 0, len(pending))
	for _, command := range pending {
		if command.Type != model.CmdMoveIntent {
			continue
		}
		if command.TargetTick != 0 && command.TargetTick <= authoritativeTick {
			continue
		}
		candidate = append(candidate, command)
	}
	sort.Slice(candidate, func(i int, j int) bool {
		return candidate[i].ClientSeq < candidate[j].ClientSeq
	})

	for _, command := range candidate {
		var payload model.MovementInputPayload
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			continue
		}
		step := physics.BaseMoveStepPerTick
		if payload.Sprint && physics.SprintMoveStepPerTick > step {
			step = physics.SprintMoveStepPerTick
		}
		move := clampMove(payload.MoveX, payload.MoveY, step)
		position.X += move.X
		position.Y += move.Y
	}

	return position
}

func clampMove(moveX float32, moveY float32, maxStep float32) model.Vector2 {
	if maxStep <= 0 {
		return model.Vector2{}
	}

	squared := moveX*moveX + moveY*moveY
	if squared == 0 {
		return model.Vector2{}
	}
	magnitude := float32(math.Sqrt(float64(squared)))
	if magnitude <= maxStep {
		return model.Vector2{X: moveX, Y: moveY}
	}

	scale := maxStep / magnitude
	return model.Vector2{
		X: moveX * scale,
		Y: moveY * scale,
	}
}

func interpolateState(from model.GameState, to model.GameState, alpha float32) model.GameState {
	out := cloneGameState(to)

	playersByID := make(map[model.PlayerID]model.PlayerState, len(from.Players))
	for _, player := range from.Players {
		playersByID[player.ID] = player
	}

	for index := range out.Players {
		previous, exists := playersByID[out.Players[index].ID]
		if !exists {
			continue
		}
		out.Players[index].Position = lerpVector(previous.Position, out.Players[index].Position, alpha)
		out.Players[index].Velocity = lerpVector(previous.Velocity, out.Players[index].Velocity, alpha)
	}

	entitiesByID := make(map[model.EntityID]model.EntityState, len(from.Entities))
	for _, entity := range from.Entities {
		entitiesByID[entity.ID] = entity
	}

	for index := range out.Entities {
		previous, exists := entitiesByID[out.Entities[index].ID]
		if !exists {
			continue
		}
		out.Entities[index].Position = lerpVector(previous.Position, out.Entities[index].Position, alpha)
		out.Entities[index].Velocity = lerpVector(previous.Velocity, out.Entities[index].Velocity, alpha)
	}

	return out
}

func lerpVector(from model.Vector2, to model.Vector2, alpha float32) model.Vector2 {
	return model.Vector2{
		X: from.X + ((to.X - from.X) * alpha),
		Y: from.Y + ((to.Y - from.Y) * alpha),
	}
}

func vectorDistance(from model.Vector2, to model.Vector2) float32 {
	dx := to.X - from.X
	dy := to.Y - from.Y
	return float32(math.Sqrt(float64((dx * dx) + (dy * dy))))
}

func findPlayerIndex(players []model.PlayerState, playerID model.PlayerID) int {
	for index := range players {
		if players[index].ID == playerID {
			return index
		}
	}
	return -1
}

func cloneGameState(in model.GameState) model.GameState {
	out := in
	out.Players = make([]model.PlayerState, len(in.Players))
	for index := range in.Players {
		out.Players[index] = clonePlayerState(in.Players[index])
	}

	out.Entities = make([]model.EntityState, len(in.Entities))
	for index := range in.Entities {
		entity := in.Entities[index]
		entity.Tags = append([]string(nil), in.Entities[index].Tags...)
		out.Entities[index] = entity
	}

	out.Map.Doors = append([]model.DoorState(nil), in.Map.Doors...)
	out.Map.Cells = make([]model.CellState, len(in.Map.Cells))
	for index := range in.Map.Cells {
		out.Map.Cells[index] = in.Map.Cells[index]
		out.Map.Cells[index].OccupantPlayerIDs = append([]model.PlayerID(nil), in.Map.Cells[index].OccupantPlayerIDs...)
	}
	out.Map.RestrictedZones = append([]model.ZoneState(nil), in.Map.RestrictedZones...)

	if in.GameOver != nil {
		gameOver := *in.GameOver
		gameOver.WinnerPlayerIDs = append([]model.PlayerID(nil), in.GameOver.WinnerPlayerIDs...)
		out.GameOver = &gameOver
	}

	return out
}

func clonePlayerState(in model.PlayerState) model.PlayerState {
	out := in
	out.Inventory = append([]model.ItemStack(nil), in.Inventory...)
	out.Cards = append([]model.CardType(nil), in.Cards...)
	out.Effects = append([]model.EffectState(nil), in.Effects...)
	return out
}
