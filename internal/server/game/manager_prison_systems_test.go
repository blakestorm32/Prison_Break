package game

import (
	"encoding/json"
	"testing"
	"time"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/gamecore/prison"
	"prison-break/internal/shared/model"
)

func TestPowerRoomInteractTogglesPowerAndDoorBehavior(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    10,
			MatchIDPrefix: "pow",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Power"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomPowerRoom)
	setDoorOpenForTest(manager, match.MatchID, 1, false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	// Empty interact in power room toggles power off.
	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if mapPowerForTest(manager, match.MatchID) {
		t.Fatalf("expected power to be off after power-room interact")
	}
	for _, door := range mapDoorsForTest(manager, match.MatchID) {
		if !door.Open {
			t.Fatalf("expected door %d to be forced open when power off", door.ID)
		}
		if door.CanClose {
			t.Fatalf("expected door %d to be non-closable when power off", door.ID)
		}
	}

	// Door-target interact cannot close while power is off.
	mustSubmitInteract(t, manager, match.MatchID, "p1", 2, model.InteractPayload{
		TargetDoorID: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if !doorOpenForTest(manager, match.MatchID, 1) {
		t.Fatalf("expected door to remain open while power off")
	}

	// Toggle power back on and confirm doors can be closed again.
	mustSubmitInteract(t, manager, match.MatchID, "p1", 3, model.InteractPayload{})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if !mapPowerForTest(manager, match.MatchID) {
		t.Fatalf("expected power to be on after second power-room interact")
	}
	for _, door := range mapDoorsForTest(manager, match.MatchID) {
		if !door.CanClose {
			t.Fatalf("expected door %d to be closable when power restored", door.ID)
		}
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 4, model.InteractPayload{
		TargetDoorID: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	if doorOpenForTest(manager, match.MatchID, 1) {
		t.Fatalf("expected door close to work when power on")
	}
}

func TestAlarmAbilitySpawnsGuardsShootsRestrictedPrisonersAndAutoStops(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    5,
			NightSeconds:  5,
			MaxCycles:     6,
			MatchIDPrefix: "alarm",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join prisoner failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "warden", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomPowerRoom)
	setPlayerPositionForTest(manager, match.MatchID, "warden", model.Vector2{X: 4, Y: 10})
	setPlayerPositionForTest(manager, match.MatchID, "pris", model.Vector2{X: 18, Y: 4})
	setPlayerHeartsForTest(manager, match.MatchID, "pris", 6)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "warden", 1, model.AbilityAlarm)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	alarm := mapAlarmForTest(manager, match.MatchID)
	if !alarm.Active || alarm.TriggeredBy != "warden" {
		t.Fatalf("expected alarm active and triggered by warden, got %+v", alarm)
	}
	expectedEnds := uint64(1) + prison.AlarmDurationTicks(2)
	if alarm.EndsTick != expectedEnds {
		t.Fatalf("expected alarm end tick %d, got %d", expectedEnds, alarm.EndsTick)
	}

	if guardCount := npcGuardCountForTest(manager, match.MatchID); guardCount != 4 {
		t.Fatalf("expected one npc guard per restricted zone (4), got %d", guardCount)
	}
	if hearts := playerHeartsForTest(manager, match.MatchID, "pris"); hearts != 4 {
		t.Fatalf("expected first guard shot to reduce hearts 6->4, got %d", hearts)
	}

	// At 2Hz with 1-second guard cadence, next shot should occur at tick 3.
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if hearts := playerHeartsForTest(manager, match.MatchID, "pris"); hearts != 4 {
		t.Fatalf("expected no guard shot on tick 2, got hearts %d", hearts)
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if hearts := playerHeartsForTest(manager, match.MatchID, "pris"); hearts != 2 {
		t.Fatalf("expected second guard shot on tick 3 (4->2), got %d", hearts)
	}

	// Move prisoner out of restricted room, alarm should auto-stop and remove guard entities.
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCorridorMain)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)

	alarm = mapAlarmForTest(manager, match.MatchID)
	if alarm.Active {
		t.Fatalf("expected alarm to auto-stop when no prisoners remain in restricted rooms, got %+v", alarm)
	}
	if guardCount := npcGuardCountForTest(manager, match.MatchID); guardCount != 0 {
		t.Fatalf("expected guard entities removed when alarm stops, got %d", guardCount)
	}
}

func TestAlarmGuardLethalHitConsumesLifeAndRespawnsPrisoner(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    5,
			NightSeconds:  5,
			MaxCycles:     6,
			MatchIDPrefix: "alarm-life",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join prisoner failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "warden", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomPowerRoom)
	setPlayerHeartsForTest(manager, match.MatchID, "pris", 2)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "warden", 1, model.AbilityAlarm)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if hearts := playerHeartsForTest(manager, match.MatchID, "pris"); hearts != 6 {
		t.Fatalf("expected lethal guard hit to consume one life and respawn with full hearts, got %d", hearts)
	}
	if lives := playerLivesForTest(manager, match.MatchID, "pris"); lives != 2 {
		t.Fatalf("expected one life consumed by lethal guard shot (3->2), got %d", lives)
	}
	if !playerAliveForTest(manager, match.MatchID, "pris") {
		t.Fatalf("expected prisoner to remain alive after non-final life loss")
	}
	if room := playerRoomForTest(manager, match.MatchID, "pris"); room != gamemap.RoomCellBlockA {
		t.Fatalf("expected prisoner respawned to cell block after life loss, got %s", room)
	}
}

func TestAlarmAbilityOncePerDayAndFixedDurationExpiry(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    10,
			NightSeconds:  10,
			MaxCycles:     6,
			MatchIDPrefix: "alarm-day",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "warden", "Warden"); err != nil {
		t.Fatalf("join warden failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join prisoner failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "warden", model.RoleWarden, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "warden", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomPowerRoom)
	setPlayerHeartsForTest(manager, match.MatchID, "pris", 40)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "warden", 1, model.AbilityAlarm)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	first := mapAlarmForTest(manager, match.MatchID)
	if !first.Active {
		t.Fatalf("expected first alarm activation")
	}

	// Same day second alarm attempt should be denied and must not extend timer.
	mustSubmitUseAbility(t, manager, match.MatchID, "warden", 2, model.AbilityAlarm)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	second := mapAlarmForTest(manager, match.MatchID)
	if second.EndsTick != first.EndsTick {
		t.Fatalf("expected second same-day alarm attempt to not extend duration: first=%d second=%d", first.EndsTick, second.EndsTick)
	}

	// Alarm should end at configured duration even if restricted prisoners remain.
	for tick := uint64(3); tick <= first.EndsTick; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(tick), 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, first.EndsTick)

	expired := mapAlarmForTest(manager, match.MatchID)
	if expired.Active {
		t.Fatalf("expected alarm to expire at fixed duration tick %d, got %+v", first.EndsTick, expired)
	}
	if guardCount := npcGuardCountForTest(manager, match.MatchID); guardCount != 0 {
		t.Fatalf("expected guard entities removed on fixed alarm expiry, got %d", guardCount)
	}
}

func mustSubmitUseAbility(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	ability model.AbilityType,
) {
	t.Helper()

	raw, err := json.Marshal(model.AbilityUsePayload{
		Ability: ability,
	})
	if err != nil {
		t.Fatalf("marshal ability payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseAbility,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit ability input failed: %v", err)
	}
}

func mapPowerForTest(manager *Manager, matchID model.MatchID) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.matches[matchID].gameState.Map.PowerOn
}

func mapDoorsForTest(manager *Manager, matchID model.MatchID) []model.DoorState {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	doors := manager.matches[matchID].gameState.Map.Doors
	out := make([]model.DoorState, len(doors))
	copy(out, doors)
	return out
}

func npcGuardCountForTest(manager *Manager, matchID model.MatchID) int {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	count := 0
	for _, entity := range session.gameState.Entities {
		if entity.Kind == model.EntityKindNPCGuard {
			count++
		}
	}
	return count
}
