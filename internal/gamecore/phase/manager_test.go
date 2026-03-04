package phase

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestInitialPhaseStateUsesDayWindowAndStartTick(t *testing.T) {
	config := Config{
		DayDurationTicks:   5,
		NightDurationTicks: 2,
		MaxCycles:          6,
	}

	phase := InitialPhaseState(config, 1)
	if phase.Current != model.PhaseDay {
		t.Fatalf("expected initial phase day, got %s", phase.Current)
	}
	if phase.StartedTick != 1 {
		t.Fatalf("expected start tick 1, got %d", phase.StartedTick)
	}
	if phase.EndsTick != 6 {
		t.Fatalf("expected end tick 6 for day window [1,6), got %d", phase.EndsTick)
	}
}

func TestAdvanceTransitionsDayToNightAndNightToDayCycleIncrement(t *testing.T) {
	state := model.GameState{
		CycleCount: 0,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    6,
		},
	}
	config := Config{
		DayDurationTicks:   5,
		NightDurationTicks: 2,
		MaxCycles:          6,
	}

	transitions := Advance(&state, 6, config, nil)
	if len(transitions) != 1 {
		t.Fatalf("expected one transition at day boundary, got %d", len(transitions))
	}
	if state.Phase.Current != model.PhaseNight {
		t.Fatalf("expected night phase after day boundary, got %s", state.Phase.Current)
	}
	if state.CycleCount != 0 {
		t.Fatalf("expected cycle count unchanged on day->night, got %d", state.CycleCount)
	}
	if state.Phase.StartedTick != 6 || state.Phase.EndsTick != 8 {
		t.Fatalf("unexpected night window: %+v", state.Phase)
	}

	transitions = Advance(&state, 8, config, nil)
	if len(transitions) != 1 {
		t.Fatalf("expected one transition at night boundary, got %d", len(transitions))
	}
	if state.Phase.Current != model.PhaseDay {
		t.Fatalf("expected day phase after night boundary, got %s", state.Phase.Current)
	}
	if state.CycleCount != 1 {
		t.Fatalf("expected cycle increment on night->day, got %d", state.CycleCount)
	}
	if state.Phase.StartedTick != 8 || state.Phase.EndsTick != 13 {
		t.Fatalf("unexpected day window after transition: %+v", state.Phase)
	}
}

func TestAdvanceCanCatchUpMultipleTransitionsInSingleCall(t *testing.T) {
	state := model.GameState{
		CycleCount: 0,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    6,
		},
	}
	config := Config{
		DayDurationTicks:   5,
		NightDurationTicks: 2,
		MaxCycles:          6,
	}

	transitions := Advance(&state, 13, config, nil)
	if len(transitions) != 3 {
		t.Fatalf("expected three catch-up transitions, got %d", len(transitions))
	}

	expectedPhase := model.PhaseNight
	if state.Phase.Current != expectedPhase {
		t.Fatalf("expected final phase %s, got %s", expectedPhase, state.Phase.Current)
	}
	if state.CycleCount != 1 {
		t.Fatalf("expected one full cycle completed in catch-up path, got %d", state.CycleCount)
	}

	gotOrder := []model.PhaseType{
		transitions[0].To,
		transitions[1].To,
		transitions[2].To,
	}
	wantOrder := []model.PhaseType{
		model.PhaseNight,
		model.PhaseDay,
		model.PhaseNight,
	}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("unexpected transition order: got=%v want=%v", gotOrder, wantOrder)
	}
}

func TestAdvanceCycleCountSaturatesAtMaxCycles(t *testing.T) {
	state := model.GameState{
		CycleCount: 6,
		Phase: model.PhaseState{
			Current:     model.PhaseNight,
			StartedTick: 100,
			EndsTick:    102,
		},
	}
	config := Config{
		DayDurationTicks:   5,
		NightDurationTicks: 2,
		MaxCycles:          6,
	}

	_ = Advance(&state, 102, config, nil)
	if state.CycleCount != 6 {
		t.Fatalf("expected saturated cycle count to remain 6, got %d", state.CycleCount)
	}
}

func TestAdvanceInvokesPhaseHooks(t *testing.T) {
	state := model.GameState{
		CycleCount: 0,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    6,
		},
	}
	config := Config{
		DayDurationTicks:   5,
		NightDurationTicks: 2,
		MaxCycles:          6,
	}

	var dayHookCalls int
	var nightHookCalls int
	hooks := HookFuncs{
		DayStart: func(_ *model.GameState, transition Transition) {
			dayHookCalls++
			if transition.To != model.PhaseDay {
				t.Fatalf("expected day hook transition target day, got %s", transition.To)
			}
		},
		NightStart: func(_ *model.GameState, transition Transition) {
			nightHookCalls++
			if transition.To != model.PhaseNight {
				t.Fatalf("expected night hook transition target night, got %s", transition.To)
			}
		},
	}

	_ = Advance(&state, 6, config, hooks)
	_ = Advance(&state, 8, config, hooks)

	if nightHookCalls != 1 {
		t.Fatalf("expected one night hook call, got %d", nightHookCalls)
	}
	if dayHookCalls != 1 {
		t.Fatalf("expected one day hook call, got %d", dayHookCalls)
	}
}
