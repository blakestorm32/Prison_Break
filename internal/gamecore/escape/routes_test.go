package escape

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestIsKnownRoute(t *testing.T) {
	known := []model.EscapeRouteType{
		model.EscapeRouteCourtyardDig,
		model.EscapeRouteBadgeEscape,
		model.EscapeRoutePowerOutEscape,
		model.EscapeRouteLadderEscape,
		model.EscapeRouteRoofHelicopter,
	}
	for _, route := range known {
		if !IsKnownRoute(route) {
			t.Fatalf("expected route %q to be known", route)
		}
	}

	if IsKnownRoute(model.EscapeRouteType("unknown_route")) {
		t.Fatalf("expected unknown route to be rejected")
	}
}

func TestCanAttemptRouteRequiresCorrectRoomItemsAndState(t *testing.T) {
	baseMap := model.MapState{PowerOn: true}

	prisoner := model.PlayerState{
		ID:      "p1",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
		Alive:   true,
	}

	t.Run("courtyard_dig_requires_shovel", func(t *testing.T) {
		player := prisoner
		player.CurrentRoomID = gamemap.RoomCourtyard
		player.Inventory = []model.ItemStack{
			{Item: model.ItemShovel, Quantity: 1},
		}
		if !CanAttemptRoute(model.EscapeRouteCourtyardDig, player, baseMap) {
			t.Fatalf("expected courtyard dig route to succeed with shovel in courtyard")
		}

		player.Inventory = nil
		if CanAttemptRoute(model.EscapeRouteCourtyardDig, player, baseMap) {
			t.Fatalf("expected courtyard dig route to fail without shovel")
		}
	})

	t.Run("badge_escape_requires_badge_in_corridor", func(t *testing.T) {
		player := prisoner
		player.CurrentRoomID = gamemap.RoomCorridorMain
		player.Inventory = []model.ItemStack{
			{Item: model.ItemBadge, Quantity: 1},
		}
		if !CanAttemptRoute(model.EscapeRouteBadgeEscape, player, baseMap) {
			t.Fatalf("expected badge escape route to succeed with badge in corridor")
		}

		player.CurrentRoomID = gamemap.RoomMailRoom
		if CanAttemptRoute(model.EscapeRouteBadgeEscape, player, baseMap) {
			t.Fatalf("expected badge escape route to fail outside corridor")
		}
	})

	t.Run("power_out_escape_requires_power_room_and_power_off", func(t *testing.T) {
		player := prisoner
		player.CurrentRoomID = gamemap.RoomPowerRoom

		powerOnMap := model.MapState{PowerOn: true}
		if CanAttemptRoute(model.EscapeRoutePowerOutEscape, player, powerOnMap) {
			t.Fatalf("expected power-out escape route to fail while power is on")
		}

		powerOffMap := model.MapState{PowerOn: false}
		if !CanAttemptRoute(model.EscapeRoutePowerOutEscape, player, powerOffMap) {
			t.Fatalf("expected power-out escape route to succeed while power is off")
		}
	})

	t.Run("ladder_escape_requires_two_ladders", func(t *testing.T) {
		player := prisoner
		player.CurrentRoomID = gamemap.RoomCourtyard
		player.Inventory = []model.ItemStack{
			{Item: model.ItemLadder, Quantity: 1},
		}
		if CanAttemptRoute(model.EscapeRouteLadderEscape, player, baseMap) {
			t.Fatalf("expected ladder escape route to fail with one ladder")
		}

		player.Inventory = []model.ItemStack{
			{Item: model.ItemLadder, Quantity: 2},
		}
		if !CanAttemptRoute(model.EscapeRouteLadderEscape, player, baseMap) {
			t.Fatalf("expected ladder escape route to succeed with two ladders")
		}
	})

	t.Run("roof_helicopter_requires_keys", func(t *testing.T) {
		player := prisoner
		player.CurrentRoomID = gamemap.RoomRoofLookout
		player.Inventory = []model.ItemStack{
			{Item: model.ItemKeys, Quantity: 1},
		}
		if !CanAttemptRoute(model.EscapeRouteRoofHelicopter, player, baseMap) {
			t.Fatalf("expected roof-helicopter route to succeed with keys on roof")
		}

		player.Inventory = nil
		if CanAttemptRoute(model.EscapeRouteRoofHelicopter, player, baseMap) {
			t.Fatalf("expected roof-helicopter route to fail without keys")
		}
	})
}

func TestCanAttemptRouteRejectsNonPrisonersAndDeadPlayers(t *testing.T) {
	baseMap := model.MapState{PowerOn: false}

	authority := model.PlayerState{
		ID:            "warden",
		Role:          model.RoleWarden,
		Faction:       model.FactionAuthority,
		Alive:         true,
		CurrentRoomID: gamemap.RoomPowerRoom,
	}
	if CanAttemptRoute(model.EscapeRoutePowerOutEscape, authority, baseMap) {
		t.Fatalf("expected authority to be blocked from prisoner escape routes")
	}

	deadPrisoner := model.PlayerState{
		ID:            "dead",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		Alive:         false,
		CurrentRoomID: gamemap.RoomCourtyard,
		Inventory: []model.ItemStack{
			{Item: model.ItemShovel, Quantity: 1},
		},
	}
	if CanAttemptRoute(model.EscapeRouteCourtyardDig, deadPrisoner, baseMap) {
		t.Fatalf("expected dead prisoner to be blocked from escape routes")
	}
}

func TestEvaluateRouteIncludesDeterministicRequirementFeedback(t *testing.T) {
	baseMap := model.MapState{PowerOn: true}
	player := model.PlayerState{
		ID:            "p1",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		Alive:         true,
		CurrentRoomID: gamemap.RoomCourtyard,
	}

	eval := EvaluateRoute(model.EscapeRouteCourtyardDig, player, baseMap)
	if eval.RouteLabel != "Courtyard Dig" {
		t.Fatalf("expected route label Courtyard Dig, got %q", eval.RouteLabel)
	}
	if eval.CanAttempt {
		t.Fatalf("expected attempt to fail without shovel")
	}
	if eval.FailureReason == "" {
		t.Fatalf("expected failure reason for unmet requirements")
	}
	if len(eval.Requirements) < 4 {
		t.Fatalf("expected full requirement matrix, got %+v", eval.Requirements)
	}

	player.Inventory = []model.ItemStack{{Item: model.ItemShovel, Quantity: 1}}
	eval = EvaluateRoute(model.EscapeRouteCourtyardDig, player, baseMap)
	if !eval.CanAttempt {
		t.Fatalf("expected attempt to succeed once all requirements are met: %+v", eval)
	}
	if eval.FailureReason != "" {
		t.Fatalf("expected no failure reason on success, got %q", eval.FailureReason)
	}
}

func TestEvaluateAllRoutesReturnsAllKnownRoutesInOrder(t *testing.T) {
	player := model.PlayerState{
		ID:      "p1",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
		Alive:   true,
	}
	evals := EvaluateAllRoutes(player, model.MapState{PowerOn: true})
	if len(evals) != len(KnownRoutes()) {
		t.Fatalf("expected one evaluation per known route, got %d", len(evals))
	}
	for index, route := range KnownRoutes() {
		if evals[index].Route != route {
			t.Fatalf("expected deterministic route order at index %d: want %s got %s", index, route, evals[index].Route)
		}
	}
}
