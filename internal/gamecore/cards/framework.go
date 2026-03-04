package cards

import (
	"hash/fnv"
	"sort"

	"prison-break/internal/shared/model"
)

const (
	MaxCardsHeld            = 3
	SpeedDurationSeconds    = 5
	DoorStopDurationSeconds = 10
)

var knownCards = map[model.CardType]struct{}{
	model.CardMorphine:         {},
	model.CardBullet:           {},
	model.CardMoney:            {},
	model.CardSpeed:            {},
	model.CardArmorPlate:       {},
	model.CardLockSnap:         {},
	model.CardItemSteal:        {},
	model.CardItemGrab:         {},
	model.CardScrapBundle:      {},
	model.CardDoorStop:         {},
	model.CardGetOutOfJailFree: {},
}

var deterministicGrabPool = []model.ItemType{
	model.ItemWood,
	model.ItemMetalSlab,
	model.ItemLockPick,
	model.ItemShiv,
	model.ItemBullet,
	model.ItemWireCutters,
}

func IsKnownCard(card model.CardType) bool {
	_, exists := knownCards[card]
	return exists
}

func HasCard(player model.PlayerState, card model.CardType) bool {
	for _, existing := range player.Cards {
		if existing == card {
			return true
		}
	}
	return false
}

func AddCard(player *model.PlayerState, card model.CardType) bool {
	if player == nil || !IsKnownCard(card) {
		return false
	}
	if len(player.Cards) >= MaxCardsHeld {
		return false
	}

	player.Cards = append(player.Cards, card)
	sort.Slice(player.Cards, func(i int, j int) bool {
		return player.Cards[i] < player.Cards[j]
	})
	return true
}

func RemoveCard(player *model.PlayerState, card model.CardType) bool {
	if player == nil || !IsKnownCard(card) || len(player.Cards) == 0 {
		return false
	}

	for index := range player.Cards {
		if player.Cards[index] != card {
			continue
		}
		player.Cards = append(player.Cards[:index], player.Cards[index+1:]...)
		return true
	}

	return false
}

func SpeedDurationTicks(tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}
	return uint64(SpeedDurationSeconds) * uint64(tickRateHz)
}

func DoorStopDurationTicks(tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}
	return uint64(DoorStopDurationSeconds) * uint64(tickRateHz)
}

func DeterministicGrabItem(playerID model.PlayerID, tickID uint64) model.ItemType {
	if len(deterministicGrabPool) == 0 {
		return ""
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(playerID))
	var bytes [8]byte
	for index := range bytes {
		bytes[index] = byte(tickID >> (uint(index) * 8))
	}
	_, _ = hasher.Write(bytes[:])

	choice := hasher.Sum64() % uint64(len(deterministicGrabPool))
	return deterministicGrabPool[choice]
}

func DeterministicGrabFromInventory(
	actorPlayerID model.PlayerID,
	targetPlayerID model.PlayerID,
	tickID uint64,
	inventory []model.ItemStack,
) model.ItemType {
	candidates := make([]model.ItemType, 0, len(inventory))
	for _, stack := range inventory {
		if stack.Quantity == 0 || stack.Item == "" {
			continue
		}
		candidates = append(candidates, stack.Item)
	}
	if len(candidates) == 0 {
		return ""
	}

	sort.Slice(candidates, func(i int, j int) bool {
		return candidates[i] < candidates[j]
	})

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(actorPlayerID))
	_, _ = hasher.Write([]byte(targetPlayerID))
	var bytes [8]byte
	for index := range bytes {
		bytes[index] = byte(tickID >> (uint(index) * 8))
	}
	_, _ = hasher.Write(bytes[:])

	choice := hasher.Sum64() % uint64(len(candidates))
	return candidates[choice]
}
