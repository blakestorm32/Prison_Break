package game

import (
	"encoding/json"
	"testing"
	"time"

	"prison-break/internal/gamecore/combat"
	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestStartMatchAppliesRoleLoadouts(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "cmb-loadout",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "WardenCandidate"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if hearts := playerHeartsForTest(manager, match.MatchID, "p1"); hearts != 10 {
		t.Fatalf("expected fallback single-player warden loadout hearts=10 half, got %d", hearts)
	}
	if bullets := playerBulletsForTest(manager, match.MatchID, "p1"); bullets != 3 {
		t.Fatalf("expected fallback single-player warden bullets=3, got %d", bullets)
	}
}

func TestFireWeaponPistolDamagesAndAppliesUnjustAuthorityPenalty(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "cmb-pen",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "a", "Shooter"); err != nil {
		t.Fatalf("join shooter failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "b", "Target"); err != nil {
		t.Fatalf("join target failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
	setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
	setPlayerHeartsForTest(manager, match.MatchID, "b", 6)
	setPlayerBulletsForTest(manager, match.MatchID, "a", 3)
	setMapPowerForTest(manager, match.MatchID, true)
	setPlayerInventoryForTest(manager, match.MatchID, "b", nil)

	phase := playerPhaseForTest(manager, match.MatchID)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
		Weapon:  model.ItemPistol,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 4 {
		t.Fatalf("expected pistol hit to deal 2 half-hearts (6->4), got %d", hearts)
	}
	if bullets := playerBulletsForTest(manager, match.MatchID, "a"); bullets != 2 {
		t.Fatalf("expected pistol shot to consume one bullet (3->2), got %d", bullets)
	}

	expectedSolitary := phase.EndsTick - 1
	if got := playerSolitaryUntilForTest(manager, match.MatchID, "a"); got != expectedSolitary {
		t.Fatalf("expected unjust authority penalty until %d, got %d", expectedSolitary, got)
	}
	if got := playerLockedInCellForTest(manager, match.MatchID, "a"); got == 0 {
		t.Fatalf("expected unjust authority shooter to be locked in assigned cell")
	}

	positionBefore := playerPositionForTest(manager, match.MatchID, "a")
	mustSubmitMoveIntent(t, manager, match.MatchID, "a", 2, 1, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	positionAfter := playerPositionForTest(manager, match.MatchID, "a")
	if positionAfter != positionBefore {
		t.Fatalf("expected solitary penalty to block movement, before=%+v after=%+v", positionBefore, positionAfter)
	}
}

func TestAuthorityPenaltyExemptCases(t *testing.T) {
	t.Run("restricted_area", func(t *testing.T) {
		manager, _, factory := newTestManager(
			Config{
				MinPlayers:    2,
				MaxPlayers:    4,
				TickRateHz:    30,
				MatchIDPrefix: "cmb-ex-r",
			},
			time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		)
		t.Cleanup(manager.Close)

		match := manager.CreateMatch()
		_, _ = manager.JoinMatch(match.MatchID, "a", "Shooter")
		_, _ = manager.JoinMatch(match.MatchID, "b", "Target")
		_, _ = manager.StartMatch(match.MatchID)

		setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
		setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
		setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomPowerRoom)
		setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomPowerRoom)
		setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 18, Y: 4})
		setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 19, Y: 4})
		setPlayerBulletsForTest(manager, match.MatchID, "a", 3)

		mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
			Weapon:  model.ItemPistol,
			TargetX: 19,
			TargetY: 4,
		})

		ticker := factory.Last()
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
		waitForTick(t, manager, match.MatchID, 1)

		if got := playerSolitaryUntilForTest(manager, match.MatchID, "a"); got != 0 {
			t.Fatalf("expected restricted-area exemption from unjust-shot penalty, got solitary until %d", got)
		}
	})

	t.Run("power_off", func(t *testing.T) {
		manager, _, factory := newTestManager(
			Config{
				MinPlayers:    2,
				MaxPlayers:    4,
				TickRateHz:    30,
				MatchIDPrefix: "cmb-ex-p",
			},
			time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		)
		t.Cleanup(manager.Close)

		match := manager.CreateMatch()
		_, _ = manager.JoinMatch(match.MatchID, "a", "Shooter")
		_, _ = manager.JoinMatch(match.MatchID, "b", "Target")
		_, _ = manager.StartMatch(match.MatchID)

		setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
		setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
		setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
		setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
		setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
		setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
		setPlayerBulletsForTest(manager, match.MatchID, "a", 3)
		setMapPowerForTest(manager, match.MatchID, false)

		mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
			Weapon:  model.ItemPistol,
			TargetX: 5,
			TargetY: 10,
		})

		ticker := factory.Last()
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
		waitForTick(t, manager, match.MatchID, 1)

		if got := playerSolitaryUntilForTest(manager, match.MatchID, "a"); got != 0 {
			t.Fatalf("expected power-off exemption from unjust-shot penalty, got solitary until %d", got)
		}
	})

	t.Run("contraband", func(t *testing.T) {
		manager, _, factory := newTestManager(
			Config{
				MinPlayers:    2,
				MaxPlayers:    4,
				TickRateHz:    30,
				MatchIDPrefix: "cmb-ex-c",
			},
			time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		)
		t.Cleanup(manager.Close)

		match := manager.CreateMatch()
		_, _ = manager.JoinMatch(match.MatchID, "a", "Shooter")
		_, _ = manager.JoinMatch(match.MatchID, "b", "Target")
		_, _ = manager.StartMatch(match.MatchID)

		setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
		setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
		setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
		setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
		setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
		setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
		setPlayerBulletsForTest(manager, match.MatchID, "a", 3)
		setPlayerInventoryForTest(manager, match.MatchID, "b", []model.ItemStack{
			{Item: model.ItemShiv, Quantity: 1},
		})

		mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
			Weapon:  model.ItemPistol,
			TargetX: 5,
			TargetY: 10,
		})

		ticker := factory.Last()
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
		waitForTick(t, manager, match.MatchID, 1)

		if got := playerSolitaryUntilForTest(manager, match.MatchID, "a"); got != 0 {
			t.Fatalf("expected contraband exemption from unjust-shot penalty, got solitary until %d", got)
		}
	})
}

func TestBatonAndShivCombatBehavior(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "cmb-melee",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "a", "Attacker"); err != nil {
		t.Fatalf("join a failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "b", "Target"); err != nil {
		t.Fatalf("join b failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
	setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
	setPlayerHeartsForTest(manager, match.MatchID, "b", 6)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// Non-authority baton attempt should be denied.
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleGangMember, model.FactionPrisoner)
	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
		Weapon:  combat.WeaponBaton,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 6 {
		t.Fatalf("expected prisoner baton attempt to do no damage, got hearts %d", hearts)
	}

	// Authority baton should stun/knockback without heart damage.
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
	positionBefore := playerPositionForTest(manager, match.MatchID, "b")
	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 2, model.FireWeaponPayload{
		Weapon:  combat.WeaponBaton,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	positionAfter := playerPositionForTest(manager, match.MatchID, "b")
	if positionAfter == positionBefore {
		t.Fatalf("expected baton knockback to move target, before=%+v after=%+v", positionBefore, positionAfter)
	}
	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 6 {
		t.Fatalf("expected baton to deal no damage, got hearts %d", hearts)
	}
	expectedStunUntil := uint64(2) + combat.BatonStunDurationTicks(30)
	if stunnedUntil := playerStunnedUntilForTest(manager, match.MatchID, "b"); stunnedUntil != expectedStunUntil {
		t.Fatalf("expected baton stun until %d, got %d", expectedStunUntil, stunnedUntil)
	}

	// Shiv requires inventory and deals half-heart damage.
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleGangMember, model.FactionPrisoner)
	setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
	setPlayerStunnedUntilForTest(manager, match.MatchID, "b", 0)
	setPlayerInventoryForTest(manager, match.MatchID, "a", nil)

	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 3, model.FireWeaponPayload{
		Weapon:  model.ItemShiv,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 6 {
		t.Fatalf("expected shiv without inventory to fail, got hearts %d", hearts)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "a", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
	})
	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 4, model.FireWeaponPayload{
		Weapon:  model.ItemShiv,
		TargetX: 5,
		TargetY: 10,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 5 {
		t.Fatalf("expected shiv to deal 1 half-heart damage (6->5), got %d", hearts)
	}
}

func TestGoldenBulletDamageAndConsumption(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "cmb-gold",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "a", "Shooter"); err != nil {
		t.Fatalf("join a failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "b", "Target"); err != nil {
		t.Fatalf("join b failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "b", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "a", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "b", gamemap.RoomCorridorMain)
	setPlayerPositionForTest(manager, match.MatchID, "a", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "b", model.Vector2{X: 5, Y: 10})
	setPlayerHeartsForTest(manager, match.MatchID, "b", 6)
	setPlayerBulletsForTest(manager, match.MatchID, "a", 3)
	setPlayerInventoryForTest(manager, match.MatchID, "a", []model.ItemStack{
		{Item: model.ItemGoldenBullet, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitFireWeapon(t, manager, match.MatchID, "a", 1, model.FireWeaponPayload{
		Weapon:         model.ItemPistol,
		TargetX:        5,
		TargetY:        10,
		UseGoldenRound: true,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if hearts := playerHeartsForTest(manager, match.MatchID, "b"); hearts != 2 {
		t.Fatalf("expected golden bullet to deal 4 half-hearts (6->2), got %d", hearts)
	}
	if bullets := playerBulletsForTest(manager, match.MatchID, "a"); bullets != 3 {
		t.Fatalf("expected golden bullet to not consume standard bullet ammo, got %d", bullets)
	}
	inventory := playerInventoryForTest(manager, match.MatchID, "a")
	if items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemGoldenBullet, 1) {
		t.Fatalf("expected golden bullet inventory to be consumed")
	}
}

func mustSubmitFireWeapon(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	payload model.FireWeaponPayload,
) {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal fire payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdFireWeapon,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit fire input failed: %v", err)
	}
}

func setPlayerHeartsForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	heartsHalf uint8,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID != playerID {
			continue
		}
		session.gameState.Players[index].HeartsHalf = heartsHalf
		session.gameState.Players[index].Alive = heartsHalf > 0
		return
	}
}

func setPlayerBulletsForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	bullets uint8,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID != playerID {
			continue
		}
		session.gameState.Players[index].Bullets = bullets
		return
	}
}

func playerHeartsForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) uint8 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.HeartsHalf
		}
	}
	return 0
}

func playerBulletsForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) uint8 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.Bullets
		}
	}
	return 0
}

func playerSolitaryUntilForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) uint64 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.SolitaryUntilTick
		}
	}
	return 0
}

func playerLockedInCellForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.CellID {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.LockedInCell
		}
	}
	return 0
}

func playerStunnedUntilForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) uint64 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.StunnedUntilTick
		}
	}
	return 0
}
