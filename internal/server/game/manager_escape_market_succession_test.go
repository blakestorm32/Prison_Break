package game

import (
	"testing"
	"time"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/gamecore/winconditions"
	"prison-break/internal/shared/model"
)

func TestEscapeRoutesRequireExpectedConditions(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "esc-routes",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Prisoner"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Other"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p2", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCellBlockA)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	stepTick := uint64(1)
	tickAt := func() {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(stepTick), 0, time.UTC))
		waitForTick(t, manager, match.MatchID, stepTick)
		stepTick++
	}

	// Courtyard dig requires a shovel.
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCourtyard)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", nil)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCourtyard {
		t.Fatalf("expected courtyard dig attempt without shovel to fail, got room %s", room)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemShovel, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		EscapeRoute: model.EscapeRouteCourtyardDig,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected courtyard dig attempt with shovel to succeed, got room %s", room)
	}

	// Badge escape requires the main corridor and a badge.
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomMailRoom)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemBadge, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 3, model.InteractPayload{
		EscapeRoute: model.EscapeRouteBadgeEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomMailRoom {
		t.Fatalf("expected badge escape attempt outside corridor to fail, got room %s", room)
	}

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 4, model.InteractPayload{
		EscapeRoute: model.EscapeRouteBadgeEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected badge escape attempt in corridor with badge to succeed, got room %s", room)
	}

	// Power-out escape requires power room + power off.
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomPowerRoom)
	setMapPowerForTest(manager, match.MatchID, true)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 5, model.InteractPayload{
		EscapeRoute: model.EscapeRoutePowerOutEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomPowerRoom {
		t.Fatalf("expected power-out escape attempt while power on to fail, got room %s", room)
	}

	setMapPowerForTest(manager, match.MatchID, false)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 6, model.InteractPayload{
		EscapeRoute: model.EscapeRoutePowerOutEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected power-out escape attempt while power off to succeed, got room %s", room)
	}

	// Ladder escape requires two ladders.
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCourtyard)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemLadder, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 7, model.InteractPayload{
		EscapeRoute: model.EscapeRouteLadderEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCourtyard {
		t.Fatalf("expected ladder escape with one ladder to fail, got room %s", room)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemLadder, Quantity: 2},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 8, model.InteractPayload{
		EscapeRoute: model.EscapeRouteLadderEscape,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected ladder escape with two ladders to succeed, got room %s", room)
	}

	// Roof/helicopter escape requires keys.
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomRoofLookout)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", nil)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 9, model.InteractPayload{
		EscapeRoute: model.EscapeRouteRoofHelicopter,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomRoofLookout {
		t.Fatalf("expected roof/helicopter escape without keys to fail, got room %s", room)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemKeys, Quantity: 1},
	})
	mustSubmitInteract(t, manager, match.MatchID, "p1", 10, model.InteractPayload{
		EscapeRoute: model.EscapeRouteRoofHelicopter,
	})
	tickAt()
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected roof/helicopter escape with keys to succeed, got room %s", room)
	}
}

func TestGangLeaderEscapeRouteTriggersGameOver(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "esc-win",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "leader", "Leader"); err != nil {
		t.Fatalf("join leader failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "leader", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoomForTest(manager, match.MatchID, "leader", gamemap.RoomRoofLookout)
	setPlayerInventoryForTest(manager, match.MatchID, "leader", []model.ItemStack{
		{Item: model.ItemKeys, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "leader", 1, model.InteractPayload{
		EscapeRoute: model.EscapeRouteRoofHelicopter,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	snapshot, ok := manager.MatchSnapshot(match.MatchID)
	if !ok {
		t.Fatalf("expected match snapshot")
	}
	if snapshot.Status != model.MatchStatusGameOver {
		t.Fatalf("expected match to end after gang leader escape, got status %s", snapshot.Status)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil || full.State.GameOver == nil {
		t.Fatalf("expected game-over state in full snapshot")
	}
	if full.State.GameOver.Reason != model.WinReasonGangLeaderEscaped {
		t.Fatalf("expected reason gang_leader_escaped, got %s", full.State.GameOver.Reason)
	}
	if room := playerRoomForTest(manager, match.MatchID, "leader"); room != winconditions.EscapedRoomID {
		t.Fatalf("expected leader room to be escaped, got %s", room)
	}
}

func TestNightlyBlackMarketFlowAndGangLeaderSet(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    1,
			DaySeconds:    2,
			NightSeconds:  2,
			MaxCycles:     6,
			MatchIDPrefix: "market",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "leader", "Leader"); err != nil {
		t.Fatalf("join leader failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "other", "Other"); err != nil {
		t.Fatalf("join other failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "leader", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "other", model.RoleGangMember, model.FactionPrisoner)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	expectedNightRoom := gamemap.DeterministicNightlyBlackMarketRoom(match.MatchID, 0, 3)
	if got := mapBlackMarketRoomForTest(manager, match.MatchID); got != expectedNightRoom {
		t.Fatalf("expected nightly market room %s on night start, got %s", expectedNightRoom, got)
	}

	mustSubmitInteract(t, manager, match.MatchID, "leader", 1, model.InteractPayload{
		MarketRoomID: gamemap.RoomCourtyard,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	if got := mapBlackMarketRoomForTest(manager, match.MatchID); got != gamemap.RoomCourtyard {
		t.Fatalf("expected gang leader to set market room to courtyard, got %s", got)
	}

	mustSubmitInteract(t, manager, match.MatchID, "other", 1, model.InteractPayload{
		MarketRoomID: gamemap.RoomMailRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 5, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 5)
	if got := mapBlackMarketRoomForTest(manager, match.MatchID); got != gamemap.RoomCourtyard {
		t.Fatalf("expected non-leader market set attempt to be denied, got %s", got)
	}

	mustSubmitInteract(t, manager, match.MatchID, "leader", 2, model.InteractPayload{
		MarketRoomID: gamemap.RoomRoofLookout,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 6, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 6)
	if got := mapBlackMarketRoomForTest(manager, match.MatchID); got != gamemap.RoomCourtyard {
		t.Fatalf("expected gang leader market set attempt during day to be denied, got %s", got)
	}
}

func TestGangLeaderSuccessionPromotesAliveGangMember(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    4,
			MaxPlayers:    6,
			TickRateHz:    10,
			MatchIDPrefix: "succ",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	for _, playerID := range []model.PlayerID{"leader", "g1", "g2", "auth"} {
		if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
			t.Fatalf("join %s failed: %v", playerID, err)
		}
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "leader", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "g1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "g2", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerAliveForTest(manager, match.MatchID, "leader", false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	snapshot, ok := manager.MatchSnapshot(match.MatchID)
	if !ok {
		t.Fatalf("expected match snapshot")
	}
	if snapshot.Status != model.MatchStatusRunning {
		t.Fatalf("expected match to remain running after succession, got %s", snapshot.Status)
	}

	if role := playerRoleForTest(manager, match.MatchID, "leader"); role != model.RoleGangMember {
		t.Fatalf("expected dead prior leader to be demoted to gang_member, got %s", role)
	}

	aliveLeaders := make([]model.PlayerID, 0, 2)
	for _, candidate := range []model.PlayerID{"g1", "g2"} {
		role := playerRoleForTest(manager, match.MatchID, candidate)
		alive := playerAliveForTest(manager, match.MatchID, candidate)
		if role == model.RoleGangLeader && alive {
			aliveLeaders = append(aliveLeaders, candidate)
		}
	}
	if len(aliveLeaders) != 1 {
		t.Fatalf("expected exactly one living promoted gang leader, got %v", aliveLeaders)
	}
}

func mapBlackMarketRoomForTest(manager *Manager, matchID model.MatchID) model.RoomID {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.matches[matchID].gameState.Map.BlackMarketRoomID
}

func playerRoleForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.RoleType {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	for _, player := range manager.matches[matchID].gameState.Players {
		if player.ID == playerID {
			return player.Role
		}
	}
	return ""
}

func playerAliveForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	for _, player := range manager.matches[matchID].gameState.Players {
		if player.ID == playerID {
			return player.Alive
		}
	}
	return false
}
