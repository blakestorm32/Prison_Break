package determinism

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestHashGameStateIgnoresOrderingNoise(t *testing.T) {
	stateA := fixtureState()
	stateA.Players[0].Inventory = []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
		{Item: model.ItemBullet, Quantity: 2},
	}
	stateA.Players[0].Cards = []model.CardType{
		model.CardMoney,
		model.CardSpeed,
	}
	stateA.Players[0].Effects = []model.EffectState{
		{Effect: model.EffectArmorPlate, EndsTick: 20, Stacks: 1, SourcePID: "p2"},
		{Effect: model.EffectStunned, EndsTick: 10, Stacks: 1},
	}
	stateA.Map.Doors = []model.DoorState{
		{ID: 2, Open: true},
		{ID: 1, Open: false},
	}
	stateA.Map.Cells = []model.CellState{
		{ID: 2, OccupantPlayerIDs: []model.PlayerID{"p3", "p1"}},
		{ID: 1, OccupantPlayerIDs: []model.PlayerID{"p2"}},
	}
	stateA.Map.RestrictedZones = []model.ZoneState{
		{ID: 2, Restricted: true},
		{ID: 1, Restricted: true},
	}
	stateA.Entities = []model.EntityState{
		{ID: 2, Active: true, Tags: []string{"b", "a"}},
		{ID: 1, Active: true, Tags: []string{"z", "x"}},
	}
	stateA.GameOver = &model.GameOverState{
		Reason:          model.WinReasonWardenDied,
		EndedTick:       30,
		WinnerPlayerIDs: []model.PlayerID{"p2", "p1"},
	}

	stateB := cloneGameState(stateA)
	stateB.Players = []model.PlayerState{stateB.Players[2], stateB.Players[0], stateB.Players[1]}

	p2Index := findPlayer(stateB.Players, "p2")
	if p2Index < 0 {
		t.Fatalf("expected to find p2 in reordered state")
	}

	stateB.Players[p2Index].Inventory = []model.ItemStack{
		{Item: model.ItemBullet, Quantity: 2},
		{Item: model.ItemShiv, Quantity: 1},
	}
	stateB.Players[p2Index].Cards = []model.CardType{
		model.CardSpeed,
		model.CardMoney,
	}
	stateB.Players[p2Index].Effects = []model.EffectState{
		{Effect: model.EffectStunned, EndsTick: 10, Stacks: 1},
		{Effect: model.EffectArmorPlate, EndsTick: 20, Stacks: 1, SourcePID: "p2"},
	}

	stateB.Map.Doors = []model.DoorState{
		{ID: 1, Open: false},
		{ID: 2, Open: true},
	}
	stateB.Map.Cells = []model.CellState{
		{ID: 1, OccupantPlayerIDs: []model.PlayerID{"p2"}},
		{ID: 2, OccupantPlayerIDs: []model.PlayerID{"p1", "p3"}},
	}
	stateB.Map.RestrictedZones = []model.ZoneState{
		{ID: 1, Restricted: true},
		{ID: 2, Restricted: true},
	}
	stateB.Entities = []model.EntityState{
		{ID: 1, Active: true, Tags: []string{"x", "z"}},
		{ID: 2, Active: true, Tags: []string{"a", "b"}},
	}
	stateB.GameOver = &model.GameOverState{
		Reason:          model.WinReasonWardenDied,
		EndedTick:       30,
		WinnerPlayerIDs: []model.PlayerID{"p1", "p2"},
	}

	hashA, err := HashGameState(stateA)
	if err != nil {
		t.Fatalf("hash state A: %v", err)
	}
	hashB, err := HashGameState(stateB)
	if err != nil {
		t.Fatalf("hash state B: %v", err)
	}

	if hashA != hashB {
		t.Fatalf("expected equal hashes for equivalent states with ordering differences, got %q and %q", hashA, hashB)
	}
}

func TestHashGameStateChangesWhenSemanticDataChanges(t *testing.T) {
	stateA := fixtureState()
	stateB := cloneGameState(stateA)

	p1Index := findPlayer(stateB.Players, "p1")
	if p1Index < 0 {
		t.Fatalf("expected to find p1 in fixture state")
	}
	stateB.Players[p1Index].HeartsHalf--

	hashA, err := HashGameState(stateA)
	if err != nil {
		t.Fatalf("hash state A: %v", err)
	}
	hashB, err := HashGameState(stateB)
	if err != nil {
		t.Fatalf("hash state B: %v", err)
	}

	if hashA == hashB {
		t.Fatalf("expected hash difference when semantic field changes")
	}
}
