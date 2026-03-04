package game

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"prison-break/internal/shared/model"
)

func TestSubmitInputRejectsInvalidCommandAndPayload(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "",
		ClientSeq: 1,
		Type:      model.CmdMoveIntent,
		Payload:   mustRawJSON(t, model.MovementInputPayload{MoveX: 1, MoveY: 0}),
	})
	if !errors.Is(err, ErrInvalidInputCommand) {
		t.Fatalf("expected ErrInvalidInputCommand for empty player id, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.InputCommandType("bad_type"),
		Payload:   json.RawMessage(`{}`),
	})
	if !errors.Is(err, ErrInvalidInputCommand) {
		t.Fatalf("expected ErrInvalidInputCommand for unsupported type, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 3,
		Type:      model.CmdMoveIntent,
		Payload:   json.RawMessage(`{bad-json`),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for malformed move payload, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "missing",
		ClientSeq: 4,
		Type:      model.CmdReload,
	})
	if !errors.Is(err, ErrInputPlayerMismatch) {
		t.Fatalf("expected ErrInputPlayerMismatch for unknown player, got %v", err)
	}
}

func TestSubmitInputRejectsUnknownItemsForItemCommands(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	unknownItem := model.ItemType("made_up_item")

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdUseItem,
		Payload: mustRawJSON(t, model.ItemUsePayload{
			Item: unknownItem,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown use-item value, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdDropItem,
		Payload: mustRawJSON(t, model.DropItemPayload{
			Item: unknownItem,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown drop-item value, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 3,
		Type:      model.CmdCraftItem,
		Payload: mustRawJSON(t, model.CraftItemPayload{
			Item: unknownItem,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown craft output, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 4,
		Type:      model.CmdBlackMarketBuy,
		Payload: mustRawJSON(t, model.BlackMarketPurchasePayload{
			Item: unknownItem,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown black-market item, got %v", err)
	}
}

func TestSubmitInputAcceptsKnownBlackMarketPurchasePayload(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdBlackMarketBuy,
		Payload: mustRawJSON(t, model.BlackMarketPurchasePayload{
			Item: model.ItemShiv,
		}),
	})
	if err != nil {
		t.Fatalf("expected known black-market item payload to validate, got %v", err)
	}
}

func TestSubmitInputFireWeaponAcceptsBatonAndRejectsUnknownWeapon(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdFireWeapon,
		Payload: mustRawJSON(t, model.FireWeaponPayload{
			Weapon:  model.ItemType("baton"),
			TargetX: 1,
			TargetY: 1,
		}),
	})
	if err != nil {
		t.Fatalf("expected baton weapon payload to validate, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdFireWeapon,
		Payload: mustRawJSON(t, model.FireWeaponPayload{
			Weapon:  model.ItemType("laser"),
			TargetX: 1,
			TargetY: 1,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected unknown fire weapon to be rejected, got %v", err)
	}
}

func TestSubmitInputRejectsUnknownAbilityAndCardPayloads(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdUseAbility,
		Payload: mustRawJSON(t, model.AbilityUsePayload{
			Ability: model.AbilityType("bad_ability"),
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown ability, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdUseCard,
		Payload: mustRawJSON(t, model.CardUsePayload{
			Card: model.CardType("bad_card"),
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown card, got %v", err)
	}
}

func TestSubmitInputAllowsAbilityPayloadWithoutTargetsForServerAutoTargeting(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdUseAbility,
		Payload: mustRawJSON(t, model.AbilityUsePayload{
			Ability: model.AbilitySearch,
		}),
	})
	if err != nil {
		t.Fatalf("expected missing search target player to be accepted for server-side target resolution, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdUseAbility,
		Payload: mustRawJSON(t, model.AbilityUsePayload{
			Ability: model.AbilityLocksmith,
		}),
	})
	if err != nil {
		t.Fatalf("expected missing locksmith target door to be accepted for server-side target resolution, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 3,
		Type:      model.CmdUseAbility,
		Payload: mustRawJSON(t, model.AbilityUsePayload{
			Ability: model.AbilityHacker,
		}),
	})
	if err != nil {
		t.Fatalf("expected non-targeted ability payload to validate, got %v", err)
	}
}

func TestSubmitInputRejectsCardPayloadWithoutRequiredTargets(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdUseCard,
		Payload: mustRawJSON(t, model.CardUsePayload{
			Card: model.CardDoorStop,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected missing door_stop target door to be rejected, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdUseCard,
		Payload: mustRawJSON(t, model.CardUsePayload{
			Card: model.CardItemSteal,
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected missing item_steal target player to be rejected, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 3,
		Type:      model.CmdUseCard,
		Payload: mustRawJSON(t, model.CardUsePayload{
			Card:       model.CardItemSteal,
			TargetItem: model.ItemType("unknown_item"),
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected unknown target item value to be rejected, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 4,
		Type:      model.CmdUseCard,
		Payload: mustRawJSON(t, model.CardUsePayload{
			Card: model.CardBullet,
		}),
	})
	if err != nil {
		t.Fatalf("expected non-targeted bullet card payload to validate, got %v", err)
	}
}

func TestSubmitInputRejectsUnknownEscapeRouteAndMarketRoom(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdInteract,
		Payload: mustRawJSON(t, model.InteractPayload{
			EscapeRoute: model.EscapeRouteType("invalid_route"),
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for unknown escape route, got %v", err)
	}

	_, err = manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 2,
		Type:      model.CmdInteract,
		Payload: mustRawJSON(t, model.InteractPayload{
			MarketRoomID: model.RoomID("warden_hq"),
		}),
	})
	if !errors.Is(err, ErrInvalidInputPayload) {
		t.Fatalf("expected ErrInvalidInputPayload for invalid market room, got %v", err)
	}
}

func TestSubmitInputRejectsWhenMatchNotRunning(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	})
	if !errors.Is(err, ErrMatchNotRunning) {
		t.Fatalf("expected ErrMatchNotRunning before start, got %v", err)
	}
}

func TestSubmitInputSchedulesAndAssignsIngressSequence(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	first, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdMoveIntent,
		Payload:   mustRawJSON(t, model.MovementInputPayload{MoveX: 1, MoveY: 0}),
	})
	if err != nil {
		t.Fatalf("submit first command failed: %v", err)
	}
	if first.IngressSeq != 1 || first.ScheduledTick != 1 {
		t.Fatalf("unexpected first submit result: %+v", first)
	}

	second, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:   "p1",
		ClientSeq:  2,
		Type:       model.CmdReload,
		TargetTick: 100,
	})
	if err != nil {
		t.Fatalf("submit second command failed: %v", err)
	}
	if second.IngressSeq != 2 || second.ScheduledTick != 2 {
		t.Fatalf("expected second command clamp to tick 2 with ingress 2, got %+v", second)
	}

	counts, err := manager.PendingInputCounts(match.MatchID)
	if err != nil {
		t.Fatalf("pending counts failed: %v", err)
	}
	if counts[1] != 1 || counts[2] != 1 {
		t.Fatalf("unexpected pending counts: %#v", counts)
	}
}

func TestSubmitInputLateHandlingContinuousVsDiscrete(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected test ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:   "p1",
		ClientSeq:  1,
		TargetTick: 1,
		Type:       model.CmdFireWeapon,
		Payload: mustRawJSON(t, model.FireWeaponPayload{
			Weapon:  model.ItemPistol,
			TargetX: 2,
			TargetY: 3,
		}),
	})
	if !errors.Is(err, ErrInputTooLateDropped) {
		t.Fatalf("expected ErrInputTooLateDropped for late discrete command, got %v", err)
	}

	result, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:   "p1",
		ClientSeq:  2,
		TargetTick: 1,
		Type:       model.CmdMoveIntent,
		Payload:    mustRawJSON(t, model.MovementInputPayload{MoveX: 0.5, MoveY: 0}),
	})
	if err != nil {
		t.Fatalf("expected late continuous command to be rescheduled, got %v", err)
	}
	if result.ScheduledTick != 3 {
		t.Fatalf("expected late continuous command to schedule to tick 3, got %d", result.ScheduledTick)
	}
}

func TestSubmitInputRateLimitPerTickWindow(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	for seq := uint64(1); seq <= 8; seq++ {
		if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
			PlayerID:  "p1",
			ClientSeq: seq,
			Type:      model.CmdReload,
		}); err != nil {
			t.Fatalf("unexpected rate-limit error before threshold at seq %d: %v", seq, err)
		}
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 9,
		Type:      model.CmdReload,
	})
	if !errors.Is(err, ErrInputRateLimited) {
		t.Fatalf("expected ErrInputRateLimited at seq 9, got %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected test ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 10,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("expected rate window reset after tick advance, got %v", err)
	}
}

func TestSubmitInputDuplicateClientSeqRejected(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("first submit failed: %v", err)
	}

	_, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	})
	if !errors.Is(err, ErrDuplicateInput) {
		t.Fatalf("expected ErrDuplicateInput, got %v", err)
	}
}

func TestConsumeScheduledInputsSortsAndDeletesTickBucket(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "P2"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p2",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("submit p2 failed: %v", err)
	}
	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("submit p1 failed: %v", err)
	}

	commands, err := manager.ConsumeScheduledInputs(match.MatchID, 1)
	if err != nil {
		t.Fatalf("consume tick 1 failed: %v", err)
	}
	if len(commands) != 2 {
		t.Fatalf("expected 2 commands at tick 1, got %d", len(commands))
	}
	if commands[0].PlayerID != "p2" || commands[1].PlayerID != "p1" {
		t.Fatalf("expected deterministic ingress ordering (p2 first), got %#v", commands)
	}

	commands, err = manager.ConsumeScheduledInputs(match.MatchID, 1)
	if err != nil {
		t.Fatalf("second consume tick 1 failed: %v", err)
	}
	if len(commands) != 0 {
		t.Fatalf("expected consumed bucket to be deleted, got %d commands", len(commands))
	}

	counts, err := manager.PendingInputCounts(match.MatchID)
	if err != nil {
		t.Fatalf("pending counts failed: %v", err)
	}
	if counts[1] != 0 {
		t.Fatalf("expected tick 1 count removed after consume, got counts=%#v", counts)
	}
}

func TestPendingInputCountsMatchNotFound(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "q",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	_, err := manager.PendingInputCounts("missing")
	if !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("expected ErrMatchNotFound, got %v", err)
	}
}

func mustRawJSON(t *testing.T, payload any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}
