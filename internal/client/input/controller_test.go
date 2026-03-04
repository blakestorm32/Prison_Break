package input

import (
	"encoding/json"
	"math"
	"testing"

	"prison-break/internal/shared/model"
)

func TestControllerBuildCommandsKeyboardMoveAimAndFire(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID:     "p1",
		ScreenWidth:  1280,
		ScreenHeight: 720,
	})

	local := &model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 10, Y: 10},
	}
	commands := controller.BuildCommands(InputSnapshot{
		MoveUp:      true,
		MoveRight:   true,
		Sprint:      true,
		FirePressed: true,
		HasAim:      true,
		AimWorldX:   12,
		AimWorldY:   8,
	}, 11, local)

	if len(commands) != 3 {
		t.Fatalf("expected 3 commands (move, aim, fire), got %d", len(commands))
	}
	if commands[0].Type != model.CmdMoveIntent || commands[1].Type != model.CmdAimIntent || commands[2].Type != model.CmdFireWeapon {
		t.Fatalf("unexpected command order/types: %+v", commands)
	}
	for index, command := range commands {
		if command.PlayerID != "p1" {
			t.Fatalf("expected player id p1, got %s", command.PlayerID)
		}
		if command.ClientSeq != uint64(index+1) {
			t.Fatalf("expected sequential client seq, got %d at index %d", command.ClientSeq, index)
		}
		if command.TargetTick != 11 {
			t.Fatalf("expected target tick 11, got %d", command.TargetTick)
		}
	}

	var move model.MovementInputPayload
	mustDecodePayload(t, commands[0].Payload, &move)
	if !move.Sprint {
		t.Fatalf("expected sprint flag true for movement payload")
	}
	if math.Abs(float64(move.MoveX)-0.707) > 0.02 || math.Abs(float64(move.MoveY)+0.707) > 0.02 {
		t.Fatalf("expected normalized diagonal move vector, got x=%f y=%f", move.MoveX, move.MoveY)
	}

	var aim model.AimInputPayload
	mustDecodePayload(t, commands[1].Payload, &aim)
	if aim.AimX != 2 || aim.AimY != -2 {
		t.Fatalf("expected aim vector local->target (2,-2), got (%f,%f)", aim.AimX, aim.AimY)
	}

	var fire model.FireWeaponPayload
	mustDecodePayload(t, commands[2].Payload, &fire)
	if fire.Weapon != model.ItemPistol {
		t.Fatalf("expected default fire weapon pistol, got %s", fire.Weapon)
	}
	if fire.TargetX != 12 || fire.TargetY != 8 {
		t.Fatalf("expected fire target world (12,8), got (%f,%f)", fire.TargetX, fire.TargetY)
	}
}

func TestControllerActionButtonsAreEdgeTriggered(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	first := controller.BuildCommands(InputSnapshot{
		FirePressed:     true,
		InteractPressed: true,
		ReloadPressed:   true,
	}, 5, nil)

	if len(first) != 3 {
		t.Fatalf("expected first press to emit 3 action commands, got %d", len(first))
	}
	if first[0].Type != model.CmdFireWeapon || first[1].Type != model.CmdInteract || first[2].Type != model.CmdReload {
		t.Fatalf("unexpected first action command order: %+v", first)
	}

	second := controller.BuildCommands(InputSnapshot{
		FirePressed:     true,
		InteractPressed: true,
		ReloadPressed:   true,
	}, 6, nil)
	if len(second) != 0 {
		t.Fatalf("expected held buttons to not re-trigger edge actions, got %+v", second)
	}

	third := controller.BuildCommands(InputSnapshot{}, 7, nil)
	if len(third) != 0 {
		t.Fatalf("expected release frame to emit no actions, got %+v", third)
	}

	fourth := controller.BuildCommands(InputSnapshot{
		FirePressed: true,
	}, 8, nil)
	if len(fourth) != 1 || fourth[0].Type != model.CmdFireWeapon {
		t.Fatalf("expected fire press after release to re-trigger exactly once, got %+v", fourth)
	}
	if fourth[0].ClientSeq != 4 {
		t.Fatalf("expected sequence to continue after suppressed frames, got %d", fourth[0].ClientSeq)
	}
}

func TestControllerAbilityButtonIsEdgeTriggeredAndUsesAssignedAbility(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	local := &model.PlayerState{
		ID:              "p1",
		Role:            model.RoleGangMember,
		Faction:         model.FactionPrisoner,
		AssignedAbility: model.AbilityDisguise,
	}

	first := controller.BuildCommands(InputSnapshot{
		AbilityPressed: true,
	}, 0, local)
	if len(first) != 1 || first[0].Type != model.CmdUseAbility {
		t.Fatalf("expected first ability press to emit one use-ability command, got %+v", first)
	}

	var payload model.AbilityUsePayload
	mustDecodePayload(t, first[0].Payload, &payload)
	if payload.Ability != model.AbilityDisguise {
		t.Fatalf("expected assigned ability disguise in payload, got %+v", payload)
	}

	second := controller.BuildCommands(InputSnapshot{
		AbilityPressed: true,
	}, 0, local)
	if len(second) != 0 {
		t.Fatalf("expected held ability button to not retrigger, got %+v", second)
	}

	third := controller.BuildCommands(InputSnapshot{}, 0, local)
	if len(third) != 0 {
		t.Fatalf("expected release frame to emit no ability command, got %+v", third)
	}

	fourth := controller.BuildCommands(InputSnapshot{
		AbilityPressed: true,
	}, 0, local)
	if len(fourth) != 1 || fourth[0].Type != model.CmdUseAbility {
		t.Fatalf("expected ability press after release to retrigger once, got %+v", fourth)
	}
}

func TestControllerTouchJoystickAndButtons(t *testing.T) {
	layout := DefaultMobileLayout(1000, 600)
	controller := NewController(ControllerConfig{
		PlayerID:     "p1",
		ScreenWidth:  1000,
		ScreenHeight: 600,
		MobileLayout: layout,
	})

	local := &model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 20, Y: 12},
	}

	commands := controller.BuildCommands(InputSnapshot{
		HasAim:    true,
		AimWorldX: 23,
		AimWorldY: 10,
		Touches: []TouchPoint{
			{
				ID: 7,
				X:  layout.JoystickCenterX + layout.JoystickRadius,
				Y:  layout.JoystickCenterY,
			},
			{
				ID: 8,
				X:  (layout.FireButton.MinX + layout.FireButton.MaxX) / 2,
				Y:  (layout.FireButton.MinY + layout.FireButton.MaxY) / 2,
			},
		},
	}, 9, local)

	if len(commands) != 3 {
		t.Fatalf("expected touch move+aim+fire commands, got %+v", commands)
	}

	var move model.MovementInputPayload
	mustDecodePayload(t, commands[0].Payload, &move)
	if move.MoveX < 0.95 || math.Abs(float64(move.MoveY)) > 0.01 {
		t.Fatalf("expected rightward joystick move near (1,0), got (%f,%f)", move.MoveX, move.MoveY)
	}
	if !move.Sprint {
		t.Fatalf("expected joystick-at-edge movement to set sprint true")
	}

	var fire model.FireWeaponPayload
	mustDecodePayload(t, commands[2].Payload, &fire)
	if fire.TargetX != 23 || fire.TargetY != 10 {
		t.Fatalf("expected touch fire command to preserve aim target, got (%f,%f)", fire.TargetX, fire.TargetY)
	}

	next := controller.BuildCommands(InputSnapshot{
		HasAim:    true,
		AimWorldX: 24,
		AimWorldY: 10,
		Touches: []TouchPoint{
			{
				ID: 7,
				X:  layout.JoystickCenterX + layout.JoystickRadius,
				Y:  layout.JoystickCenterY,
			},
			{
				ID: 8,
				X:  (layout.FireButton.MinX + layout.FireButton.MaxX) / 2,
				Y:  (layout.FireButton.MinY + layout.FireButton.MaxY) / 2,
			},
		},
	}, 10, local)

	if len(next) != 2 {
		t.Fatalf("expected held touch fire button to suppress repeat fire, got %+v", next)
	}
	if next[0].Type != model.CmdMoveIntent || next[1].Type != model.CmdAimIntent {
		t.Fatalf("expected only continuous move+aim commands on held touch, got %+v", next)
	}
}

func TestControllerContinuousCommandsAreThrottledPerTargetTick(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})
	local := &model.PlayerState{
		ID:       "p1",
		Position: model.Vector2{X: 10, Y: 10},
	}

	first := controller.BuildCommands(InputSnapshot{
		MoveRight: true,
		HasAim:    true,
		AimWorldX: 12,
		AimWorldY: 10,
	}, 20, local)
	if len(first) != 2 {
		t.Fatalf("expected first frame move+aim commands, got %+v", first)
	}

	secondSameTick := controller.BuildCommands(InputSnapshot{
		MoveRight: true,
		HasAim:    true,
		AimWorldX: 12,
		AimWorldY: 10,
	}, 20, local)
	if len(secondSameTick) != 0 {
		t.Fatalf("expected no repeated continuous commands for same target tick, got %+v", secondSameTick)
	}

	thirdNextTick := controller.BuildCommands(InputSnapshot{
		MoveRight: true,
		HasAim:    true,
		AimWorldX: 12,
		AimWorldY: 10,
	}, 21, local)
	if len(thirdNextTick) != 2 {
		t.Fatalf("expected move+aim commands again at next target tick, got %+v", thirdNextTick)
	}
}

func TestControllerWithEmptyPlayerIDEmitsNoCommands(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "   ",
	})

	commands := controller.BuildCommands(InputSnapshot{
		MoveUp: true,
	}, 1, nil)
	if len(commands) != 0 {
		t.Fatalf("expected no commands for empty player id controller, got %+v", commands)
	}
}

func TestControllerFallsBackToKnownFireWeapon(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID:     "p1",
		FireWeapon:   model.ItemType("laser"),
		ScreenWidth:  800,
		ScreenHeight: 600,
	})

	commands := controller.BuildCommands(InputSnapshot{
		FirePressed: true,
		HasAim:      true,
		AimWorldX:   2,
		AimWorldY:   3,
	}, 2, nil)
	if len(commands) != 2 {
		t.Fatalf("expected aim+fire commands, got %+v", commands)
	}

	var fire model.FireWeaponPayload
	mustDecodePayload(t, commands[1].Payload, &fire)
	if fire.Weapon != model.ItemPistol {
		t.Fatalf("expected invalid configured weapon to fall back to pistol, got %s", fire.Weapon)
	}
}

func TestControllerMobileLayoutAccessorReturnsConfiguredLayout(t *testing.T) {
	layout := DefaultMobileLayout(900, 600)
	controller := NewController(ControllerConfig{
		PlayerID:     "p1",
		ScreenWidth:  900,
		ScreenHeight: 600,
		MobileLayout: layout,
	})

	got := controller.MobileLayout()
	if !got.Enabled {
		t.Fatalf("expected configured mobile layout to remain enabled")
	}
	if got.JoystickCenterX != layout.JoystickCenterX || got.JoystickCenterY != layout.JoystickCenterY {
		t.Fatalf("expected layout centers to roundtrip through accessor, got=%+v want=%+v", got, layout)
	}
}

func TestControllerBuildUseCardCommandProducesValidPayloadAndSequence(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	cardCmd, ok := controller.BuildUseCardCommand(model.CardUsePayload{
		Card: model.CardMorphine,
	}, 20)
	if !ok {
		t.Fatalf("expected known card command to build")
	}
	if cardCmd.Type != model.CmdUseCard {
		t.Fatalf("expected use-card command type, got %s", cardCmd.Type)
	}
	if cardCmd.ClientSeq != 1 {
		t.Fatalf("expected first command sequence to be 1, got %d", cardCmd.ClientSeq)
	}
	var cardPayload model.CardUsePayload
	mustDecodePayload(t, cardCmd.Payload, &cardPayload)
	if cardPayload.Card != model.CardMorphine {
		t.Fatalf("expected morphine card payload, got %+v", cardPayload)
	}

	abilityCmd, ok := controller.BuildUseAbilityCommand(model.AbilityUsePayload{
		Ability: model.AbilitySearch,
	}, 21)
	if !ok {
		t.Fatalf("expected known ability command to build")
	}
	if abilityCmd.Type != model.CmdUseAbility {
		t.Fatalf("expected use-ability command type, got %s", abilityCmd.Type)
	}
	if abilityCmd.ClientSeq != 2 {
		t.Fatalf("expected second command sequence to be 2, got %d", abilityCmd.ClientSeq)
	}

	marketCmd, ok := controller.BuildBlackMarketBuyCommand(model.BlackMarketPurchasePayload{
		Item: model.ItemShiv,
	}, 22)
	if !ok {
		t.Fatalf("expected known black-market item command to build")
	}
	if marketCmd.Type != model.CmdBlackMarketBuy {
		t.Fatalf("expected black-market command type, got %s", marketCmd.Type)
	}
	if marketCmd.ClientSeq != 3 {
		t.Fatalf("expected third command sequence to be 3, got %d", marketCmd.ClientSeq)
	}
	var marketPayload model.BlackMarketPurchasePayload
	mustDecodePayload(t, marketCmd.Payload, &marketPayload)
	if marketPayload.Item != model.ItemShiv {
		t.Fatalf("expected shiv payload for market purchase, got %+v", marketPayload)
	}

	interactCmd, ok := controller.BuildInteractCommand(model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	}, 23)
	if !ok {
		t.Fatalf("expected known escape-route interact command to build")
	}
	if interactCmd.Type != model.CmdInteract {
		t.Fatalf("expected interact command type, got %s", interactCmd.Type)
	}
	if interactCmd.ClientSeq != 4 {
		t.Fatalf("expected fourth command sequence to be 4, got %d", interactCmd.ClientSeq)
	}
	var interactPayload model.InteractPayload
	mustDecodePayload(t, interactCmd.Payload, &interactPayload)
	if interactPayload.EscapeRoute != model.EscapeRouteCourtyardDig {
		t.Fatalf("expected courtyard_dig payload for interact command, got %+v", interactPayload)
	}
}

func TestControllerBuildUseAbilityCommandRejectsUnknownAbility(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	if _, ok := controller.BuildUseAbilityCommand(model.AbilityUsePayload{
		Ability: model.AbilityType("bad_ability"),
	}, 5); ok {
		t.Fatalf("expected unknown ability to be rejected")
	}
}

func TestControllerBuildUseItemCommandRejectsEmptyItem(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	if _, ok := controller.BuildUseItemCommand(model.ItemUsePayload{}, 5); ok {
		t.Fatalf("expected empty item payload to be rejected")
	}
}

func TestControllerBuildBlackMarketBuyRejectsUnknownMarketItem(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	if _, ok := controller.BuildBlackMarketBuyCommand(model.BlackMarketPurchasePayload{
		Item: model.ItemLadder,
	}, 5); ok {
		t.Fatalf("expected non-market item to be rejected")
	}
}

func TestControllerBuildInteractCommandRejectsUnknownEscapeRoute(t *testing.T) {
	controller := NewController(ControllerConfig{
		PlayerID: "p1",
	})

	if _, ok := controller.BuildInteractCommand(model.InteractPayload{
		EscapeRoute: model.EscapeRouteType("bad_route"),
	}, 5); ok {
		t.Fatalf("expected unknown escape route to be rejected")
	}
}

func mustDecodePayload(t *testing.T, payload json.RawMessage, out any) {
	t.Helper()
	if err := json.Unmarshal(payload, out); err != nil {
		t.Fatalf("decode payload failed: %v payload=%s", err, string(payload))
	}
}
