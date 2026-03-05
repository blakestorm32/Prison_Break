package items

import (
	"sort"
	"strconv"
	"strings"

	"prison-break/internal/shared/model"
)

const (
	// BaseInventoryStackLimit is the number of distinct item stacks a player can carry.
	BaseInventoryStackLimit = 6
	// SatchelBonusStacks is the additional stack capacity granted per satchel carried.
	SatchelBonusStacks = 3

	droppedTagItemPrefix = "item:"
	droppedTagQtyPrefix  = "qty:"

	droppedTagMarker     = "dropped_item"
	droppedTagContraband = "contraband"
)

type Recipe struct {
	Output         model.ItemType    `json:"output"`
	OutputQuantity uint8             `json:"output_quantity"`
	Inputs         []model.ItemStack `json:"inputs"`
}

var knownItems = map[model.ItemType]struct{}{
	model.ItemBaton:        {},
	model.ItemWood:         {},
	model.ItemMetalSlab:    {},
	model.ItemShiv:         {},
	model.ItemBullet:       {},
	model.ItemPistol:       {},
	model.ItemHuntingRifle: {},
	model.ItemLockPick:     {},
	model.ItemWireCutters:  {},
	model.ItemSilencer:     {},
	model.ItemSatchel:      {},
	model.ItemDoorStop:     {},
	model.ItemGoldenBullet: {},
	model.ItemLadder:       {},
	model.ItemShovel:       {},
	model.ItemBadge:        {},
	model.ItemKeys:         {},
}

var contrabandItems = map[model.ItemType]struct{}{
	model.ItemShiv:         {},
	model.ItemPistol:       {},
	model.ItemHuntingRifle: {},
	model.ItemLadder:       {},
	model.ItemShovel:       {},
	model.ItemWireCutters:  {},
}

var recipesByOutput = map[model.ItemType]Recipe{
	model.ItemShiv: {
		Output:         model.ItemShiv,
		OutputQuantity: 1,
		Inputs: []model.ItemStack{
			{Item: model.ItemWood, Quantity: 1},
			{Item: model.ItemMetalSlab, Quantity: 1},
		},
	},
	model.ItemDoorStop: {
		Output:         model.ItemDoorStop,
		OutputQuantity: 1,
		Inputs: []model.ItemStack{
			{Item: model.ItemWood, Quantity: 1},
			{Item: model.ItemMetalSlab, Quantity: 1},
		},
	},
	model.ItemLadder: {
		Output:         model.ItemLadder,
		OutputQuantity: 1,
		Inputs: []model.ItemStack{
			{Item: model.ItemWood, Quantity: 2},
			{Item: model.ItemMetalSlab, Quantity: 1},
		},
	},
	model.ItemShovel: {
		Output:         model.ItemShovel,
		OutputQuantity: 1,
		Inputs: []model.ItemStack{
			{Item: model.ItemWood, Quantity: 1},
			{Item: model.ItemMetalSlab, Quantity: 2},
		},
	},
	model.ItemWireCutters: {
		Output:         model.ItemWireCutters,
		OutputQuantity: 1,
		Inputs: []model.ItemStack{
			{Item: model.ItemMetalSlab, Quantity: 2},
			{Item: model.ItemLockPick, Quantity: 1},
		},
	},
}

func IsKnownItem(item model.ItemType) bool {
	_, exists := knownItems[item]
	return exists
}

func IsContraband(item model.ItemType) bool {
	_, illegal := contrabandItems[item]
	return illegal
}

func InventoryStackLimit(player model.PlayerState) uint8 {
	return uint8(stackLimitForPlayer(player, normalizeInventory(player.Inventory)))
}

func HasItem(player model.PlayerState, item model.ItemType, amount uint8) bool {
	if !IsKnownItem(item) {
		return false
	}
	if amount == 0 {
		return true
	}

	inventory := normalizeInventory(player.Inventory)
	index := findStackIndex(inventory, item)
	if index < 0 {
		return false
	}
	return inventory[index].Quantity >= amount
}

func AddItem(player *model.PlayerState, item model.ItemType, amount uint8) bool {
	if player == nil || amount == 0 || !IsKnownItem(item) {
		return false
	}

	normalizedInventory := normalizeInventory(player.Inventory)
	nextInventory, ok := addToInventory(
		normalizedInventory,
		item,
		amount,
		baseStackLimitForPlayer(*player),
	)
	if !ok {
		return false
	}

	player.Inventory = nextInventory
	return true
}

func RemoveItem(player *model.PlayerState, item model.ItemType, amount uint8) bool {
	if player == nil || amount == 0 || !IsKnownItem(item) {
		return false
	}

	nextInventory, ok := removeFromInventory(normalizeInventory(player.Inventory), item, amount)
	if !ok {
		return false
	}
	if item == model.ItemSatchel && len(nextInventory) > stackLimitForPlayer(*player, nextInventory) {
		return false
	}

	player.Inventory = nextInventory
	return true
}

func TransferItem(from *model.PlayerState, to *model.PlayerState, item model.ItemType, amount uint8) bool {
	if from == nil || to == nil || amount == 0 || !IsKnownItem(item) {
		return false
	}
	if from.ID == to.ID {
		return false
	}

	sourceInventory := normalizeInventory(from.Inventory)
	destinationInventory := normalizeInventory(to.Inventory)

	sourceAfter, ok := removeFromInventory(sourceInventory, item, amount)
	if !ok {
		return false
	}

	destinationAfter, ok := addToInventory(
		destinationInventory,
		item,
		amount,
		baseStackLimitForPlayer(*to),
	)
	if !ok {
		return false
	}

	from.Inventory = sourceAfter
	to.Inventory = destinationAfter
	return true
}

func RecipeFor(output model.ItemType) (Recipe, bool) {
	recipe, exists := recipesByOutput[output]
	if !exists {
		return Recipe{}, false
	}

	cloned := Recipe{
		Output:         recipe.Output,
		OutputQuantity: recipe.OutputQuantity,
		Inputs:         append([]model.ItemStack(nil), recipe.Inputs...),
	}
	sort.Slice(cloned.Inputs, func(i int, j int) bool {
		return cloned.Inputs[i].Item < cloned.Inputs[j].Item
	})
	return cloned, true
}

func Craft(player *model.PlayerState, output model.ItemType) bool {
	if player == nil {
		return false
	}

	recipe, exists := RecipeFor(output)
	if !exists {
		return false
	}

	nextInventory := normalizeInventory(player.Inventory)
	for _, requirement := range recipe.Inputs {
		var ok bool
		nextInventory, ok = removeFromInventory(nextInventory, requirement.Item, requirement.Quantity)
		if !ok {
			return false
		}
	}

	var ok bool
	nextInventory, ok = addToInventory(
		nextInventory,
		recipe.Output,
		recipe.OutputQuantity,
		baseStackLimitForPlayer(*player),
	)
	if !ok {
		return false
	}

	player.Inventory = nextInventory
	return true
}

func ContrabandStacks(player model.PlayerState) []model.ItemStack {
	inventory := normalizeInventory(player.Inventory)
	out := make([]model.ItemStack, 0, len(inventory))
	for _, stack := range inventory {
		if IsContraband(stack.Item) {
			out = append(out, stack)
		}
	}
	return out
}

func HasContraband(player model.PlayerState) bool {
	return len(ContrabandStacks(player)) > 0
}

func BuildDroppedItemTags(item model.ItemType, quantity uint8) []string {
	if quantity == 0 {
		quantity = 1
	}

	tags := []string{
		droppedTagMarker,
		droppedTagItemPrefix + string(item),
		droppedTagQtyPrefix + strconv.Itoa(int(quantity)),
	}
	if IsContraband(item) {
		tags = append(tags, droppedTagContraband)
	}

	sort.Strings(tags)
	return tags
}

func ParseDroppedItem(entity model.EntityState) (model.ItemType, uint8, bool) {
	if entity.Kind != model.EntityKindDroppedItem {
		return "", 0, false
	}

	var (
		item     model.ItemType
		quantity uint8 = 1
	)

	for _, tag := range entity.Tags {
		if strings.HasPrefix(tag, droppedTagItemPrefix) {
			candidate := model.ItemType(strings.TrimPrefix(tag, droppedTagItemPrefix))
			if !IsKnownItem(candidate) {
				return "", 0, false
			}
			item = candidate
			continue
		}

		if strings.HasPrefix(tag, droppedTagQtyPrefix) {
			raw := strings.TrimPrefix(tag, droppedTagQtyPrefix)
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 || parsed > 255 {
				return "", 0, false
			}
			quantity = uint8(parsed)
		}
	}

	if item == "" {
		return "", 0, false
	}

	return item, quantity, true
}

func addToInventory(
	inventory []model.ItemStack,
	item model.ItemType,
	amount uint8,
	baseStackLimit int,
) ([]model.ItemStack, bool) {
	if amount == 0 || !IsKnownItem(item) {
		return nil, false
	}

	next := append([]model.ItemStack(nil), inventory...)
	index := findStackIndex(next, item)
	if index >= 0 {
		nextQuantity := int(next[index].Quantity) + int(amount)
		if nextQuantity > 255 {
			return nil, false
		}
		next[index].Quantity = uint8(nextQuantity)
	} else {
		next = append(next, model.ItemStack{
			Item:     item,
			Quantity: amount,
		})
	}

	next = normalizeInventory(next)
	if baseStackLimit <= 0 {
		baseStackLimit = BaseInventoryStackLimit
	}
	if len(next) > stackLimitForBase(next, baseStackLimit) {
		return nil, false
	}
	return next, true
}

func removeFromInventory(inventory []model.ItemStack, item model.ItemType, amount uint8) ([]model.ItemStack, bool) {
	if amount == 0 || !IsKnownItem(item) {
		return nil, false
	}

	next := append([]model.ItemStack(nil), inventory...)
	index := findStackIndex(next, item)
	if index < 0 {
		return nil, false
	}
	if next[index].Quantity < amount {
		return nil, false
	}

	next[index].Quantity -= amount
	if next[index].Quantity == 0 {
		next = append(next[:index], next[index+1:]...)
	}

	next = normalizeInventory(next)
	return next, true
}

func normalizeInventory(inventory []model.ItemStack) []model.ItemStack {
	if len(inventory) == 0 {
		return nil
	}

	aggregated := make(map[model.ItemType]uint16, len(inventory))
	for _, stack := range inventory {
		if stack.Quantity == 0 || !IsKnownItem(stack.Item) {
			continue
		}
		aggregated[stack.Item] += uint16(stack.Quantity)
	}

	normalized := make([]model.ItemStack, 0, len(aggregated))
	for item, quantity := range aggregated {
		if quantity == 0 {
			continue
		}
		if quantity > 255 {
			quantity = 255
		}
		normalized = append(normalized, model.ItemStack{
			Item:     item,
			Quantity: uint8(quantity),
		})
	}

	sort.Slice(normalized, func(i int, j int) bool {
		return normalized[i].Item < normalized[j].Item
	})
	return normalized
}

func findStackIndex(inventory []model.ItemStack, item model.ItemType) int {
	for index := range inventory {
		if inventory[index].Item == item {
			return index
		}
	}
	return -1
}

func stackLimitForPlayer(player model.PlayerState, inventory []model.ItemStack) int {
	return stackLimitForBase(inventory, baseStackLimitForPlayer(player))
}

func baseStackLimitForPlayer(player model.PlayerState) int {
	if player.InventorySlots > 0 {
		return int(player.InventorySlots)
	}
	return BaseInventoryStackLimit
}

func stackLimitForBase(inventory []model.ItemStack, baseLimit int) int {
	satchelCount := 0
	for _, stack := range inventory {
		if stack.Item != model.ItemSatchel {
			continue
		}
		satchelCount += int(stack.Quantity)
	}

	return baseLimit + (satchelCount * SatchelBonusStacks)
}
