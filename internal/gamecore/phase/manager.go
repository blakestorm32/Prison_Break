package phase

import (
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type Config struct {
	DayDurationTicks   uint64
	NightDurationTicks uint64
	MaxCycles          uint8
}

type Transition struct {
	From      model.PhaseType `json:"from"`
	To        model.PhaseType `json:"to"`
	StartTick uint64          `json:"start_tick"`
	EndTick   uint64          `json:"end_tick"`
	Cycle     uint8           `json:"cycle"`
}

type Hooks interface {
	OnDayStart(state *model.GameState, transition Transition)
	OnNightStart(state *model.GameState, transition Transition)
}

type HookFuncs struct {
	DayStart   func(state *model.GameState, transition Transition)
	NightStart func(state *model.GameState, transition Transition)
}

func (h HookFuncs) OnDayStart(state *model.GameState, transition Transition) {
	if h.DayStart != nil {
		h.DayStart(state, transition)
	}
}

func (h HookFuncs) OnNightStart(state *model.GameState, transition Transition) {
	if h.NightStart != nil {
		h.NightStart(state, transition)
	}
}

func DefaultConfig(tickRateHz uint32) Config {
	if tickRateHz == 0 {
		tickRateHz = constants.ServerTickRateHz
	}

	return Config{
		DayDurationTicks:   uint64(constants.DayPhaseDurationSeconds) * uint64(tickRateHz),
		NightDurationTicks: uint64(constants.NightPhaseDurationSeconds) * uint64(tickRateHz),
		MaxCycles:          constants.MaxDayNightCycles,
	}
}

func (c Config) normalized() Config {
	if c.DayDurationTicks == 0 {
		c.DayDurationTicks = 1
	}
	if c.NightDurationTicks == 0 {
		c.NightDurationTicks = 1
	}
	if c.MaxCycles == 0 {
		c.MaxCycles = constants.MaxDayNightCycles
	}
	return c
}

func InitialPhaseState(config Config, startTick uint64) model.PhaseState {
	normalized := config.normalized()
	if startTick == 0 {
		startTick = 1
	}

	return model.PhaseState{
		Current:     model.PhaseDay,
		StartedTick: startTick,
		EndsTick:    startTick + normalized.DayDurationTicks,
	}
}

func Advance(state *model.GameState, currentTick uint64, config Config, hooks Hooks) []Transition {
	if state == nil {
		return nil
	}

	normalized := config.normalized()

	transitions := make([]Transition, 0, 2)
	for currentTick >= state.Phase.EndsTick {
		nextPhase := model.PhaseNight
		duration := normalized.NightDurationTicks
		if state.Phase.Current == model.PhaseNight {
			nextPhase = model.PhaseDay
			duration = normalized.DayDurationTicks
			if state.CycleCount < normalized.MaxCycles {
				state.CycleCount++
			}
		}

		transition := Transition{
			From:      state.Phase.Current,
			To:        nextPhase,
			StartTick: state.Phase.EndsTick,
			EndTick:   state.Phase.EndsTick + duration,
			Cycle:     state.CycleCount,
		}

		state.Phase = model.PhaseState{
			Current:     nextPhase,
			StartedTick: transition.StartTick,
			EndsTick:    transition.EndTick,
		}
		transitions = append(transitions, transition)

		if hooks != nil {
			switch nextPhase {
			case model.PhaseDay:
				hooks.OnDayStart(state, transition)
			case model.PhaseNight:
				hooks.OnNightStart(state, transition)
			}
		}
	}

	return transitions
}
