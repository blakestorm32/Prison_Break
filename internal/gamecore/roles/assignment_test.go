package roles

import (
	"fmt"
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestAssignDeterministicForSameInputs(t *testing.T) {
	playerIDs := []model.PlayerID{"p3", "p1", "p2", "p4", "p5", "p6"}

	first, err := Assign(playerIDs, "match-000001")
	if err != nil {
		t.Fatalf("first assign failed: %v", err)
	}
	second, err := Assign(playerIDs, "match-000001")
	if err != nil {
		t.Fatalf("second assign failed: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic assignment for same inputs")
	}
}

func TestAssignSupportsSixToTwelvePlayersWithCoreRoleGuarantees(t *testing.T) {
	for playerCount := 6; playerCount <= 12; playerCount++ {
		playerIDs := make([]model.PlayerID, 0, playerCount)
		for idx := 1; idx <= playerCount; idx++ {
			playerIDs = append(playerIDs, model.PlayerID(fmt.Sprintf("p%02d", idx)))
		}

		assignments, err := Assign(playerIDs, model.MatchID(fmt.Sprintf("match-role-%d", playerCount)))
		if err != nil {
			t.Fatalf("assign failed for count %d: %v", playerCount, err)
		}
		if len(assignments) != playerCount {
			t.Fatalf("expected %d assignments, got %d", playerCount, len(assignments))
		}

		var wardenCount int
		var gangLeaderCount int
		for _, assignment := range assignments {
			if assignment.Role == "" || assignment.Faction == "" || assignment.Alignment == "" {
				t.Fatalf("expected fully specified assignment for %+v", assignment)
			}
			if assignment.Role == model.RoleWarden {
				wardenCount++
			}
			if assignment.Role == model.RoleGangLeader {
				gangLeaderCount++
			}
		}
		if wardenCount != 1 {
			t.Fatalf("expected exactly one warden for count %d, got %d", playerCount, wardenCount)
		}
		if gangLeaderCount != 1 {
			t.Fatalf("expected exactly one gang leader for count %d, got %d", playerCount, gangLeaderCount)
		}
	}
}

func TestAssignFallbackForSmallMatches(t *testing.T) {
	assignments, err := Assign([]model.PlayerID{"p1", "p2", "p3"}, "small")
	if err != nil {
		t.Fatalf("fallback assign failed: %v", err)
	}
	if len(assignments) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(assignments))
	}

	var hasWarden bool
	var hasGangLeader bool
	for _, assignment := range assignments {
		if assignment.Role == model.RoleWarden {
			hasWarden = true
		}
		if assignment.Role == model.RoleGangLeader {
			hasGangLeader = true
		}
	}
	if !hasWarden || !hasGangLeader {
		t.Fatalf("expected fallback assignment to include warden and gang leader, got %+v", assignments)
	}
}

func TestAssignRejectsDuplicatesAndEmptyInput(t *testing.T) {
	if _, err := Assign(nil, "none"); err != ErrNoPlayersProvided {
		t.Fatalf("expected ErrNoPlayersProvided, got %v", err)
	}

	_, err := Assign([]model.PlayerID{"p1", "p1"}, "dup")
	if err != ErrDuplicatePlayerID {
		t.Fatalf("expected ErrDuplicatePlayerID, got %v", err)
	}
}

func TestApplyAssignmentsToGameState(t *testing.T) {
	state := &model.GameState{
		MatchID: "assign",
		Players: []model.PlayerState{
			{ID: "p3"},
			{ID: "p2"},
			{ID: "p1"},
			{ID: "p4"},
			{ID: "p5"},
			{ID: "p6"},
		},
	}

	if err := ApplyAssignments(state, "assign-match"); err != nil {
		t.Fatalf("apply assignments failed: %v", err)
	}

	for _, player := range state.Players {
		if player.Role == "" || player.Faction == "" || player.Alignment == "" {
			t.Fatalf("expected applied role/faction/alignment for player %+v", player)
		}
	}
}
