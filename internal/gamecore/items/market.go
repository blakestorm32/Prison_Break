package items

import "prison-break/internal/shared/model"

type BlackMarketOffer struct {
	Item          model.ItemType `json:"item"`
	Quantity      uint8          `json:"quantity"`
	MoneyCardCost uint8          `json:"money_card_cost"`
}

var blackMarketOffers = []BlackMarketOffer{
	{Item: model.ItemWood, Quantity: 1, MoneyCardCost: 1},
	{Item: model.ItemMetalSlab, Quantity: 1, MoneyCardCost: 1},
	{Item: model.ItemBullet, Quantity: 1, MoneyCardCost: 1},
	{Item: model.ItemLockPick, Quantity: 1, MoneyCardCost: 1},
	{Item: model.ItemShiv, Quantity: 1, MoneyCardCost: 2},
	{Item: model.ItemDoorStop, Quantity: 1, MoneyCardCost: 2},
	{Item: model.ItemSilencer, Quantity: 1, MoneyCardCost: 2},
	{Item: model.ItemSatchel, Quantity: 1, MoneyCardCost: 2},
	{Item: model.ItemWireCutters, Quantity: 1, MoneyCardCost: 2},
	{Item: model.ItemPistol, Quantity: 1, MoneyCardCost: 3},
	{Item: model.ItemHuntingRifle, Quantity: 1, MoneyCardCost: 3},
	{Item: model.ItemGoldenBullet, Quantity: 1, MoneyCardCost: 3},
}

func BlackMarketCatalog() []BlackMarketOffer {
	out := make([]BlackMarketOffer, len(blackMarketOffers))
	copy(out, blackMarketOffers)
	return out
}

func BlackMarketOfferForItem(item model.ItemType) (BlackMarketOffer, bool) {
	for _, offer := range blackMarketOffers {
		if offer.Item == item {
			return offer, true
		}
	}
	return BlackMarketOffer{}, false
}

func IsBlackMarketItem(item model.ItemType) bool {
	_, exists := BlackMarketOfferForItem(item)
	return exists
}
