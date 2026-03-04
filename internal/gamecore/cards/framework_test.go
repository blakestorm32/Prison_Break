package cards

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestCardValidationAndSlotLimit(t *testing.T) {
	player := model.PlayerState{}

	if ok := AddCard(&player, model.CardMorphine); !ok {
		t.Fatalf("expected add morphine card to succeed")
	}
	if ok := AddCard(&player, model.CardBullet); !ok {
		t.Fatalf("expected add bullet card to succeed")
	}
	if ok := AddCard(&player, model.CardSpeed); !ok {
		t.Fatalf("expected add speed card to succeed")
	}
	if ok := AddCard(&player, model.CardArmorPlate); ok {
		t.Fatalf("expected add to fail when max card slots reached")
	}

	wantSorted := []model.CardType{
		model.CardBullet,
		model.CardMorphine,
		model.CardSpeed,
	}
	if !reflect.DeepEqual(player.Cards, wantSorted) {
		t.Fatalf("expected deterministic sorted cards, got=%v want=%v", player.Cards, wantSorted)
	}

	if IsKnownCard(model.CardType("unknown_card")) {
		t.Fatalf("expected unknown card to fail known-card check")
	}
}

func TestHasAndRemoveCard(t *testing.T) {
	player := model.PlayerState{
		Cards: []model.CardType{
			model.CardDoorStop,
			model.CardMorphine,
		},
	}

	if !HasCard(player, model.CardMorphine) {
		t.Fatalf("expected has-card check to find morphine")
	}
	if !RemoveCard(&player, model.CardMorphine) {
		t.Fatalf("expected remove-card to succeed")
	}
	if HasCard(player, model.CardMorphine) {
		t.Fatalf("expected morphine card to be removed")
	}
	if RemoveCard(&player, model.CardMorphine) {
		t.Fatalf("expected second morphine removal to fail")
	}
}

func TestDurationsAndDeterministicGrab(t *testing.T) {
	if got := SpeedDurationTicks(30); got != 150 {
		t.Fatalf("expected speed duration 150 ticks at 30Hz, got %d", got)
	}
	if got := DoorStopDurationTicks(30); got != 300 {
		t.Fatalf("expected door-stop duration 300 ticks at 30Hz, got %d", got)
	}

	first := DeterministicGrabItem("p1", 10)
	second := DeterministicGrabItem("p1", 10)
	if first != second {
		t.Fatalf("expected deterministic grab item for same player/tick, got %s and %s", first, second)
	}

	other := DeterministicGrabItem("p2", 10)
	if first == "" || other == "" {
		t.Fatalf("expected deterministic grab output to always be a known item")
	}
}

func TestDeterministicGrabFromInventory(t *testing.T) {
	inventory := []model.ItemStack{
		{Item: model.ItemWireCutters, Quantity: 1},
		{Item: model.ItemWood, Quantity: 2},
		{Item: model.ItemBullet, Quantity: 1},
	}

	first := DeterministicGrabFromInventory("actor", "target", 7, inventory)
	second := DeterministicGrabFromInventory("actor", "target", 7, inventory)
	if first == "" {
		t.Fatalf("expected deterministic inventory grab to return an item")
	}
	if first != second {
		t.Fatalf("expected deterministic inventory grab for same inputs, got %s and %s", first, second)
	}

	otherActor := DeterministicGrabFromInventory("other", "target", 7, inventory)
	if otherActor == "" {
		t.Fatalf("expected non-empty item for other actor")
	}
	if none := DeterministicGrabFromInventory("actor", "target", 7, nil); none != "" {
		t.Fatalf("expected empty inventory to produce no item, got %s", none)
	}
}
