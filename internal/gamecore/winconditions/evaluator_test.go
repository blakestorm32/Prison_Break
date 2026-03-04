package winconditions

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestEvaluateTriggersGangLeaderEscapedByRoom(t *testing.T) {
	state := fixtureState()
	state.TickID = 55
	for idx := range state.Players {
		if state.Players[idx].Role == model.RoleGangLeader {
			state.Players[idx].CurrentRoomID = EscapedRoomID
		}
	}

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome == nil {
		t.Fatalf("expected outcome when gang leader escaped")
	}
	if outcome.Reason != model.WinReasonGangLeaderEscaped {
		t.Fatalf("expected gang_leader_escaped, got %s", outcome.Reason)
	}
	if outcome.EndedTick != 55 {
		t.Fatalf("expected ended tick 55, got %d", outcome.EndedTick)
	}
}

func TestEvaluateTriggersWardenDied(t *testing.T) {
	state := fixtureState()
	for idx := range state.Players {
		if state.Players[idx].Role == model.RoleWarden {
			state.Players[idx].Alive = false
		}
	}

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome == nil || outcome.Reason != model.WinReasonWardenDied {
		t.Fatalf("expected warden_died, got %+v", outcome)
	}
}

func TestEvaluateTriggersAllGangMembersDead(t *testing.T) {
	state := fixtureState()
	for idx := range state.Players {
		if state.Players[idx].Role == model.RoleGangLeader || state.Players[idx].Role == model.RoleGangMember {
			state.Players[idx].Alive = false
		}
	}

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome == nil || outcome.Reason != model.WinReasonAllGangMembersDead {
		t.Fatalf("expected all_gang_members_dead, got %+v", outcome)
	}
}

func TestEvaluateTriggersMaxCyclesReached(t *testing.T) {
	state := fixtureState()
	state.CycleCount = 6

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome == nil || outcome.Reason != model.WinReasonMaxCyclesReached {
		t.Fatalf("expected max_cycles_reached, got %+v", outcome)
	}
}

func TestEvaluateAppliesDeterministicReasonPriority(t *testing.T) {
	state := fixtureState()
	state.CycleCount = 6
	for idx := range state.Players {
		if state.Players[idx].Role == model.RoleWarden {
			state.Players[idx].Alive = false
		}
		if state.Players[idx].Role == model.RoleGangLeader {
			state.Players[idx].CurrentRoomID = EscapedRoomID
		}
	}

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome == nil {
		t.Fatalf("expected outcome")
	}
	if outcome.Reason != model.WinReasonGangLeaderEscaped {
		t.Fatalf("expected gang_leader_escaped priority, got %s", outcome.Reason)
	}
}

func TestResolveOutcomeRoleSpecificWinners(t *testing.T) {
	state := fixtureState()

	checkWinners := func(reason model.WinReason, want []model.PlayerID) {
		t.Helper()
		outcome := ResolveOutcome(state, reason, 10)
		if !reflect.DeepEqual(outcome.WinnerPlayerIDs, want) {
			t.Fatalf("unexpected winners for %s: got=%v want=%v", reason, outcome.WinnerPlayerIDs, want)
		}
	}

	checkWinners(model.WinReasonWardenDied, []model.PlayerID{"gang", "gmember"})
	checkWinners(model.WinReasonGangLeaderEscaped, []model.PlayerID{"evildep", "gang", "gmember"})
	checkWinners(model.WinReasonAllGangMembersDead, []model.PlayerID{"gooddep", "snitch", "warden"})
	checkWinners(model.WinReasonMaxCyclesReached, []model.PlayerID{"gooddep", "snitch", "warden"})
	checkWinners(model.WinReasonHitmanTargetEliminated, []model.PlayerID{"neutral"})
}

func TestEvaluateReturnsNilWhenNoWinConditionMet(t *testing.T) {
	state := fixtureState()
	state.CycleCount = 0

	outcome := Evaluate(state, Config{MaxCycles: 6})
	if outcome != nil {
		t.Fatalf("expected nil outcome, got %+v", outcome)
	}
}

func fixtureState() model.GameState {
	return model.GameState{
		TickID:     10,
		CycleCount: 0,
		Players: []model.PlayerState{
			{
				ID:        "warden",
				Role:      model.RoleWarden,
				Faction:   model.FactionAuthority,
				Alignment: model.AlignmentGood,
				Alive:     true,
			},
			{
				ID:        "gooddep",
				Role:      model.RoleDeputy,
				Faction:   model.FactionAuthority,
				Alignment: model.AlignmentGood,
				Alive:     true,
			},
			{
				ID:        "evildep",
				Role:      model.RoleDeputy,
				Faction:   model.FactionAuthority,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
			{
				ID:        "gang",
				Role:      model.RoleGangLeader,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
			{
				ID:        "gmember",
				Role:      model.RoleGangMember,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
			{
				ID:        "snitch",
				Role:      model.RoleSnitch,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentGood,
				Alive:     true,
			},
			{
				ID:        "neutral",
				Role:      model.RoleNeutralPrisoner,
				Faction:   model.FactionNeutral,
				Alignment: model.AlignmentNeutral,
				Alive:     true,
			},
		},
	}
}
