package roles

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestApplyGangLeaderSuccessionPromotesDeterministically(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{
				ID:        "leader",
				Role:      model.RoleGangLeader,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     false,
			},
			{
				ID:        "g1",
				Role:      model.RoleGangMember,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
			{
				ID:        "g2",
				Role:      model.RoleGangMember,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
		},
	}

	firstState := cloneStateForSuccessionTest(state)
	secondState := cloneStateForSuccessionTest(state)

	firstChanged := ApplyGangLeaderSuccession(&firstState, "match-001", 42)
	secondChanged := ApplyGangLeaderSuccession(&secondState, "match-001", 42)

	if !reflect.DeepEqual(firstChanged, secondChanged) {
		t.Fatalf("expected deterministic changed-id set, got first=%v second=%v", firstChanged, secondChanged)
	}
	if len(firstChanged) == 0 {
		t.Fatalf("expected at least one player change from succession")
	}

	aliveLeaderCount := 0
	var newLeader model.PlayerID
	for _, player := range firstState.Players {
		if player.Role != model.RoleGangLeader {
			continue
		}
		if player.Alive {
			aliveLeaderCount++
			newLeader = player.ID
		}
	}
	if aliveLeaderCount != 1 {
		t.Fatalf("expected exactly one living gang leader after succession, got %d", aliveLeaderCount)
	}
	if newLeader != "g1" && newLeader != "g2" {
		t.Fatalf("expected successor to be one of alive gang members, got %q", newLeader)
	}

	for _, player := range firstState.Players {
		if player.ID != "leader" {
			continue
		}
		if player.Role != model.RoleGangMember {
			t.Fatalf("expected dead prior gang leader to be demoted to gang_member, got %s", player.Role)
		}
	}
}

func TestApplyGangLeaderSuccessionNoOpWhenLeaderAlive(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{
				ID:        "leader",
				Role:      model.RoleGangLeader,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
			{
				ID:        "g1",
				Role:      model.RoleGangMember,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     true,
			},
		},
	}

	changed := ApplyGangLeaderSuccession(&state, "match-001", 10)
	if len(changed) != 0 {
		t.Fatalf("expected no changes while a living gang leader exists, got %v", changed)
	}
}

func TestApplyGangLeaderSuccessionNoOpWhenNoEligibleGangMember(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{
				ID:        "leader",
				Role:      model.RoleGangLeader,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     false,
			},
			{
				ID:        "g1",
				Role:      model.RoleGangMember,
				Faction:   model.FactionPrisoner,
				Alignment: model.AlignmentEvil,
				Alive:     false,
			},
		},
	}

	changed := ApplyGangLeaderSuccession(&state, "match-001", 10)
	if len(changed) != 0 {
		t.Fatalf("expected no succession when no gang members are alive, got %v", changed)
	}
}

func cloneStateForSuccessionTest(in model.GameState) model.GameState {
	out := in
	out.Players = append([]model.PlayerState(nil), in.Players...)
	return out
}
