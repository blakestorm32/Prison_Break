package determinism

import (
	"bytes"
	"sort"

	"prison-break/internal/shared/model"
)

type SimulationConfig struct {
	MatchSeed uint64
	StartTick uint64
	EndTick   uint64
}

type StepFunc func(state *model.GameState, tickID uint64, commands []model.InputCommand, rng *RNGStreams)

func Run(config SimulationConfig, initial model.GameState, commands []model.InputCommand, step StepFunc) model.GameState {
	startTick := config.StartTick
	if startTick == 0 {
		startTick = 1
	}
	if config.EndTick < startTick {
		return cloneGameState(initial)
	}

	state := cloneGameState(initial)
	rng := NewRNGStreams(config.MatchSeed)
	normalized := normalizeInputLog(commands)
	scheduled := scheduleByTick(normalized, startTick, config.EndTick)

	for tick := startTick; tick <= config.EndTick; tick++ {
		commandsForTick := scheduled[tick]
		sortInputCommands(commandsForTick)

		if step != nil {
			step(&state, tick, commandsForTick, rng)
		}

		state.TickID = tick
	}

	return state
}

type inputKey struct {
	playerID  model.PlayerID
	clientSeq uint64
}

func normalizeInputLog(commands []model.InputCommand) []model.InputCommand {
	if len(commands) == 0 {
		return nil
	}

	ordered := make([]model.InputCommand, len(commands))
	copy(ordered, commands)
	sortInputCommands(ordered)

	deduped := make([]model.InputCommand, 0, len(ordered))
	seen := make(map[inputKey]struct{}, len(ordered))
	for _, cmd := range ordered {
		key := inputKey{
			playerID:  cmd.PlayerID,
			clientSeq: cmd.ClientSeq,
		}
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		deduped = append(deduped, cmd)
	}

	return deduped
}

func scheduleByTick(commands []model.InputCommand, startTick uint64, endTick uint64) map[uint64][]model.InputCommand {
	byTick := make(map[uint64][]model.InputCommand)
	for _, cmd := range commands {
		targetTick := cmd.TargetTick
		if targetTick == 0 {
			targetTick = startTick
		}
		if targetTick < startTick {
			targetTick = startTick
		}
		if targetTick > endTick {
			continue
		}

		cmd.TargetTick = targetTick
		byTick[targetTick] = append(byTick[targetTick], cmd)
	}

	return byTick
}

func sortInputCommands(commands []model.InputCommand) {
	sort.SliceStable(commands, func(i int, j int) bool {
		left := commands[i]
		right := commands[j]

		if left.IngressSeq != right.IngressSeq {
			return left.IngressSeq < right.IngressSeq
		}
		if left.PlayerID != right.PlayerID {
			return left.PlayerID < right.PlayerID
		}
		if left.ClientSeq != right.ClientSeq {
			return left.ClientSeq < right.ClientSeq
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}

		return bytes.Compare(left.Payload, right.Payload) < 0
	})
}

func cloneGameState(in model.GameState) model.GameState {
	out := in

	out.Players = make([]model.PlayerState, len(in.Players))
	copy(out.Players, in.Players)
	for i := range out.Players {
		out.Players[i].Inventory = append([]model.ItemStack(nil), in.Players[i].Inventory...)
		out.Players[i].Cards = append([]model.CardType(nil), in.Players[i].Cards...)
		out.Players[i].Effects = append([]model.EffectState(nil), in.Players[i].Effects...)
	}

	out.Entities = make([]model.EntityState, len(in.Entities))
	copy(out.Entities, in.Entities)
	for i := range out.Entities {
		out.Entities[i].Tags = append([]string(nil), in.Entities[i].Tags...)
	}

	out.Map.Doors = append([]model.DoorState(nil), in.Map.Doors...)
	out.Map.Cells = append([]model.CellState(nil), in.Map.Cells...)
	for i := range out.Map.Cells {
		out.Map.Cells[i].OccupantPlayerIDs = append([]model.PlayerID(nil), in.Map.Cells[i].OccupantPlayerIDs...)
	}
	out.Map.RestrictedZones = append([]model.ZoneState(nil), in.Map.RestrictedZones...)

	if in.GameOver != nil {
		gameOverCopy := *in.GameOver
		gameOverCopy.WinnerPlayerIDs = append([]model.PlayerID(nil), in.GameOver.WinnerPlayerIDs...)
		out.GameOver = &gameOverCopy
	}

	return out
}
