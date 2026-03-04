package items

import "testing"

func TestBlackMarketCatalogIsDeterministicAndValid(t *testing.T) {
	first := BlackMarketCatalog()
	second := BlackMarketCatalog()

	if len(first) == 0 {
		t.Fatalf("expected non-empty market catalog")
	}
	if len(first) != len(second) {
		t.Fatalf("expected copied market catalogs to match length: %d vs %d", len(first), len(second))
	}

	for index := range first {
		left := first[index]
		right := second[index]
		if left != right {
			t.Fatalf("expected deterministic offer order at index %d, got %v vs %v", index, left, right)
		}
		if !IsKnownItem(left.Item) {
			t.Fatalf("expected catalog item to be known: %s", left.Item)
		}
		if left.Quantity == 0 {
			t.Fatalf("expected positive offer quantity for %s", left.Item)
		}
		if left.MoneyCardCost == 0 {
			t.Fatalf("expected positive money-card cost for %s", left.Item)
		}
	}
}

func TestBlackMarketOfferLookupByItem(t *testing.T) {
	offer, ok := BlackMarketOfferForItem("missing_item")
	if ok {
		t.Fatalf("expected lookup miss for unknown item, got %+v", offer)
	}

	offer, ok = BlackMarketOfferForItem("golden_bullet")
	if !ok {
		t.Fatalf("expected known market offer for golden_bullet")
	}
	if offer.MoneyCardCost != 3 {
		t.Fatalf("expected golden_bullet price to be 3 money cards, got %d", offer.MoneyCardCost)
	}
	if !IsBlackMarketItem("golden_bullet") {
		t.Fatalf("expected IsBlackMarketItem to return true for golden_bullet")
	}
	if IsBlackMarketItem("not_real") {
		t.Fatalf("expected IsBlackMarketItem false for unknown item")
	}
}
