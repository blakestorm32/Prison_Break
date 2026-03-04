package items

import (
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestAddItemNormalizesMergesAndSortsStacks(t *testing.T) {
	player := model.PlayerState{
		Inventory: []model.ItemStack{
			{Item: model.ItemShiv, Quantity: 1},
			{Item: model.ItemBullet, Quantity: 2},
			{Item: model.ItemShiv, Quantity: 2},
			{Item: model.ItemType("unknown_item"), Quantity: 10},
			{Item: model.ItemWood, Quantity: 0},
		},
	}

	if ok := AddItem(&player, model.ItemBullet, 3); !ok {
		t.Fatalf("expected add item to succeed")
	}

	want := []model.ItemStack{
		{Item: model.ItemBullet, Quantity: 5},
		{Item: model.ItemShiv, Quantity: 3},
	}
	if !reflect.DeepEqual(player.Inventory, want) {
		t.Fatalf("unexpected normalized inventory: got=%v want=%v", player.Inventory, want)
	}
}

func TestAddItemRespectsCapacityAndSatchelExpansion(t *testing.T) {
	player := model.PlayerState{
		Inventory: []model.ItemStack{
			{Item: model.ItemBadge, Quantity: 1},
			{Item: model.ItemBullet, Quantity: 1},
			{Item: model.ItemDoorStop, Quantity: 1},
			{Item: model.ItemKeys, Quantity: 1},
			{Item: model.ItemMetalSlab, Quantity: 1},
			{Item: model.ItemWood, Quantity: 1},
		},
	}

	if ok := AddItem(&player, model.ItemLockPick, 1); ok {
		t.Fatalf("expected add to fail at base stack limit")
	}

	if ok := AddItem(&player, model.ItemSatchel, 1); !ok {
		t.Fatalf("expected satchel add to succeed and expand capacity")
	}
	if limit := InventoryStackLimit(player); limit != 9 {
		t.Fatalf("expected satchel-adjusted capacity 9, got %d", limit)
	}

	if ok := AddItem(&player, model.ItemLadder, 1); !ok {
		t.Fatalf("expected ladder add after satchel capacity increase")
	}
	if ok := AddItem(&player, model.ItemPistol, 1); !ok {
		t.Fatalf("expected pistol add after satchel capacity increase")
	}
	if ok := AddItem(&player, model.ItemShovel, 1); ok {
		t.Fatalf("expected add to fail when satchel-adjusted capacity is exceeded")
	}

	if ok := RemoveItem(&player, model.ItemSatchel, 1); ok {
		t.Fatalf("expected satchel removal to fail while inventory would exceed base capacity")
	}
}

func TestTransferItemIsAtomicWhenDestinationCannotCarryNewStack(t *testing.T) {
	source := model.PlayerState{
		ID: "source",
		Inventory: []model.ItemStack{
			{Item: model.ItemShiv, Quantity: 1},
		},
	}
	destination := model.PlayerState{
		ID: "destination",
		Inventory: []model.ItemStack{
			{Item: model.ItemBadge, Quantity: 1},
			{Item: model.ItemBullet, Quantity: 1},
			{Item: model.ItemDoorStop, Quantity: 1},
			{Item: model.ItemKeys, Quantity: 1},
			{Item: model.ItemMetalSlab, Quantity: 1},
			{Item: model.ItemWood, Quantity: 1},
		},
	}

	sourceBefore := append([]model.ItemStack(nil), source.Inventory...)
	destinationBefore := append([]model.ItemStack(nil), destination.Inventory...)

	if ok := TransferItem(&source, &destination, model.ItemShiv, 1); ok {
		t.Fatalf("expected transfer to fail when destination cannot add a new stack")
	}
	if !reflect.DeepEqual(source.Inventory, sourceBefore) {
		t.Fatalf("expected source inventory unchanged on failed transfer: got=%v want=%v", source.Inventory, sourceBefore)
	}
	if !reflect.DeepEqual(destination.Inventory, destinationBefore) {
		t.Fatalf("expected destination inventory unchanged on failed transfer: got=%v want=%v", destination.Inventory, destinationBefore)
	}

	if ok := RemoveItem(&destination, model.ItemBadge, 1); !ok {
		t.Fatalf("expected destination slot free-up to succeed")
	}
	if ok := TransferItem(&source, &destination, model.ItemShiv, 1); !ok {
		t.Fatalf("expected transfer to succeed after freeing destination capacity")
	}
	if HasItem(source, model.ItemShiv, 1) {
		t.Fatalf("expected source to no longer carry transferred shiv")
	}
	if !HasItem(destination, model.ItemShiv, 1) {
		t.Fatalf("expected destination to receive transferred shiv")
	}
}

func TestCraftConsumesInputsAndIsAtomicOnOutputOverflow(t *testing.T) {
	player := model.PlayerState{
		Inventory: []model.ItemStack{
			{Item: model.ItemMetalSlab, Quantity: 1},
			{Item: model.ItemShiv, Quantity: 255},
			{Item: model.ItemWood, Quantity: 1},
		},
	}

	before := append([]model.ItemStack(nil), player.Inventory...)
	if ok := Craft(&player, model.ItemShiv); ok {
		t.Fatalf("expected craft to fail due output stack overflow")
	}
	if !reflect.DeepEqual(player.Inventory, before) {
		t.Fatalf("expected craft failure to be atomic: got=%v want=%v", player.Inventory, before)
	}

	player.Inventory = []model.ItemStack{
		{Item: model.ItemMetalSlab, Quantity: 1},
		{Item: model.ItemShiv, Quantity: 254},
		{Item: model.ItemWood, Quantity: 1},
	}
	if ok := Craft(&player, model.ItemShiv); !ok {
		t.Fatalf("expected craft to succeed with space in output stack")
	}
	if !HasItem(player, model.ItemShiv, 255) {
		t.Fatalf("expected crafted output quantity to reach 255 shivs")
	}
	if HasItem(player, model.ItemWood, 1) || HasItem(player, model.ItemMetalSlab, 1) {
		t.Fatalf("expected craft inputs to be consumed")
	}

	if ok := Craft(&player, model.ItemType("unknown_item")); ok {
		t.Fatalf("expected unknown craft output to fail")
	}
}

func TestContrabandStacksDetection(t *testing.T) {
	player := model.PlayerState{
		Inventory: []model.ItemStack{
			{Item: model.ItemBullet, Quantity: 3},
			{Item: model.ItemLadder, Quantity: 1},
			{Item: model.ItemShiv, Quantity: 2},
		},
	}

	got := ContrabandStacks(player)
	want := []model.ItemStack{
		{Item: model.ItemLadder, Quantity: 1},
		{Item: model.ItemShiv, Quantity: 2},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected contraband stack list: got=%v want=%v", got, want)
	}
	if !HasContraband(player) {
		t.Fatalf("expected contraband detection to be true")
	}
}

func TestDroppedItemTagRoundTripAndValidation(t *testing.T) {
	tags := BuildDroppedItemTags(model.ItemShiv, 2)
	if !containsTag(tags, "contraband") {
		t.Fatalf("expected contraband tag for dropped shiv")
	}

	entity := model.EntityState{
		ID:   10,
		Kind: model.EntityKindDroppedItem,
		Tags: tags,
	}
	item, qty, ok := ParseDroppedItem(entity)
	if !ok {
		t.Fatalf("expected dropped-item tag parse to succeed")
	}
	if item != model.ItemShiv || qty != 2 {
		t.Fatalf("unexpected parsed dropped item: item=%s qty=%d", item, qty)
	}

	_, _, ok = ParseDroppedItem(model.EntityState{
		ID:   11,
		Kind: model.EntityKindDroppedItem,
		Tags: []string{"qty:1"},
	})
	if ok {
		t.Fatalf("expected parse failure when item tag is missing")
	}

	_, _, ok = ParseDroppedItem(model.EntityState{
		ID:   12,
		Kind: model.EntityKindDroppedItem,
		Tags: []string{"item:shiv", "qty:999"},
	})
	if ok {
		t.Fatalf("expected parse failure for out-of-range quantity")
	}

	_, _, ok = ParseDroppedItem(model.EntityState{
		ID:   13,
		Kind: model.EntityKindPlayer,
		Tags: tags,
	})
	if ok {
		t.Fatalf("expected parse failure for non-dropped entity kinds")
	}
}

func containsTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
}
