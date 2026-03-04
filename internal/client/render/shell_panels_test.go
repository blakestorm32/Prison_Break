package render

import (
	"encoding/json"
	"strings"
	"testing"

	"prison-break/internal/client/input"
	"prison-break/internal/client/netclient"
	gameitems "prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestShellActionPanelUseCardGeneratesUseCardCommand(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Local",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: "cell_block_a",
		Cards:         []model.CardType{model.CardMorphine, model.CardSpeed},
	}, nil)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelCardsPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	useCard, found := findCommandByType(commands, model.CmdUseCard)
	if !found {
		t.Fatalf("expected use-card command from cards panel, got %+v", commands)
	}

	var payload model.CardUsePayload
	if err := json.Unmarshal(useCard.Payload, &payload); err != nil {
		t.Fatalf("decode card payload: %v", err)
	}
	if payload.Card != model.CardMorphine {
		t.Fatalf("expected first card selection to use morphine, got %+v", payload)
	}
}

func TestShellActionPanelInventorySelectionAndUseItem(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Local",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: "cell_block_a",
		Inventory: []model.ItemStack{
			{Item: model.ItemWood, Quantity: 2},
			{Item: model.ItemMetalSlab, Quantity: 1},
		},
	}, nil)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelInventoryPressed: true}
		case 3:
			return input.InputSnapshot{PanelNextPressed: true}
		case 5:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	useItem, found := findCommandByType(commands, model.CmdUseItem)
	if !found {
		t.Fatalf("expected use-item command from inventory panel, got %+v", commands)
	}
	var payload model.ItemUsePayload
	if err := json.Unmarshal(useItem.Payload, &payload); err != nil {
		t.Fatalf("decode item payload: %v", err)
	}
	if payload.Item != model.ItemMetalSlab || payload.Amount != 1 {
		t.Fatalf("expected second inventory selection metal_slab x1, got %+v", payload)
	}
}

func TestShellActionPanelAbilityUsesRoleAbilityAndTargetsLocalRoomPlayer(t *testing.T) {
	extraPlayers := []model.PlayerState{
		{
			ID:            "p2",
			Name:          "Target",
			Alive:         true,
			CurrentRoomID: "warden_hq",
			Position:      model.Vector2{X: 6, Y: 5},
		},
		{
			ID:            "p3",
			Name:          "FarAway",
			Alive:         true,
			CurrentRoomID: "courtyard",
			Position:      model.Vector2{X: 30, Y: 30},
		},
	}
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Local",
		Alive:         true,
		Faction:       model.FactionAuthority,
		Role:          model.RoleDeputy,
		CurrentRoomID: "warden_hq",
		Position:      model.Vector2{X: 5, Y: 5},
	}, extraPlayers)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelAbilitiesPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	useAbility, found := findCommandByType(commands, model.CmdUseAbility)
	if !found {
		t.Fatalf("expected use-ability command from abilities panel, got %+v", commands)
	}

	var payload model.AbilityUsePayload
	if err := json.Unmarshal(useAbility.Payload, &payload); err != nil {
		t.Fatalf("decode ability payload: %v", err)
	}
	if payload.Ability != model.AbilitySearch {
		t.Fatalf("expected deputy default ability selection to be search, got %+v", payload)
	}
	if payload.TargetPlayerID != "p2" {
		t.Fatalf("expected in-room target player p2, got %+v", payload)
	}
}

func TestShellActionPanelAbilityUsesAssignedAbilityWhenPresent(t *testing.T) {
	extraPlayers := []model.PlayerState{
		{
			ID:            "p2",
			Name:          "Target",
			Alive:         true,
			CurrentRoomID: gamemap.RoomCorridorMain,
			Position:      model.Vector2{X: 6, Y: 5},
		},
	}
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:              "p1",
		Name:            "Local",
		Alive:           true,
		Faction:         model.FactionAuthority,
		Role:            model.RoleDeputy,
		AssignedAbility: model.AbilityTracker,
		CurrentRoomID:   gamemap.RoomCorridorMain,
		Position:        model.Vector2{X: 5, Y: 5},
	}, extraPlayers)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelAbilitiesPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	useAbility, found := findCommandByType(commands, model.CmdUseAbility)
	if !found {
		t.Fatalf("expected use-ability command from abilities panel, got %+v", commands)
	}

	var payload model.AbilityUsePayload
	if err := json.Unmarshal(useAbility.Payload, &payload); err != nil {
		t.Fatalf("decode ability payload: %v", err)
	}
	if payload.Ability != model.AbilityTracker {
		t.Fatalf("expected assigned tracker ability payload, got %+v", payload)
	}
}

func TestShellActionPanelMobileTouchAugmentsSnapshotButtons(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:      "p1",
		Name:    "Local",
		Alive:   true,
		Faction: model.FactionAuthority,
		Role:    model.RoleWarden,
	}, nil)

	layout := shell.actionPanelLayout()
	snapshot := shell.augmentSnapshotWithPanelTouches(input.InputSnapshot{
		Touches: []input.TouchPoint{
			{ID: 1, X: layout.cardsTab.MinX + 4, Y: layout.cardsTab.MinY + 4},
			{ID: 2, X: layout.marketTab.MinX + 4, Y: layout.marketTab.MinY + 4},
			{ID: 3, X: layout.escapeTab.MinX + 4, Y: layout.escapeTab.MinY + 4},
			{ID: 4, X: layout.useButton.MinX + 4, Y: layout.useButton.MinY + 4},
		},
	})
	if !snapshot.PanelCardsPressed {
		t.Fatalf("expected touch in cards tab to set PanelCardsPressed")
	}
	if !snapshot.PanelUsePressed {
		t.Fatalf("expected touch in use button to set PanelUsePressed")
	}
	if !snapshot.PanelMarketPressed {
		t.Fatalf("expected touch in market tab to set PanelMarketPressed")
	}
	if !snapshot.PanelEscapePressed {
		t.Fatalf("expected touch in escape tab to set PanelEscapePressed")
	}
}

func TestShellActionPanelMarketUseGeneratesPurchaseCommandWhenEligible(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
		Cards: []model.CardType{
			model.CardMoney,
			model.CardMoney,
		},
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseNight, gamemap.RoomCourtyard)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{InteractPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	buyCmd, found := findCommandByType(commands, model.CmdBlackMarketBuy)
	if !found {
		t.Fatalf("expected black-market buy command from market panel, got %+v", commands)
	}

	var payload model.BlackMarketPurchasePayload
	if err := json.Unmarshal(buyCmd.Payload, &payload); err != nil {
		t.Fatalf("decode black-market payload: %v", err)
	}
	catalog := gameitems.BlackMarketCatalog()
	if len(catalog) == 0 {
		t.Fatalf("expected non-empty market catalog")
	}
	if payload.Item != catalog[0].Item {
		t.Fatalf("expected first offer item %s, got %+v", catalog[0].Item, payload)
	}
}

func TestShellActionPanelMarketUseBlockedWhenNotAffordable(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
		Cards:         []model.CardType{model.CardSpeed},
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseNight, gamemap.RoomCourtyard)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{InteractPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	if _, found := findCommandByType(commands, model.CmdBlackMarketBuy); found {
		t.Fatalf("expected no black-market buy command when unaffordable, got %+v", commands)
	}
}

func TestShellActionPanelEscapeUseGeneratesInteractCommand(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Escaper",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
		Inventory: []model.ItemStack{
			{Item: model.ItemShovel, Quantity: 1},
		},
	}, nil)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelEscapePressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	interactCmd, found := findCommandByType(commands, model.CmdInteract)
	if !found {
		t.Fatalf("expected interact command from escape panel, got %+v", commands)
	}

	var payload model.InteractPayload
	if err := json.Unmarshal(interactCmd.Payload, &payload); err != nil {
		t.Fatalf("decode interact payload: %v", err)
	}
	if payload.EscapeRoute != model.EscapeRouteCourtyardDig {
		t.Fatalf("expected default escape route selection courtyard_dig, got %+v", payload)
	}
}

func TestShellActionPanelMarketHotkeyShowsInstructionWithoutOpeningModal(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseNight, gamemap.RoomCourtyard)

	shell.inputSnapshotProvider = func() input.InputSnapshot {
		return input.InputSnapshot{PanelMarketPressed: true}
	}

	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()
	if _, found := findCommandByType(commands, model.CmdBlackMarketBuy); found {
		t.Fatalf("expected market hotkey not to dispatch purchase command, got %+v", commands)
	}
	if shell.panelMode != actionPanelNone {
		t.Fatalf("expected market hotkey not to open modal, got mode %d", shell.panelMode)
	}
	if !strings.Contains(strings.ToLower(shell.panelLocalHint), "interact") {
		t.Fatalf("expected market hotkey guidance to mention interact, got %q", shell.panelLocalHint)
	}
}

func TestShellActionPanelMarketInteractBlockedDuringDayShowsReason(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseDay, gamemap.RoomCourtyard)

	shell.inputSnapshotProvider = func() input.InputSnapshot {
		return input.InputSnapshot{InteractPressed: true}
	}

	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()
	if _, found := findCommandByType(commands, model.CmdInteract); found {
		t.Fatalf("expected blocked market interact to suppress server interact command, got %+v", commands)
	}
	if _, found := findCommandByType(commands, model.CmdBlackMarketBuy); found {
		t.Fatalf("expected blocked market interact not to dispatch market command, got %+v", commands)
	}
	if shell.panelMode != actionPanelNone {
		t.Fatalf("expected blocked market interact not to open modal, got mode %d", shell.panelMode)
	}
	if !strings.Contains(strings.ToLower(shell.panelLocalHint), "night") {
		t.Fatalf("expected blocked market reason to mention night, got %q", shell.panelLocalHint)
	}
}

func TestShellActionPanelEscapeUsesArrowSelection(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Escaper",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Inventory: []model.ItemStack{
			{Item: model.ItemBadge, Quantity: 1},
		},
	}, nil)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelEscapePressed: true}
		case 3:
			return input.InputSnapshot{PanelNextPressed: true}
		case 5:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	interactCmd, found := findCommandByType(commands, model.CmdInteract)
	if !found {
		t.Fatalf("expected escape interact command after selecting next route, got %+v", commands)
	}
	var payload model.InteractPayload
	if err := json.Unmarshal(interactCmd.Payload, &payload); err != nil {
		t.Fatalf("decode interact payload: %v", err)
	}
	if payload.EscapeRoute != model.EscapeRouteBadgeEscape {
		t.Fatalf("expected second route badge_escape after arrow selection, got %+v", payload)
	}
}

func TestShellActionPanelMarketModalSuppressesMovementCommands(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
		Cards: []model.CardType{
			model.CardMoney,
			model.CardMoney,
		},
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseNight, gamemap.RoomCourtyard)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{InteractPressed: true}
		case 3:
			return input.InputSnapshot{MoveRight: true, PanelNextPressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	if _, found := findCommandByType(commands, model.CmdMoveIntent); found {
		t.Fatalf("expected centered modal navigation to suppress movement command, got %+v", commands)
	}
}

func TestShellActionPanelMoneyCardTargetsNearestLocalNPCPrisoner(t *testing.T) {
	entities := []model.EntityState{
		{
			ID:       40,
			Kind:     model.EntityKindNPCPrisoner,
			RoomID:   gamemap.RoomCourtyard,
			Position: model.Vector2{X: 8, Y: 8},
			Active:   true,
		},
		{
			ID:       42,
			Kind:     model.EntityKindNPCPrisoner,
			RoomID:   gamemap.RoomCourtyard,
			Position: model.Vector2{X: 11, Y: 10},
			Active:   true,
		},
		{
			ID:       99,
			Kind:     model.EntityKindNPCPrisoner,
			RoomID:   gamemap.RoomCellBlockA,
			Position: model.Vector2{X: 1, Y: 1},
			Active:   true,
		},
	}
	shell := newShellForActionPanelWithEntitiesTest(t, model.PlayerState{
		ID:            "p1",
		Name:          "Buyer",
		Alive:         true,
		Faction:       model.FactionPrisoner,
		Role:          model.RoleGangMember,
		CurrentRoomID: gamemap.RoomCourtyard,
		Position:      model.Vector2{X: 10, Y: 10},
		Cards:         []model.CardType{model.CardMoney},
	}, nil, entities)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelCardsPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	useCard, found := findCommandByType(commands, model.CmdUseCard)
	if !found {
		t.Fatalf("expected use-card command from cards panel, got %+v", commands)
	}

	var payload model.CardUsePayload
	if err := json.Unmarshal(useCard.Payload, &payload); err != nil {
		t.Fatalf("decode card payload: %v", err)
	}
	if payload.Card != model.CardMoney {
		t.Fatalf("expected money card payload, got %+v", payload)
	}
	if payload.TargetEntityID != 42 {
		t.Fatalf("expected nearest in-room npc prisoner entity id 42, got %+v", payload)
	}
}

func TestActionPanelAvailabilityHidesMarketAndEscapeForAuthority(t *testing.T) {
	availability := computeActionPanelAvailability(model.PlayerState{
		ID:      "auth-1",
		Alive:   true,
		Faction: model.FactionAuthority,
		Role:    model.RoleDeputy,
	})
	if availability.market {
		t.Fatalf("expected market panel hidden for authority roles")
	}
	if availability.escape {
		t.Fatalf("expected escape panel hidden for authority roles")
	}
}

func TestActionPanelAvailabilityShowsMarketAndEscapeForPrisoners(t *testing.T) {
	availability := computeActionPanelAvailability(model.PlayerState{
		ID:      "pris-1",
		Alive:   true,
		Faction: model.FactionPrisoner,
		Role:    model.RoleGangMember,
	})
	if !availability.market {
		t.Fatalf("expected market panel visible for prisoner roles")
	}
	if !availability.escape {
		t.Fatalf("expected escape panel visible for prisoner roles")
	}
}

func TestShellActionPanelIgnoresMarketToggleForAuthority(t *testing.T) {
	shell := newShellForActionPanelTest(t, model.PlayerState{
		ID:            "auth-1",
		Name:          "Authority",
		Alive:         true,
		Faction:       model.FactionAuthority,
		Role:          model.RoleDeputy,
		CurrentRoomID: gamemap.RoomCorridorMain,
		Cards:         []model.CardType{model.CardMoney, model.CardMoney},
	}, nil)
	setMarketStateForPanelTest(t, shell, model.PhaseNight, gamemap.RoomCorridorMain)

	frame := 0
	shell.inputSnapshotProvider = func() input.InputSnapshot {
		frame++
		switch frame {
		case 1:
			return input.InputSnapshot{PanelMarketPressed: true}
		case 3:
			return input.InputSnapshot{PanelUsePressed: true}
		default:
			return input.InputSnapshot{}
		}
	}

	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()
	_ = shell.Update()
	commands := shell.DrainOutgoingCommands()

	if _, found := findCommandByType(commands, model.CmdBlackMarketBuy); found {
		t.Fatalf("expected authority panel flow to block market command dispatch, got %+v", commands)
	}
}

func TestTargetDoorForPanelReturnsZeroWhenAdjacentDoorIsAccessDenied(t *testing.T) {
	local := model.PlayerState{
		ID:            "deputy",
		Role:          model.RoleDeputy,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCorridorMain,
	}
	mapState := model.MapState{
		PowerOn: true,
		Doors: []model.DoorState{
			{
				ID:    1,
				RoomA: gamemap.RoomWardenHQ,
				RoomB: gamemap.RoomCorridorMain,
				Open:  true,
			},
		},
	}

	if doorID := targetDoorForPanel(local, mapState); doorID != 0 {
		t.Fatalf("expected no eligible target door, got %d", doorID)
	}
}

func TestTargetDoorForPanelPrefersFirstEligibleDoor(t *testing.T) {
	local := model.PlayerState{
		ID:            "prisoner",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		CurrentRoomID: gamemap.RoomCorridorMain,
	}
	mapState := model.MapState{
		PowerOn: true,
		Doors: []model.DoorState{
			{
				ID:    1,
				RoomA: gamemap.RoomWardenHQ,
				RoomB: gamemap.RoomCorridorMain,
				Open:  true,
			},
			{
				ID:    8,
				RoomA: gamemap.RoomCourtyard,
				RoomB: gamemap.RoomCorridorMain,
				Open:  true,
			},
		},
	}

	if doorID := targetDoorForPanel(local, mapState); doorID != 8 {
		t.Fatalf("expected first accessible door id 8, got %d", doorID)
	}
}

func newShellForActionPanelTest(t *testing.T, local model.PlayerState, extraPlayers []model.PlayerState) *Shell {
	t.Helper()

	return newShellForActionPanelWithEntitiesTest(t, local, extraPlayers, nil)
}

func newShellForActionPanelWithEntitiesTest(
	t *testing.T,
	local model.PlayerState,
	extraPlayers []model.PlayerState,
	entities []model.EntityState,
) *Shell {
	t.Helper()

	store := netclient.NewSnapshotStore()
	players := make([]model.PlayerState, 0, 1+len(extraPlayers))
	players = append(players, local)
	players = append(players, extraPlayers...)

	state := model.GameState{
		MatchID:  "panel-match",
		TickID:   50,
		Status:   model.MatchStatusRunning,
		Map:      gamemap.DefaultPrisonLayout().ToMapState(),
		Players:  players,
		Entities: append([]model.EntityState(nil), entities...),
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: state.TickID,
		State:  &state,
	}) {
		t.Fatalf("expected baseline snapshot apply")
	}

	return NewShell(ShellConfig{
		ScreenWidth:   1280,
		ScreenHeight:  720,
		LocalPlayerID: local.ID,
		Store:         store,
		Layout:        gamemap.DefaultPrisonLayout(),
		InputController: input.NewController(input.ControllerConfig{
			PlayerID: local.ID,
		}),
		InputSnapshotProvider: func() input.InputSnapshot { return input.InputSnapshot{} },
	})
}

func findCommandByType(commands []model.InputCommand, commandType model.InputCommandType) (model.InputCommand, bool) {
	for _, command := range commands {
		if command.Type == commandType {
			return command, true
		}
	}
	return model.InputCommand{}, false
}

func setMarketStateForPanelTest(
	t *testing.T,
	shell *Shell,
	phase model.PhaseType,
	marketRoom model.RoomID,
) {
	t.Helper()

	state, ok := shell.store.CurrentState()
	if !ok {
		t.Fatalf("expected baseline shell state for market test")
	}
	state.TickID++
	state.Phase.Current = phase
	state.Map.BlackMarketRoomID = marketRoom
	if !shell.store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: state.TickID,
		State:  &state,
	}) {
		t.Fatalf("expected market state snapshot apply")
	}
}
