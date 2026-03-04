package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"prison-break/internal/engine/physics"
	"prison-break/internal/gamecore/abilities"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestCreateMatchInitializesLobbySnapshot(t *testing.T) {
	manager, clock, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "it",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	snapshot := manager.CreateMatch()

	if snapshot.MatchID != "it-000001" {
		t.Fatalf("unexpected match id: got=%s want=%s", snapshot.MatchID, "it-000001")
	}
	if snapshot.Status != model.MatchStatusLobby {
		t.Fatalf("unexpected status: got=%s want=%s", snapshot.Status, model.MatchStatusLobby)
	}
	if snapshot.TickID != 0 {
		t.Fatalf("unexpected tick id: got=%d want=0", snapshot.TickID)
	}
	if len(snapshot.Players) != 0 {
		t.Fatalf("expected empty player list on new match, got %d players", len(snapshot.Players))
	}
	if !snapshot.CreatedAt.Equal(clock.Now()) {
		t.Fatalf("unexpected created_at: got=%s want=%s", snapshot.CreatedAt, clock.Now())
	}

	events := manager.LifecycleEvents(snapshot.MatchID)
	if len(events) != 1 {
		t.Fatalf("expected 1 lifecycle event after create, got %d", len(events))
	}
	if events[0].Type != LifecycleEventMatchCreated {
		t.Fatalf("unexpected first event type: got=%s want=%s", events[0].Type, LifecycleEventMatchCreated)
	}
}

func TestCreateMatchSeedsDefaultMapTopology(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "map",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}

	stateMap := full.State.Map
	if !stateMap.PowerOn {
		t.Fatalf("expected map power to start on")
	}
	if stateMap.Alarm.Active {
		t.Fatalf("expected map alarm to start inactive")
	}
	if stateMap.BlackMarketRoomID != gamemap.RoomBlackMarket {
		t.Fatalf(
			"expected black market room id %s, got %s",
			gamemap.RoomBlackMarket,
			stateMap.BlackMarketRoomID,
		)
	}
	if len(stateMap.Doors) != 22 {
		t.Fatalf("expected 22 seeded doors, got %d", len(stateMap.Doors))
	}
	if len(stateMap.Cells) != 12 {
		t.Fatalf("expected 12 seeded cells, got %d", len(stateMap.Cells))
	}
	if len(stateMap.RestrictedZones) != 4 {
		t.Fatalf("expected 4 seeded restricted zones, got %d", len(stateMap.RestrictedZones))
	}
}

func TestStartMatchAssignsDeterministicCellOwnership(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    30,
			MatchIDPrefix: "cell",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p3", "C"); err != nil {
		t.Fatalf("join p3 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p1", "A"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "B"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}

	assigned := make(map[model.CellID]model.PlayerID, len(full.State.Players))
	for _, player := range full.State.Players {
		if player.AssignedCell == 0 {
			t.Fatalf("expected assigned cell for player %s", player.ID)
		}
		expectedRoom := gamemap.RoomCellBlockA
		switch player.Role {
		case model.RoleWarden:
			expectedRoom = gamemap.RoomWardenHQ
		case model.RoleDeputy:
			expectedRoom = gamemap.RoomAmmoRoom
		}
		if player.CurrentRoomID != expectedRoom {
			t.Fatalf("expected player %s role %s start room %s, got %s", player.ID, player.Role, expectedRoom, player.CurrentRoomID)
		}
		if _, duplicate := assigned[player.AssignedCell]; duplicate {
			t.Fatalf("duplicate assigned cell %d", player.AssignedCell)
		}
		assigned[player.AssignedCell] = player.ID
	}

	for _, cell := range full.State.Map.Cells {
		owner, tracked := assigned[cell.ID]
		if !tracked {
			continue
		}
		if cell.OwnerPlayerID != owner {
			t.Fatalf("expected cell %d owner %s, got %s", cell.ID, owner, cell.OwnerPlayerID)
		}
	}
}

func TestStartMatchAssignsRolesForSixPlayers(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    6,
			MaxPlayers:    12,
			TickRateHz:    30,
			DaySeconds:    300,
			NightSeconds:  120,
			MaxCycles:     6,
			MatchIDPrefix: "roles",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	for idx := 1; idx <= 6; idx++ {
		playerID := model.PlayerID(fmt.Sprintf("p%d", idx))
		if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
			t.Fatalf("join %s failed: %v", playerID, err)
		}
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}

	var wardenCount int
	var gangLeaderCount int
	for _, player := range full.State.Players {
		if player.Role == "" || player.Faction == "" || player.Alignment == "" {
			t.Fatalf("expected role fields populated for player %+v", player)
		}
		if player.Role == model.RoleWarden {
			wardenCount++
		}
		if player.Role == model.RoleGangLeader {
			gangLeaderCount++
		}
	}
	if wardenCount != 1 {
		t.Fatalf("expected one warden, got %d", wardenCount)
	}
	if gangLeaderCount != 1 {
		t.Fatalf("expected one gang leader, got %d", gangLeaderCount)
	}
}

func TestStartMatchAssignsDeterministicAbilityPerPlayer(t *testing.T) {
	newManagerAndMatch := func() (*Manager, model.MatchID) {
		manager, _, _ := newTestManager(
			Config{
				MinPlayers:    4,
				MaxPlayers:    6,
				TickRateHz:    30,
				MatchIDPrefix: "ability",
			},
			time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
		)

		match := manager.CreateMatch()
		for _, playerID := range []model.PlayerID{"p1", "p2", "p3", "p4"} {
			if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
				t.Fatalf("join %s failed: %v", playerID, err)
			}
		}
		if _, err := manager.StartMatch(match.MatchID); err != nil {
			t.Fatalf("start failed: %v", err)
		}
		return manager, match.MatchID
	}

	managerA, matchA := newManagerAndMatch()
	t.Cleanup(managerA.Close)
	managerB, matchB := newManagerAndMatch()
	t.Cleanup(managerB.Close)

	fullA, err := managerA.FullSnapshot(matchA)
	if err != nil {
		t.Fatalf("snapshot A failed: %v", err)
	}
	fullB, err := managerB.FullSnapshot(matchB)
	if err != nil {
		t.Fatalf("snapshot B failed: %v", err)
	}
	if fullA.State == nil || fullB.State == nil {
		t.Fatalf("expected both snapshots to include full state")
	}

	assignedA := make(map[model.PlayerID]model.AbilityType, len(fullA.State.Players))
	for _, player := range fullA.State.Players {
		if player.AssignedAbility == "" {
			t.Fatalf("expected player %s to have assigned ability", player.ID)
		}
		if !abilities.CanPlayerUse(player, player.AssignedAbility) {
			t.Fatalf("expected assigned ability %s to be valid for player %+v", player.AssignedAbility, player)
		}
		assignedA[player.ID] = player.AssignedAbility
	}

	for _, player := range fullB.State.Players {
		expected, exists := assignedA[player.ID]
		if !exists {
			t.Fatalf("expected player %s in deterministic comparison set", player.ID)
		}
		if player.AssignedAbility != expected {
			t.Fatalf("expected deterministic assigned ability for player %s to be %s, got %s", player.ID, expected, player.AssignedAbility)
		}
	}
}

func TestInteractCellDoorOwnerAndAuthorityPrivileges(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    30,
			MatchIDPrefix: "door",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "owner", "Owner"); err != nil {
		t.Fatalf("join owner failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "other", "Other"); err != nil {
		t.Fatalf("join other failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "owner", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "other", model.RoleNeutralPrisoner, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)

	ownerCell := playerAssignedCellForTest(manager, match.MatchID, "owner")
	ownerDoor := doorIDForCellForTest(manager, match.MatchID, ownerCell)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// Non-owner prisoner cannot operate another prisoner's cell door.
	mustSubmitInteract(t, manager, match.MatchID, "other", 1, model.InteractPayload{
		TargetCellID: ownerCell,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	if doorOpenForTest(manager, match.MatchID, ownerDoor) {
		t.Fatalf("expected non-owner prisoner action to be denied")
	}

	// Cell owner can operate the cell door.
	mustSubmitInteract(t, manager, match.MatchID, "owner", 1, model.InteractPayload{
		TargetDoorID: ownerDoor,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if !doorOpenForTest(manager, match.MatchID, ownerDoor) {
		t.Fatalf("expected owner to open their cell door")
	}

	// Non-owner prisoner still cannot bypass by targeting door directly.
	mustSubmitInteract(t, manager, match.MatchID, "other", 2, model.InteractPayload{
		TargetDoorID: ownerDoor,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if !doorOpenForTest(manager, match.MatchID, ownerDoor) {
		t.Fatalf("expected non-owner direct door target to remain denied")
	}

	// Authority can operate any cell door.
	mustSubmitInteract(t, manager, match.MatchID, "auth", 1, model.InteractPayload{
		TargetDoorID: ownerDoor,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	if doorOpenForTest(manager, match.MatchID, ownerDoor) {
		t.Fatalf("expected authority to close owner cell door")
	}
}

func TestInteractRoomAccessRestrictions(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    30,
			MatchIDPrefix: "room",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "deputy", "Deputy"); err != nil {
		t.Fatalf("join deputy failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "prisoner", "Prisoner"); err != nil {
		t.Fatalf("join prisoner failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "deputy", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "prisoner", model.RoleGangMember, model.FactionPrisoner)

	setPlayerRoomForTest(manager, match.MatchID, "warden", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "deputy", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "prisoner", gamemap.RoomCorridorMain)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "prisoner", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomWardenHQ,
	})
	mustSubmitInteract(t, manager, match.MatchID, "warden", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomWardenHQ,
	})
	mustSubmitInteract(t, manager, match.MatchID, "prisoner", 2, model.InteractPayload{
		TargetRoomID: gamemap.RoomCameraRoom,
	})
	mustSubmitInteract(t, manager, match.MatchID, "deputy", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomCameraRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if room := playerRoomForTest(manager, match.MatchID, "prisoner"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected prisoner blocked from warden/camera rooms, got %s", room)
	}
	if room := playerRoomForTest(manager, match.MatchID, "warden"); room != gamemap.RoomWardenHQ {
		t.Fatalf("expected warden allowed into warden_hq, got %s", room)
	}
	if room := playerRoomForTest(manager, match.MatchID, "deputy"); room != gamemap.RoomCameraRoom {
		t.Fatalf("expected deputy allowed into camera room while power on, got %s", room)
	}

	setMapPowerForTest(manager, match.MatchID, false)
	setPlayerRoomForTest(manager, match.MatchID, "deputy", gamemap.RoomCorridorMain)

	mustSubmitInteract(t, manager, match.MatchID, "deputy", 2, model.InteractPayload{
		TargetRoomID: gamemap.RoomCameraRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if room := playerRoomForTest(manager, match.MatchID, "deputy"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected deputy blocked from camera room when power off, got %s", room)
	}

	setMapPowerForTest(manager, match.MatchID, true)
	setPlayerRoomForTest(manager, match.MatchID, "deputy", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "prisoner", gamemap.RoomCorridorMain)

	mustSubmitInteract(t, manager, match.MatchID, "deputy", 3, model.InteractPayload{
		TargetRoomID: gamemap.RoomBlackMarket,
	})
	mustSubmitInteract(t, manager, match.MatchID, "prisoner", 3, model.InteractPayload{
		TargetRoomID: gamemap.RoomBlackMarket,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	if room := playerRoomForTest(manager, match.MatchID, "deputy"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected authority blocked from black market, got %s", room)
	}
	if room := playerRoomForTest(manager, match.MatchID, "prisoner"); room != gamemap.RoomBlackMarket {
		t.Fatalf("expected prisoner allowed into black market, got %s", room)
	}
}

func TestMoveIntentCollisionWithClosedDoorThenOpenDoor(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "mv-door",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Mover"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	startPosition, moveX, moveY := approachDoorForManagerTest(t, 7, gamemap.RoomCorridorMain)
	setPlayerPositionForTest(manager, match.MatchID, "p1", startPosition)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setDoorOpenForTest(manager, match.MatchID, 7, false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 1, moveX, moveY, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	closedPosition := playerPositionForTest(manager, match.MatchID, "p1")
	if closedPosition != startPosition {
		t.Fatalf("expected closed door to block movement, got %+v", closedPosition)
	}

	setDoorOpenForTest(manager, match.MatchID, 7, true)
	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 2, moveX, moveY, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	openPosition := playerPositionForTest(manager, match.MatchID, "p1")
	if openPosition == startPosition {
		t.Fatalf("expected open door to allow movement through doorway, got %+v", openPosition)
	}
}

func TestMoveIntentRespectsRoomAccessRestrictions(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "mv-access",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Mover"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	startPosition, moveX, moveY := approachDoorForManagerTest(t, 1, gamemap.RoomCorridorMain)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerPositionForTest(manager, match.MatchID, "p1", startPosition)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setDoorOpenForTest(manager, match.MatchID, 1, true)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 1, moveX, moveY, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	blockedPosition := playerPositionForTest(manager, match.MatchID, "p1")
	if blockedPosition != startPosition {
		t.Fatalf("expected non-warden entry into warden_hq to be blocked, got %+v", blockedPosition)
	}
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected room to remain corridor_main after denied entry, got %s", room)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleWarden, model.FactionAuthority)
	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 2, moveX, moveY, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	allowedPosition := playerPositionForTest(manager, match.MatchID, "p1")
	if allowedPosition == startPosition {
		t.Fatalf("expected warden to enter warden_hq, got %+v", allowedPosition)
	}
	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomWardenHQ {
		t.Fatalf("expected room transition into warden_hq, got %s", room)
	}
}

func TestMoveIntentBlockedByPlayerAndStunPreventsMotion(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "mv-block",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Mover"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Blocker"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerPositionForTest(manager, match.MatchID, "p2", model.Vector2{X: 4, Y: 14})
	startPosition := edgeApproachPositionForManagerStep(gamemap.Point{X: 3, Y: 14}, 1, 0, physics.BaseMoveStepPerTick)
	setPlayerPositionForTest(
		manager,
		match.MatchID,
		"p1",
		startPosition,
	)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCellBlockA)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCellBlockA)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 1, 1, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if position := playerPositionForTest(manager, match.MatchID, "p1"); position != startPosition {
		t.Fatalf("expected occupied tile collision to block movement, got %+v", position)
	}

	setPlayerPositionForTest(manager, match.MatchID, "p2", model.Vector2{X: 6, Y: 14})
	setPlayerStunnedUntilForTest(manager, match.MatchID, "p1", 3)

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 2, 1, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if position := playerPositionForTest(manager, match.MatchID, "p1"); position != startPosition {
		t.Fatalf("expected stunned player to remain in place, got %+v", position)
	}
	if velocity := playerVelocityForTest(manager, match.MatchID, "p1"); velocity != (model.Vector2{}) {
		t.Fatalf("expected stunned movement velocity reset, got %+v", velocity)
	}
}

func TestApplyKnockbackAppliesDisplacementAndStun(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "kb",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Knockback"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerPositionForTest(manager, match.MatchID, "p1", model.Vector2{X: 3, Y: 14})

	player, err := manager.ApplyKnockback(match.MatchID, "p1", model.Vector2{X: 1, Y: 0}, 3)
	if err != nil {
		t.Fatalf("apply knockback failed: %v", err)
	}
	if player.Position.X <= 3 {
		t.Fatalf("expected knockback to move player, got %+v", player.Position)
	}
	if player.StunnedUntilTick != 3 {
		t.Fatalf("expected stunned until tick 3, got %d", player.StunnedUntilTick)
	}

	cellBlock, exists := defaultMatchLayout.Room(gamemap.RoomCellBlockA)
	if !exists {
		t.Fatalf("expected cell block room in default layout")
	}
	wallStart := model.Vector2{X: float32(cellBlock.Min.X), Y: float32(cellBlock.Min.Y + 1)}
	setPlayerPositionForTest(manager, match.MatchID, "p1", wallStart)
	player, err = manager.ApplyKnockback(match.MatchID, "p1", model.Vector2{X: -1, Y: 0}, 1)
	if err != nil {
		t.Fatalf("apply wall knockback failed: %v", err)
	}
	if player.Position != wallStart {
		t.Fatalf("expected wall collision to block knockback, got %+v", player.Position)
	}
	if player.StunnedUntilTick != 3 {
		t.Fatalf("expected previous longer stun duration to remain, got %d", player.StunnedUntilTick)
	}

	if _, err := manager.ApplyKnockback(match.MatchID, "missing", model.Vector2{X: 1, Y: 0}, 1); !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("expected ErrPlayerNotFound, got %v", err)
	}
}

func TestPhaseTransitionsUseConfiguredDurationsAndDayHookReset(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    1,
			NightSeconds:  1,
			MaxCycles:     6,
			MatchIDPrefix: "phase",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "PhasePlayer"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}
	if full.State.Phase.Current != model.PhaseDay {
		t.Fatalf("expected initial phase day, got %s", full.State.Phase.Current)
	}
	if full.State.Phase.StartedTick != 1 || full.State.Phase.EndsTick != 3 {
		t.Fatalf("expected day window [1,3), got %+v", full.State.Phase)
	}

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

	nightPhase := playerPhaseForTest(manager, match.MatchID)
	if nightPhase.Current != model.PhaseNight {
		t.Fatalf("expected night phase at tick 3, got %+v", nightPhase)
	}
	if nightPhase.StartedTick != 3 || nightPhase.EndsTick != 5 {
		t.Fatalf("expected night window [3,5), got %+v", nightPhase)
	}

	setMapAlarmForTest(manager, match.MatchID, model.AlarmState{
		Active:      true,
		EndsTick:    100,
		TriggeredBy: "p1",
	})

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 5, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 5)

	dayPhase := playerPhaseForTest(manager, match.MatchID)
	if dayPhase.Current != model.PhaseDay {
		t.Fatalf("expected day phase at tick 5, got %+v", dayPhase)
	}
	if cycle := cycleCountForTest(manager, match.MatchID); cycle != 1 {
		t.Fatalf("expected cycle count to increment to 1, got %d", cycle)
	}
	alarm := mapAlarmForTest(manager, match.MatchID)
	if alarm.Active || alarm.TriggeredBy != "" || alarm.EndsTick != 0 {
		t.Fatalf("expected day-start hook to reset alarm, got %+v", alarm)
	}
}

func TestPhaseCycleCountSaturatesAtConfiguredMaximum(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    1,
			DaySeconds:    1,
			NightSeconds:  1,
			MaxCycles:     6,
			MatchIDPrefix: "phase-cap",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "PhaseCap"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// With 1-second day/night at 1Hz, cycle reaches 6 on tick 13 then game ends.
	for tick := 1; tick <= 13; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, tick, 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, 13)

	if cycle := cycleCountForTest(manager, match.MatchID); cycle != 6 {
		t.Fatalf("expected cycle count saturation at 6, got %d", cycle)
	}
	if snapshot, exists := manager.MatchSnapshot(match.MatchID); !exists || snapshot.Status != model.MatchStatusGameOver {
		t.Fatalf("expected match to end at cycle cap, got snapshot=%+v exists=%v", snapshot, exists)
	}
}

func TestPhaseTransitionsAreIncludedInDeltaSnapshots(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    1,
			NightSeconds:  1,
			MaxCycles:     6,
			MatchIDPrefix: "phase-delta",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "PhaseDelta"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	for tick := 1; tick <= 5; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, tick, 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, 5)

	snapshots, err := manager.SnapshotsSince(match.MatchID, 0)
	if err != nil {
		t.Fatalf("snapshots since failed: %v", err)
	}

	var sawNightTransition bool
	var sawDayTransition bool
	for _, snapshot := range snapshots {
		if snapshot.Delta == nil || snapshot.Delta.Phase == nil {
			continue
		}

		if snapshot.TickID == 3 && snapshot.Delta.Phase.Current == model.PhaseNight {
			sawNightTransition = true
		}
		if snapshot.TickID == 5 && snapshot.Delta.Phase.Current == model.PhaseDay {
			sawDayTransition = true
			if snapshot.Delta.CycleCount == nil || *snapshot.Delta.CycleCount != 1 {
				t.Fatalf("expected cycle_count=1 at day transition tick 5, got %+v", snapshot.Delta.CycleCount)
			}
		}
	}

	if !sawNightTransition {
		t.Fatalf("expected delta phase transition to night at tick 3")
	}
	if !sawDayTransition {
		t.Fatalf("expected delta phase transition to day at tick 5")
	}
}

func TestWinConditionAutoEndMaxCyclesReached(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    1,
			DaySeconds:    1,
			NightSeconds:  1,
			MaxCycles:     1,
			MatchIDPrefix: "win-cycle",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Cycle"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleWarden, model.FactionAuthority)
	setPlayerAlignmentForTest(manager, match.MatchID, "p1", model.AlignmentGood)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	for tick := 1; tick <= 3; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, tick, 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, 3)

	snapshot, exists := manager.MatchSnapshot(match.MatchID)
	if !exists {
		t.Fatalf("expected match snapshot")
	}
	if snapshot.Status != model.MatchStatusGameOver {
		t.Fatalf("expected game_over status, got %s", snapshot.Status)
	}
	if snapshot.EndedReason != string(model.WinReasonMaxCyclesReached) {
		t.Fatalf("expected ended reason %s, got %s", model.WinReasonMaxCyclesReached, snapshot.EndedReason)
	}

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil || full.State.GameOver == nil {
		t.Fatalf("expected game over state in full snapshot")
	}
	if full.State.GameOver.Reason != model.WinReasonMaxCyclesReached {
		t.Fatalf("expected max_cycles_reached game over reason, got %s", full.State.GameOver.Reason)
	}
	if !reflect.DeepEqual(full.State.GameOver.WinnerPlayerIDs, []model.PlayerID{"p1"}) {
		t.Fatalf("expected p1 winner, got %v", full.State.GameOver.WinnerPlayerIDs)
	}
}

func TestWinConditionAutoEndWardenDiedAndGangWinners(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    30,
			DaySeconds:    300,
			NightSeconds:  120,
			MaxCycles:     6,
			MatchIDPrefix: "win-warden",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "gang", "Gang"); err != nil {
		t.Fatalf("join gang failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "gang2", "Gang2"); err != nil {
		t.Fatalf("join gang2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerAlignmentForTest(manager, match.MatchID, "warden", model.AlignmentGood)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "gang", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerAlignmentForTest(manager, match.MatchID, "gang", model.AlignmentEvil)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "gang2", model.RoleGangMember, model.FactionPrisoner)
	setPlayerAlignmentForTest(manager, match.MatchID, "gang2", model.AlignmentEvil)
	setPlayerAliveForTest(manager, match.MatchID, "warden", false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil || full.State.GameOver == nil {
		t.Fatalf("expected game over state")
	}
	if full.State.GameOver.Reason != model.WinReasonWardenDied {
		t.Fatalf("expected reason warden_died, got %s", full.State.GameOver.Reason)
	}
	if !reflect.DeepEqual(full.State.GameOver.WinnerPlayerIDs, []model.PlayerID{"gang", "gang2"}) {
		t.Fatalf("expected gang winners, got %v", full.State.GameOver.WinnerPlayerIDs)
	}
}

func TestWinConditionAutoEndAllGangMembersDeadAndAuthorityWinners(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    4,
			MaxPlayers:    8,
			TickRateHz:    30,
			DaySeconds:    300,
			NightSeconds:  120,
			MaxCycles:     6,
			MatchIDPrefix: "win-gang",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	for _, playerID := range []model.PlayerID{"warden", "deputy", "snitch", "gang"} {
		if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
			t.Fatalf("join %s failed: %v", playerID, err)
		}
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerAlignmentForTest(manager, match.MatchID, "warden", model.AlignmentGood)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "deputy", model.RoleDeputy, model.FactionAuthority)
	setPlayerAlignmentForTest(manager, match.MatchID, "deputy", model.AlignmentGood)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "snitch", model.RoleSnitch, model.FactionPrisoner)
	setPlayerAlignmentForTest(manager, match.MatchID, "snitch", model.AlignmentGood)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "gang", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerAlignmentForTest(manager, match.MatchID, "gang", model.AlignmentEvil)
	setPlayerAliveForTest(manager, match.MatchID, "gang", false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil || full.State.GameOver == nil {
		t.Fatalf("expected game over state")
	}
	if full.State.GameOver.Reason != model.WinReasonAllGangMembersDead {
		t.Fatalf("expected reason all_gang_members_dead, got %s", full.State.GameOver.Reason)
	}
	if !reflect.DeepEqual(full.State.GameOver.WinnerPlayerIDs, []model.PlayerID{"deputy", "snitch", "warden"}) {
		t.Fatalf("expected authority/snitch winners, got %v", full.State.GameOver.WinnerPlayerIDs)
	}
}

func TestWinConditionPriorityGangLeaderEscapedOverWardenDied(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    6,
			TickRateHz:    30,
			DaySeconds:    300,
			NightSeconds:  120,
			MaxCycles:     6,
			MatchIDPrefix: "win-pri",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	for _, playerID := range []model.PlayerID{"warden", "gang"} {
		if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
			t.Fatalf("join %s failed: %v", playerID, err)
		}
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerAlignmentForTest(manager, match.MatchID, "warden", model.AlignmentGood)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "gang", model.RoleGangLeader, model.FactionPrisoner)
	setPlayerAlignmentForTest(manager, match.MatchID, "gang", model.AlignmentEvil)
	setPlayerAliveForTest(manager, match.MatchID, "warden", false)
	setPlayerRoomForTest(manager, match.MatchID, "gang", "escaped")

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil || full.State.GameOver == nil {
		t.Fatalf("expected game over state")
	}
	if full.State.GameOver.Reason != model.WinReasonGangLeaderEscaped {
		t.Fatalf("expected gang_leader_escaped priority, got %s", full.State.GameOver.Reason)
	}
}

func TestJoinMatchValidationAndCapacityEdgeCases(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    2,
			TickRateHz:    30,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()

	if _, err := manager.JoinMatch(match.MatchID, "", "Alice"); !errors.Is(err, ErrInvalidPlayerID) {
		t.Fatalf("expected ErrInvalidPlayerID, got %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p1", "   "); !errors.Is(err, ErrInvalidPlayerName) {
		t.Fatalf("expected ErrInvalidPlayerName, got %v", err)
	}
	if _, err := manager.JoinMatch("missing", "p1", "Alice"); !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("expected ErrMatchNotFound, got %v", err)
	}

	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("unexpected error joining p1: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice-again"); !errors.Is(err, ErrPlayerAlreadyInMatch) {
		t.Fatalf("expected ErrPlayerAlreadyInMatch, got %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Bob"); err != nil {
		t.Fatalf("unexpected error joining p2: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p3", "Charlie"); !errors.Is(err, ErrMatchFull) {
		t.Fatalf("expected ErrMatchFull, got %v", err)
	}

	snapshot, ok := manager.MatchSnapshot(match.MatchID)
	if !ok {
		t.Fatalf("expected match snapshot to exist")
	}
	if len(snapshot.Players) != 2 {
		t.Fatalf("expected 2 players in snapshot, got %d", len(snapshot.Players))
	}
	if snapshot.Players[0].PlayerID != "p1" || snapshot.Players[1].PlayerID != "p2" {
		t.Fatalf("expected deterministic player ordering by player_id, got %#v", snapshot.Players)
	}
}

func TestStartMatchValidationEdgeCases(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    20,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	match := manager.CreateMatch()

	if _, err := manager.StartMatch(match.MatchID); !errors.Is(err, ErrNotEnoughPlayers) {
		t.Fatalf("expected ErrNotEnoughPlayers with 0 players, got %v", err)
	}

	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); !errors.Is(err, ErrNotEnoughPlayers) {
		t.Fatalf("expected ErrNotEnoughPlayers with 1 player, got %v", err)
	}

	if _, err := manager.JoinMatch(match.MatchID, "p2", "Bob"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}

	started, err := manager.StartMatch(match.MatchID)
	if err != nil {
		t.Fatalf("start match failed: %v", err)
	}
	if started.Status != model.MatchStatusRunning {
		t.Fatalf("unexpected started status: got=%s want=%s", started.Status, model.MatchStatusRunning)
	}
	if started.StartedAt == nil {
		t.Fatalf("expected started_at to be set")
	}

	if _, err := manager.StartMatch(match.MatchID); !errors.Is(err, ErrMatchAlreadyRunning) {
		t.Fatalf("expected ErrMatchAlreadyRunning, got %v", err)
	}

	if _, err := manager.EndMatch(match.MatchID, "manual_end"); err != nil {
		t.Fatalf("end match failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); !errors.Is(err, ErrMatchAlreadyEnded) {
		t.Fatalf("expected ErrMatchAlreadyEnded, got %v", err)
	}
}

func TestEndMatchValidationAndDefaults(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	if _, err := manager.EndMatch("missing", "reason"); !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("expected ErrMatchNotFound, got %v", err)
	}

	match := manager.CreateMatch()
	ended, err := manager.EndMatch(match.MatchID, "   ")
	if err != nil {
		t.Fatalf("unexpected end error: %v", err)
	}
	if ended.Status != model.MatchStatusGameOver {
		t.Fatalf("unexpected status after end: got=%s want=%s", ended.Status, model.MatchStatusGameOver)
	}
	if ended.EndedReason != "manual_end" {
		t.Fatalf("expected default ended reason manual_end, got %s", ended.EndedReason)
	}
	if ended.EndedAt == nil {
		t.Fatalf("expected ended_at timestamp to be set")
	}

	if _, err := manager.EndMatch(match.MatchID, "again"); !errors.Is(err, ErrMatchAlreadyEnded) {
		t.Fatalf("expected ErrMatchAlreadyEnded on second end, got %v", err)
	}
}

func TestLifecycleEventsFilteringAndOrdering(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	matchA := manager.CreateMatch()
	matchB := manager.CreateMatch()

	if _, err := manager.JoinMatch(matchA.MatchID, "p1", "A1"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(matchA.MatchID, "p2", "A2"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(matchA.MatchID); err != nil {
		t.Fatalf("start match A failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker to be created on start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, matchA.MatchID, 1)

	if _, err := manager.EndMatch(matchA.MatchID, "manual_end"); err != nil {
		t.Fatalf("end match A failed: %v", err)
	}

	allEvents := manager.LifecycleEvents("")
	if len(allEvents) < 6 {
		t.Fatalf("expected multiple events, got %d", len(allEvents))
	}

	onlyA := manager.LifecycleEvents(matchA.MatchID)
	for _, event := range onlyA {
		if event.MatchID != matchA.MatchID {
			t.Fatalf("expected only match A events, found event for %s", event.MatchID)
		}
	}
	if len(onlyA) == 0 {
		t.Fatalf("expected at least one event for match A")
	}

	onlyB := manager.LifecycleEvents(matchB.MatchID)
	if len(onlyB) != 1 || onlyB[0].Type != LifecycleEventMatchCreated {
		t.Fatalf("expected match B to have only creation event, got %#v", onlyB)
	}
}

func TestCreateJoinStartTickEndLifecycleIntegration(t *testing.T) {
	manager, clock, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    3,
			TickRateHz:    30,
			MatchIDPrefix: "int",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Bob"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}

	started, err := manager.StartMatch(match.MatchID)
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	if started.Status != model.MatchStatusRunning {
		t.Fatalf("expected running status after start, got %s", started.Status)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	clock.Advance(33 * time.Millisecond)
	ticker.Tick(clock.Now())
	clock.Advance(33 * time.Millisecond)
	ticker.Tick(clock.Now())

	waitForTick(t, manager, match.MatchID, 2)

	ended, err := manager.EndMatch(match.MatchID, "manual_end")
	if err != nil {
		t.Fatalf("end failed: %v", err)
	}
	if ended.Status != model.MatchStatusGameOver {
		t.Fatalf("expected game_over after end, got %s", ended.Status)
	}
	if ended.TickID < 2 {
		t.Fatalf("expected tick id to retain progressed value, got %d", ended.TickID)
	}
	if ended.EndedAt == nil {
		t.Fatalf("expected ended_at to be set")
	}

	waitForStop(t, ticker)

	lastSnapshot, ok := manager.MatchSnapshot(match.MatchID)
	if !ok {
		t.Fatalf("expected snapshot after ending match")
	}
	if lastSnapshot.Status != model.MatchStatusGameOver {
		t.Fatalf("unexpected final status: got=%s want=%s", lastSnapshot.Status, model.MatchStatusGameOver)
	}
}

func TestJoinIsRejectedAfterMatchStart(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    3,
			TickRateHz:    30,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Bob"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.JoinMatch(match.MatchID, "p3", "Charlie"); !errors.Is(err, ErrMatchNotJoinable) {
		t.Fatalf("expected ErrMatchNotJoinable after start, got %v", err)
	}
}

func TestCloseEndsRunningMatchesAndStopsTickers(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    20,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "A"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "B"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	manager.Close()
	waitForStop(t, ticker)

	snapshot, ok := manager.MatchSnapshot(match.MatchID)
	if !ok {
		t.Fatalf("expected snapshot after close")
	}
	if snapshot.Status != model.MatchStatusGameOver {
		t.Fatalf("expected close to force game_over, got %s", snapshot.Status)
	}
	if snapshot.EndedReason != "server_shutdown" {
		t.Fatalf("expected ended reason server_shutdown, got %s", snapshot.EndedReason)
	}
}

func TestPlayersCanJoinNewMatchAfterPreviousMatchEnds(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "m",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	first := manager.CreateMatch()
	if _, err := manager.JoinMatch(first.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join on first match failed: %v", err)
	}
	if _, err := manager.EndMatch(first.MatchID, "manual_end"); err != nil {
		t.Fatalf("end first match failed: %v", err)
	}

	second := manager.CreateMatch()
	if _, err := manager.JoinMatch(second.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("expected player to re-join after previous match ended, got error: %v", err)
	}
}

func TestFullSnapshotIncludesCurrentStateAndReturnsDeepCopy(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "snap",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	first, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if first.Kind != model.SnapshotKindFull {
		t.Fatalf("expected full snapshot kind, got %s", first.Kind)
	}
	if first.State == nil {
		t.Fatalf("expected full snapshot to include state")
	}
	if first.State.Status != model.MatchStatusRunning {
		t.Fatalf("expected running status in full snapshot, got %s", first.State.Status)
	}
	if len(first.State.Players) != 1 {
		t.Fatalf("expected one player in full snapshot, got %d", len(first.State.Players))
	}
	if len(first.PlayerAcks) != 1 || first.PlayerAcks[0].PlayerID != "p1" {
		t.Fatalf("expected one player ack for p1, got %#v", first.PlayerAcks)
	}

	first.State.Players[0].Name = "MutatedOutside"
	first.PlayerAcks[0].LastProcessedClientSeq = 99

	second, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("second full snapshot failed: %v", err)
	}
	if second.State == nil {
		t.Fatalf("expected second full snapshot state")
	}
	if second.State.Players[0].Name != "Alice" {
		t.Fatalf("expected manager state to be immutable from caller mutation, got %s", second.State.Players[0].Name)
	}
	if second.PlayerAcks[0].LastProcessedClientSeq != 0 {
		t.Fatalf("expected ack seq to remain 0, got %d", second.PlayerAcks[0].LastProcessedClientSeq)
	}
}

func TestSnapshotsSinceReturnsOrderedDeltaHistory(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "snap",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	movePayload, err := json.Marshal(model.MovementInputPayload{
		MoveX: 1,
		MoveY: 0,
	})
	if err != nil {
		t.Fatalf("marshal move payload: %v", err)
	}
	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdMoveIntent,
		Payload:   movePayload,
	}); err != nil {
		t.Fatalf("submit move command failed: %v", err)
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	snapshots, err := manager.SnapshotsSince(match.MatchID, 0)
	if err != nil {
		t.Fatalf("snapshots since 0 failed: %v", err)
	}
	if len(snapshots) < 2 {
		t.Fatalf("expected at least two snapshots after two ticks, got %d", len(snapshots))
	}

	first := snapshots[0]
	second := snapshots[1]
	if first.Kind != model.SnapshotKindDelta {
		t.Fatalf("expected first streamed snapshot to be delta, got %s", first.Kind)
	}
	if first.TickID != 1 || first.BaseTickID != 0 {
		t.Fatalf("expected first delta tick/base 1/0, got %d/%d", first.TickID, first.BaseTickID)
	}
	if second.Kind != model.SnapshotKindDelta {
		t.Fatalf("expected second streamed snapshot to be delta, got %s", second.Kind)
	}
	if second.TickID != 2 || second.BaseTickID != 1 {
		t.Fatalf("expected second delta tick/base 2/1, got %d/%d", second.TickID, second.BaseTickID)
	}
	if first.Delta == nil {
		t.Fatalf("expected first delta payload")
	}
	if len(first.Delta.ChangedPlayers) == 0 {
		t.Fatalf("expected first delta to include changed player from move command")
	}
	if first.Delta.ChangedPlayers[0].ID != "p1" {
		t.Fatalf("expected changed player id p1, got %s", first.Delta.ChangedPlayers[0].ID)
	}
	if len(first.PlayerAcks) == 0 || first.PlayerAcks[0].LastProcessedClientSeq < 1 {
		t.Fatalf("expected ack to include processed seq >=1, got %#v", first.PlayerAcks)
	}

	filtered, err := manager.SnapshotsSince(match.MatchID, 1)
	if err != nil {
		t.Fatalf("snapshots since 1 failed: %v", err)
	}
	if len(filtered) == 0 || filtered[0].TickID <= 1 {
		t.Fatalf("expected snapshots filtered after tick 1, got %#v", filtered)
	}
}

func mustSubmitInteract(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	payload model.InteractPayload,
) {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal interact payload: %v", err)
	}

	_, err = manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdInteract,
		Payload:   raw,
	})
	if err != nil {
		t.Fatalf("submit interact input failed: %v", err)
	}
}

func mustSubmitMoveIntent(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	moveX float32,
	moveY float32,
	sprint bool,
) {
	t.Helper()

	raw, err := json.Marshal(model.MovementInputPayload{
		MoveX:  moveX,
		MoveY:  moveY,
		Sprint: sprint,
	})
	if err != nil {
		t.Fatalf("marshal move payload: %v", err)
	}

	_, err = manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdMoveIntent,
		Payload:   raw,
	})
	if err != nil {
		t.Fatalf("submit move input failed: %v", err)
	}
}

func setPlayerRoleAndFactionForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	role model.RoleType,
	faction model.FactionType,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].Role = role
		session.gameState.Players[idx].Faction = faction
		session.gameState.Players[idx].AssignedAbility = ""
		return
	}
}

func setPlayerAssignedAbilityForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	ability model.AbilityType,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].AssignedAbility = ability
		return
	}
}

func setPlayerAlignmentForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	alignment model.AlignmentType,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].Alignment = alignment
		return
	}
}

func setPlayerRoomForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	roomID model.RoomID,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].CurrentRoomID = roomID
		return
	}
}

func setMapPowerForTest(manager *Manager, matchID model.MatchID, powerOn bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	session.gameState.Map.PowerOn = powerOn
}

func setMapAlarmForTest(manager *Manager, matchID model.MatchID, alarm model.AlarmState) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	session.gameState.Map.Alarm = alarm
}

func setDoorOpenForTest(manager *Manager, matchID model.MatchID, doorID model.DoorID, open bool) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Map.Doors {
		if session.gameState.Map.Doors[idx].ID == doorID {
			session.gameState.Map.Doors[idx].Open = open
			return
		}
	}
}

func setPlayerPositionForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	position model.Vector2,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].Position = position
		return
	}
}

func setPlayerStunnedUntilForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	tickID uint64,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].StunnedUntilTick = tickID
		return
	}
}

func approachDoorForManagerTest(
	t *testing.T,
	doorID model.DoorID,
	fromRoomID model.RoomID,
) (model.Vector2, float32, float32) {
	t.Helper()

	var link gamemap.DoorLink
	found := false
	for _, candidate := range defaultMatchLayout.DoorLinks() {
		if candidate.ID == doorID {
			link = candidate
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("door %d not found", doorID)
	}

	directions := []gamemap.Point{
		{X: 0, Y: -1},
		{X: 0, Y: 1},
		{X: -1, Y: 0},
		{X: 1, Y: 0},
	}
	doorRoomID, doorRoomExists := defaultMatchLayout.RoomAt(link.Position)
	if doorRoomExists && doorRoomID == fromRoomID {
		for _, direction := range directions {
			neighbor := gamemap.Point{
				X: link.Position.X + direction.X,
				Y: link.Position.Y + direction.Y,
			}
			roomID, exists := defaultMatchLayout.RoomAt(neighbor)
			if !exists || roomID == "" || roomID == fromRoomID {
				continue
			}
			moveX := float32(neighbor.X - link.Position.X)
			moveY := float32(neighbor.Y - link.Position.Y)
			position := edgeApproachPositionForManagerStep(link.Position, moveX, moveY, physics.BaseMoveStepPerTick)
			return position, moveX, moveY
		}
	}

	for _, direction := range directions {
		neighbor := gamemap.Point{
			X: link.Position.X + direction.X,
			Y: link.Position.Y + direction.Y,
		}
		roomID, exists := defaultMatchLayout.RoomAt(neighbor)
		if !exists || roomID != fromRoomID {
			continue
		}
		moveX := float32(link.Position.X - neighbor.X)
		moveY := float32(link.Position.Y - neighbor.Y)
		position := edgeApproachPositionForManagerStep(neighbor, moveX, moveY, physics.BaseMoveStepPerTick)
		return position, moveX, moveY
	}

	t.Fatalf("door %d has no adjacent tile in room %s", doorID, fromRoomID)
	return model.Vector2{}, 0, 0
}

func edgeApproachPositionForManagerStep(
	fromTile gamemap.Point,
	moveX float32,
	moveY float32,
	step float32,
) model.Vector2 {
	shift := float32(0.01)
	if step > 0 && step < 0.49 {
		shift = 0.5 - step + 0.01
	}

	x := float32(fromTile.X)
	y := float32(fromTile.Y)
	if moveX > 0 {
		x += shift
	} else if moveX < 0 {
		x -= shift
	}
	if moveY > 0 {
		y += shift
	} else if moveY < 0 {
		y -= shift
	}
	return model.Vector2{X: x, Y: y}
}

func setPlayerAliveForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	alive bool,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID != playerID {
			continue
		}
		session.gameState.Players[idx].Alive = alive
		return
	}
}

func playerAssignedCellForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.CellID {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.AssignedCell
		}
	}
	return 0
}

func playerAssignedAbilityForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.AbilityType {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.AssignedAbility
		}
	}
	return ""
}

func doorIDForCellForTest(manager *Manager, matchID model.MatchID, cellID model.CellID) model.DoorID {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, cell := range session.gameState.Map.Cells {
		if cell.ID == cellID {
			return cell.DoorID
		}
	}
	return 0
}

func doorOpenForTest(manager *Manager, matchID model.MatchID, doorID model.DoorID) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, door := range session.gameState.Map.Doors {
		if door.ID == doorID {
			return door.Open
		}
	}
	return false
}

func playerRoomForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.RoomID {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.CurrentRoomID
		}
	}
	return ""
}

func playerPositionForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.Vector2 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.Position
		}
	}
	return model.Vector2{}
}

func playerVelocityForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) model.Vector2 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.Velocity
		}
	}
	return model.Vector2{}
}

func playerPhaseForTest(manager *Manager, matchID model.MatchID) model.PhaseState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	return session.gameState.Phase
}

func cycleCountForTest(manager *Manager, matchID model.MatchID) uint8 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	return session.gameState.CycleCount
}

func mapAlarmForTest(manager *Manager, matchID model.MatchID) model.AlarmState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	return session.gameState.Map.Alarm
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start.UTC()}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	return c.now
}

type manualTicker struct {
	mu        sync.Mutex
	channel   chan time.Time
	stopped   bool
	stopCalls int
}

func newManualTicker(buffer int) *manualTicker {
	if buffer <= 0 {
		buffer = 1
	}
	return &manualTicker{
		channel: make(chan time.Time, buffer),
	}
}

func (t *manualTicker) Chan() <-chan time.Time {
	return t.channel
}

func (t *manualTicker) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return
	}
	t.stopped = true
	t.stopCalls++
	close(t.channel)
}

func (t *manualTicker) Tick(at time.Time) {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		return
	}
	channel := t.channel
	t.mu.Unlock()

	channel <- at
}

func (t *manualTicker) IsStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

type tickerFactoryRecorder struct {
	mu        sync.Mutex
	tickers   []*manualTicker
	intervals []time.Duration
}

func (f *tickerFactoryRecorder) New(interval time.Duration) lifecycleTicker {
	ticker := newManualTicker(32)

	f.mu.Lock()
	f.tickers = append(f.tickers, ticker)
	f.intervals = append(f.intervals, interval)
	f.mu.Unlock()

	return ticker
}

func (f *tickerFactoryRecorder) Last() *manualTicker {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.tickers) == 0 {
		return nil
	}
	return f.tickers[len(f.tickers)-1]
}

func (f *tickerFactoryRecorder) Intervals() []time.Duration {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]time.Duration, len(f.intervals))
	copy(out, f.intervals)
	return out
}

func newTestManager(config Config, start time.Time) (*Manager, *fakeClock, *tickerFactoryRecorder) {
	clock := newFakeClock(start)
	factory := &tickerFactoryRecorder{}
	manager := newManagerWithDeps(config, clock.Now, factory.New)
	return manager, clock, factory
}

func waitForTick(t *testing.T, manager *Manager, matchID model.MatchID, tickID uint64) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		snapshot, exists := manager.MatchSnapshot(matchID)
		if exists && snapshot.TickID >= tickID {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}

	snapshot, _ := manager.MatchSnapshot(matchID)
	t.Fatalf("timeout waiting for tick >= %d (latest=%d)", tickID, snapshot.TickID)
}

func waitForStop(t *testing.T, ticker *manualTicker) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if ticker.IsStopped() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}

	t.Fatalf("timeout waiting for ticker stop")
}

func TestConfigNormalizationAndTickIntervalEdgeCases(t *testing.T) {
	cfg := Config{
		MinPlayers:    8,
		MaxPlayers:    4,
		TickRateHz:    0,
		DaySeconds:    0,
		NightSeconds:  0,
		MaxCycles:     0,
		MatchIDPrefix: "   ",
	}
	normalized := cfg.normalized()

	if normalized.MinPlayers != 4 || normalized.MaxPlayers != 4 {
		t.Fatalf("expected min to clamp to max, got min=%d max=%d", normalized.MinPlayers, normalized.MaxPlayers)
	}
	if normalized.TickRateHz == 0 {
		t.Fatalf("expected tick rate default to be set")
	}
	if normalized.MatchIDPrefix != "match" {
		t.Fatalf("expected default match id prefix, got %q", normalized.MatchIDPrefix)
	}
	if normalized.DaySeconds == 0 || normalized.NightSeconds == 0 {
		t.Fatalf("expected non-zero default phase durations, got day=%d night=%d", normalized.DaySeconds, normalized.NightSeconds)
	}
	if normalized.MaxCycles != 6 {
		t.Fatalf("expected default max cycles 6, got %d", normalized.MaxCycles)
	}

	interval := normalized.TickInterval()
	if interval <= 0 {
		t.Fatalf("expected positive tick interval, got %s", interval)
	}

	highRateInterval := Config{
		MinPlayers:    2,
		MaxPlayers:    4,
		TickRateHz:    2_000_000_000,
		MatchIDPrefix: "x",
	}.TickInterval()
	if highRateInterval != time.Nanosecond {
		t.Fatalf("expected high tick rates to clamp to 1ns interval, got %s", highRateInterval)
	}
}

func TestListMatchSnapshotsSortedByMatchID(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "mx",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	manager.CreateMatch()
	manager.CreateMatch()
	manager.CreateMatch()

	snapshots := manager.ListMatchSnapshots()
	got := make([]model.MatchID, 0, len(snapshots))
	for _, snapshot := range snapshots {
		got = append(got, snapshot.MatchID)
	}

	want := []model.MatchID{"mx-000001", "mx-000002", "mx-000003"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sorted match list: got=%v want=%v", got, want)
	}
}

func TestMatchConstraintsExposeNormalizedLobbyBounds(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    0,
			MaxPlayers:    0,
			TickRateHz:    0,
			MatchIDPrefix: "mx",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)

	minPlayers, maxPlayers := manager.MatchConstraints()
	if minPlayers == 0 || maxPlayers == 0 {
		t.Fatalf("expected normalized min/max players, got min=%d max=%d", minPlayers, maxPlayers)
	}
	if minPlayers > maxPlayers {
		t.Fatalf("expected min players <= max players, got min=%d max=%d", minPlayers, maxPlayers)
	}

	if manager.TickRateHz() == 0 {
		t.Fatalf("expected normalized non-zero tick rate")
	}
}
