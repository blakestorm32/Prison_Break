package game

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"prison-break/internal/engine/physics"
	"prison-break/internal/gamecore/cards"
	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestAbilitySearchConfiscatesContrabandAndIsOncePerDay(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-search",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "pris", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
		{Item: model.ItemWood, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 1, model.AbilitySearch, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "pris")
	if items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected search ability to confiscate contraband shiv")
	}
	if !items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemWood, 1) {
		t.Fatalf("expected non-contraband inventory to remain after search")
	}

	setPlayerInventoryForTest(manager, match.MatchID, "pris", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
		{Item: model.ItemWood, Quantity: 1},
	})
	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 2, model.AbilitySearch, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	inventory = playerInventoryForTest(manager, match.MatchID, "pris")
	if !items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected once-per-day search to block second same-day confiscation")
	}
}

func TestAbilityUseAutoTargetsWhenPayloadOmitsTarget(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-auto-target",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "far", "Far"); err != nil {
		t.Fatalf("join far failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "far", model.RoleGangMember, model.FactionPrisoner)
	setPlayerAssignedAbilityForTest(manager, match.MatchID, "auth", model.AbilitySearch)
	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "far", gamemap.RoomCourtyard)
	setPlayerPositionForTest(manager, match.MatchID, "auth", model.Vector2{X: 4, Y: 4})
	setPlayerPositionForTest(manager, match.MatchID, "pris", model.Vector2{X: 5, Y: 4})
	setPlayerPositionForTest(manager, match.MatchID, "far", model.Vector2{X: 12, Y: 12})
	setPlayerInventoryForTest(manager, match.MatchID, "pris", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "auth", 1, model.AbilitySearch)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "pris")
	if items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected server auto-targeting to select in-room target and confiscate contraband")
	}
}

func TestAbilityUseDeniedWhenAssignedAbilityMismatch(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-mismatch",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerAssignedAbilityForTest(manager, match.MatchID, "auth", model.AbilityTracker)
	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "pris", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 1, model.AbilitySearch, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "pris")
	if !items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemShiv, 1) {
		t.Fatalf("expected mismatched assigned ability to block search effect")
	}

	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "auth")
	if !strings.Contains(feedback.Message, "can't use that here") {
		t.Fatalf("expected clear denial feedback message, got %+v", feedback)
	}
	if !strings.Contains(feedback.Message, string(model.AbilityTracker)) {
		t.Fatalf("expected denial feedback to mention assigned ability, got %+v", feedback)
	}
}

func TestAbilityUseDeniedMessageExplainsContextRequirement(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-context",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "cam", "Camera"); err != nil {
		t.Fatalf("join cam failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "cam", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerAssignedAbilityForTest(manager, match.MatchID, "cam", model.AbilityCameraMan)
	setPlayerRoomForTest(manager, match.MatchID, "cam", gamemap.RoomCorridorMain)
	setMapPowerForTest(manager, match.MatchID, true)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 1, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	feedback := playerLastActionFeedbackForTest(manager, match.MatchID, "cam")
	if !strings.Contains(feedback.Message, "can't use that here: go to the camera room") {
		t.Fatalf("expected contextual denial feedback for camera room requirement, got %+v", feedback)
	}
}

func TestAbilityTrackerCooldownBlocksRapidReuseThenAllows(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    30,
			NightSeconds:  30,
			MaxCycles:     6,
			MatchIDPrefix: "ab-track",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 1, model.AbilityTracker, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	firstEnds := effectEndTickForTest(manager, match.MatchID, "auth", model.EffectTrackerView)
	if firstEnds == 0 {
		t.Fatalf("expected tracker_view effect on ability user after first use")
	}
	if end := effectEndTickForTest(manager, match.MatchID, "pris", model.EffectTracked); end != 0 {
		t.Fatalf("expected tracker ability to stop marking target players directly, got tracked end %d", end)
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 2, model.AbilityTracker, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	secondEnds := effectEndTickForTest(manager, match.MatchID, "auth", model.EffectTrackerView)
	if secondEnds != firstEnds {
		t.Fatalf("expected tracker cooldown to block rapid second use, first=%d second=%d", firstEnds, secondEnds)
	}

	for tick := 3; tick <= 8; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, tick, 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, 8)

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 3, model.AbilityTracker, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 9, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 9)
	thirdEnds := effectEndTickForTest(manager, match.MatchID, "auth", model.EffectTrackerView)
	if thirdEnds <= firstEnds {
		t.Fatalf("expected tracker reuse after cooldown to extend effect, first=%d third=%d", firstEnds, thirdEnds)
	}
}

func TestAbilityPickPocketAndHackerApplyExpectedEffects(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-pris",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "thief", "Thief"); err != nil {
		t.Fatalf("join thief failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "victim", "Victim"); err != nil {
		t.Fatalf("join victim failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "thief", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "victim", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "thief", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "victim", gamemap.RoomCorridorMain)
	setPlayerInventoryForTest(manager, match.MatchID, "victim", []model.ItemStack{
		{Item: model.ItemWood, Quantity: 2},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "thief", 1, model.AbilityPickPocket, "victim")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	thiefInventory := playerInventoryForTest(manager, match.MatchID, "thief")
	victimInventory := playerInventoryForTest(manager, match.MatchID, "victim")
	if !items.HasItem(model.PlayerState{Inventory: thiefInventory}, model.ItemWood, 1) {
		t.Fatalf("expected pick-pocket to steal one wood into thief inventory")
	}
	if !items.HasItem(model.PlayerState{Inventory: victimInventory}, model.ItemWood, 1) {
		t.Fatalf("expected victim to retain one wood after one-item pick-pocket")
	}

	if !mapPowerForTest(manager, match.MatchID) {
		t.Fatalf("expected map power to start on")
	}
	mustSubmitUseAbility(t, manager, match.MatchID, "thief", 2, model.AbilityHacker)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if mapPowerForTest(manager, match.MatchID) {
		t.Fatalf("expected hacker ability to toggle map power off")
	}
}

func TestAbilityCameraManRequiresCameraRoomAndPowerAndAppliesCameraViewOnUser(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-camera",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "cam", "Camera"); err != nil {
		t.Fatalf("join cam failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "restricted", "Restricted"); err != nil {
		t.Fatalf("join restricted failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "free", "Free"); err != nil {
		t.Fatalf("join free failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "cam", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "restricted", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "free", model.RoleGangMember, model.FactionPrisoner)

	setPlayerRoomForTest(manager, match.MatchID, "cam", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "restricted", gamemap.RoomWardenHQ)
	setPlayerRoomForTest(manager, match.MatchID, "free", gamemap.RoomCorridorMain)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 1, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	if end := effectEndTickForTest(manager, match.MatchID, "cam", model.EffectCameraView); end != 0 {
		t.Fatalf("expected camera_man outside camera room to fail, got camera_view end %d", end)
	}

	setPlayerRoomForTest(manager, match.MatchID, "cam", gamemap.RoomCameraRoom)
	setMapPowerForTest(manager, match.MatchID, false)
	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 2, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if end := effectEndTickForTest(manager, match.MatchID, "cam", model.EffectCameraView); end != 0 {
		t.Fatalf("expected camera_man to fail while power off, got camera_view end %d", end)
	}

	setMapPowerForTest(manager, match.MatchID, true)
	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 3, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	cameraEnd := effectEndTickForTest(manager, match.MatchID, "cam", model.EffectCameraView)
	if cameraEnd <= 3 {
		t.Fatalf("expected camera_man to grant camera_view effect on user, got end tick %d", cameraEnd)
	}
	if end := effectEndTickForTest(manager, match.MatchID, "restricted", model.EffectTracked); end != 0 {
		t.Fatalf("expected camera_man to stop tracked-marker side effects, restricted tracked end=%d", end)
	}
	if end := effectEndTickForTest(manager, match.MatchID, "free", model.EffectTracked); end != 0 {
		t.Fatalf("expected camera_man to stop tracked-marker side effects, free tracked end=%d", end)
	}
}

func TestAbilityCameraManActivatesWithoutRestrictedTargetsAndConsumesCooldown(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-camera-zero",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "cam", "Camera"); err != nil {
		t.Fatalf("join cam failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "free", "Free"); err != nil {
		t.Fatalf("join free failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "cam", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "free", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "cam", gamemap.RoomCameraRoom)
	setPlayerRoomForTest(manager, match.MatchID, "free", gamemap.RoomCorridorMain)
	setMapPowerForTest(manager, match.MatchID, true)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 1, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	firstFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "cam")
	if !strings.Contains(firstFeedback.Message, "Camera feed active for") {
		t.Fatalf("expected camera ability to activate with zero targets, got %+v", firstFeedback)
	}
	if strings.Contains(strings.ToLower(firstFeedback.Message), "can't use that here") {
		t.Fatalf("expected camera ability not to deny when zero restricted targets, got %+v", firstFeedback)
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "cam", 2, model.AbilityCameraMan)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	secondFeedback := playerLastActionFeedbackForTest(manager, match.MatchID, "cam")
	if !strings.Contains(secondFeedback.Message, "cooldown") {
		t.Fatalf("expected second immediate camera use to be blocked by cooldown, got %+v", secondFeedback)
	}
}

func TestAbilityDetainerRequiresSameRoomAndLocksAssignedCell(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-detain",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "auth", "Authority"); err != nil {
		t.Fatalf("join auth failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "pris", "Prisoner"); err != nil {
		t.Fatalf("join pris failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "auth", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "pris", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomWardenHQ)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCourtyard)

	assignedCell := playerAssignedCellForTest(manager, match.MatchID, "pris")
	if assignedCell == 0 {
		t.Fatalf("expected assigned cell for detainer target")
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 1, model.AbilityDetainer, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	if until := playerSolitaryUntilForTest(manager, match.MatchID, "pris"); until != 0 {
		t.Fatalf("expected detainer out-of-room use to fail, solitary=%d", until)
	}

	setPlayerRoomForTest(manager, match.MatchID, "auth", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "pris", gamemap.RoomCorridorMain)
	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 2, model.AbilityDetainer, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	firstUntil := playerSolitaryUntilForTest(manager, match.MatchID, "pris")
	if firstUntil <= 2 {
		t.Fatalf("expected successful detainer to set solitary duration, got %d", firstUntil)
	}
	if lockedCell := playerLockedInCellForTest(manager, match.MatchID, "pris"); lockedCell != assignedCell {
		t.Fatalf("expected detainer to lock target in assigned cell %d, got %d", assignedCell, lockedCell)
	}

	mustSubmitUseAbilityTargetForTest(t, manager, match.MatchID, "auth", 3, model.AbilityDetainer, "pris")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if until := playerSolitaryUntilForTest(manager, match.MatchID, "pris"); until != firstUntil {
		t.Fatalf("expected cooldown to block rapid detainer reuse, first=%d second=%d", firstUntil, until)
	}
}

func TestAbilityDisguiseAndChameleonApplyDurationsCooldownAndExpire(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-self",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "p1", 1, model.AbilityDisguise)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	disguiseEnd := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectDisguised)
	if disguiseEnd <= 1 {
		t.Fatalf("expected disguise effect to apply with future end tick, got %d", disguiseEnd)
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "p1", 2, model.AbilityDisguise)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if end := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectDisguised); end != disguiseEnd {
		t.Fatalf("expected disguise cooldown to block immediate reuse, first=%d second=%d", disguiseEnd, end)
	}

	mustSubmitUseAbility(t, manager, match.MatchID, "p1", 3, model.AbilityChameleon)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	if _, active := effectForTest(manager, match.MatchID, "p1", model.EffectChameleon); active {
		t.Fatalf("expected chameleon to require stillness delay before activation")
	}

	for tick := uint64(4); tick <= 13; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(tick), 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, 13)

	chameleonEnd, active := effectForTest(manager, match.MatchID, "p1", model.EffectChameleon)
	if !active {
		t.Fatalf("expected chameleon to activate after 5s stillness")
	}
	if chameleonEnd != 0 {
		t.Fatalf("expected chameleon active effect to be movement-gated (no end tick), got %d", chameleonEnd)
	}

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 4, 1, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 14, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 14)

	for tick := uint64(15); tick <= disguiseEnd+1; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(tick), 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, disguiseEnd+1)

	if end := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectDisguised); end != 0 {
		t.Fatalf("expected disguise effect cleanup after expiry, got end tick %d", end)
	}
	if _, active = effectForTest(manager, match.MatchID, "p1", model.EffectChameleon); active {
		t.Fatalf("expected chameleon effect cleanup after moving")
	}
}

func TestAbilityLocksmithRequiresPowerAndTargetDoor(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-lock",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setDoorStateForTest(manager, match.MatchID, 1, true, false)
	setMapPowerForTest(manager, match.MatchID, false)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseAbilityWithDoorForTest(t, manager, match.MatchID, "p1", 1, model.AbilityLocksmith, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)
	locked, open := doorLockStateForTest(manager, match.MatchID, 1)
	if !locked || open {
		t.Fatalf("expected locksmith to fail while power off, got locked=%t open=%t", locked, open)
	}

	setMapPowerForTest(manager, match.MatchID, true)
	mustSubmitUseAbilityWithDoorForTest(t, manager, match.MatchID, "p1", 2, model.AbilityLocksmith, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	locked, open = doorLockStateForTest(manager, match.MatchID, 1)
	if locked || !open {
		t.Fatalf("expected locksmith to unlock and open target door, got locked=%t open=%t", locked, open)
	}

	setDoorStateForTest(manager, match.MatchID, 1, true, false)
	mustSubmitUseAbilityWithDoorForTest(t, manager, match.MatchID, "p1", 3, model.AbilityLocksmith, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)
	locked, open = doorLockStateForTest(manager, match.MatchID, 1)
	if !locked || open {
		t.Fatalf("expected cooldown to block immediate locksmith reuse, got locked=%t open=%t", locked, open)
	}
}

func TestAbilityLocksmithUnlocksPowerRoomForAllPrisoners(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    3,
			MaxPlayers:    6,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-lock-room",
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
	if _, err := manager.JoinMatch(match.MatchID, "a1", "A1"); err != nil {
		t.Fatalf("join a1 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p2", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "a1", model.RoleDeputy, model.FactionAuthority)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "a1", gamemap.RoomCorridorMain)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomPowerRoom,
	})
	mustSubmitInteract(t, manager, match.MatchID, "p2", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomPowerRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected locksmith prisoner blocked from power room before unlock, got %s", room)
	}
	if room := playerRoomForTest(manager, match.MatchID, "p2"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected second prisoner blocked from power room before unlock, got %s", room)
	}

	mustSubmitUseAbilityWithDoorForTest(t, manager, match.MatchID, "p1", 2, model.AbilityLocksmith, 3)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	mustSubmitInteract(t, manager, match.MatchID, "p1", 3, model.InteractPayload{
		TargetRoomID: gamemap.RoomPowerRoom,
	})
	mustSubmitInteract(t, manager, match.MatchID, "p2", 2, model.InteractPayload{
		TargetRoomID: gamemap.RoomPowerRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomPowerRoom {
		t.Fatalf("expected locksmith user allowed into power room after unlock, got %s", room)
	}
	if room := playerRoomForTest(manager, match.MatchID, "p2"); room != gamemap.RoomPowerRoom {
		t.Fatalf("expected room unlock to apply to all prisoners, got %s", room)
	}
}

func TestAbilityLocksmithAmmoUnlockStillRespectsPowerOffRule(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "ab-lock-ammo",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitInteract(t, manager, match.MatchID, "p1", 1, model.InteractPayload{
		TargetRoomID: gamemap.RoomAmmoRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected prisoner blocked from ammo room before locksmith unlock, got %s", room)
	}

	mustSubmitUseAbilityWithDoorForTest(t, manager, match.MatchID, "p1", 2, model.AbilityLocksmith, 4)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	mustSubmitInteract(t, manager, match.MatchID, "p1", 3, model.InteractPayload{
		TargetRoomID: gamemap.RoomAmmoRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 3)

	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomAmmoRoom {
		t.Fatalf("expected prisoner allowed into ammo room after locksmith unlock, got %s", room)
	}

	setMapPowerForTest(manager, match.MatchID, false)
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	mustSubmitInteract(t, manager, match.MatchID, "p1", 4, model.InteractPayload{
		TargetRoomID: gamemap.RoomAmmoRoom,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)

	if room := playerRoomForTest(manager, match.MatchID, "p1"); room != gamemap.RoomCorridorMain {
		t.Fatalf("expected ammo room to stay blocked while power is off, got %s", room)
	}
}

func TestCardMorphineConsumesCardAndHealsWithCap(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-heal",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerHeartsForTest(manager, match.MatchID, "p1", 4)
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardMorphine})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardMorphine)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if hearts := playerHeartsForTest(manager, match.MatchID, "p1"); hearts != 6 {
		t.Fatalf("expected morphine heal capped at role max (4->6), got %d", hearts)
	}
	if cards := playerCardsForTest(manager, match.MatchID, "p1"); len(cards) != 0 {
		t.Fatalf("expected morphine card to be consumed, got %v", cards)
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 2, model.CardMorphine)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if hearts := playerHeartsForTest(manager, match.MatchID, "p1"); hearts != 6 {
		t.Fatalf("expected card use without inventory card to no-op, got hearts %d", hearts)
	}
}

func TestCardSpeedBoostAffectsMovementAndExpires(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "card-speed",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardSpeed})
	setPlayerPositionForTest(manager, match.MatchID, "p1", model.Vector2{X: 3, Y: 14})
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCellBlockA)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardSpeed)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	speedEndsTick := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectSpeedBoost)
	if speedEndsTick == 0 {
		t.Fatalf("expected speed card to add speed-boost effect")
	}

	beforeFast := playerPositionForTest(manager, match.MatchID, "p1")
	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 2, 1.35, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	afterFast := playerPositionForTest(manager, match.MatchID, "p1")
	if (afterFast.X - beforeFast.X) < (physics.SprintMoveStepPerTick - 0.01) {
		t.Fatalf("expected boosted move step close to sprint step, before=%+v after=%+v", beforeFast, afterFast)
	}

	for tick := uint64(3); tick <= speedEndsTick+1; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(tick), 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, speedEndsTick+1)
	if end := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectSpeedBoost); end != 0 {
		t.Fatalf("expected speed effect to expire and be cleaned up, got end tick %d", end)
	}

	beforeSlow := playerPositionForTest(manager, match.MatchID, "p1")
	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 3, 1.35, 0, false)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(speedEndsTick+2), 0, time.UTC))
	waitForTick(t, manager, match.MatchID, speedEndsTick+2)
	afterSlow := playerPositionForTest(manager, match.MatchID, "p1")
	if (afterSlow.X - beforeSlow.X) > (physics.BaseMoveStepPerTick + 0.01) {
		t.Fatalf("expected expired speed effect to return to base move step, before=%+v after=%+v", beforeSlow, afterSlow)
	}
}

func TestCardDoorStopBlocksDoorToggleUntilExpiry(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			DaySeconds:    20,
			NightSeconds:  20,
			MaxCycles:     6,
			MatchIDPrefix: "card-door",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "DoorStop"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "Other"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardDoorStop})
	setPlayerRoomForTest(manager, match.MatchID, "p1", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "p2", gamemap.RoomCorridorMain)
	setDoorOpenForTest(manager, match.MatchID, 1, true)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardWithDoorForTest(t, manager, match.MatchID, "p1", 1, model.CardDoorStop, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if doorOpenForTest(manager, match.MatchID, 1) {
		t.Fatalf("expected door stop card to close the target door")
	}
	blockedUntil := doorBlockedUntilForTest(manager, match.MatchID, 1)
	expectedBlockedUntil := uint64(1) + cards.DoorStopDurationTicks(2)
	if blockedUntil != expectedBlockedUntil {
		t.Fatalf("expected door blocked until %d, got %d", expectedBlockedUntil, blockedUntil)
	}

	mustSubmitInteract(t, manager, match.MatchID, "p2", 1, model.InteractPayload{
		TargetDoorID: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	if doorOpenForTest(manager, match.MatchID, 1) {
		t.Fatalf("expected blocked door to reject toggles before expiry")
	}

	for tick := uint64(3); tick <= blockedUntil; tick++ {
		ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(tick), 0, time.UTC))
	}
	waitForTick(t, manager, match.MatchID, blockedUntil)

	mustSubmitInteract(t, manager, match.MatchID, "p2", 2, model.InteractPayload{
		TargetDoorID: 1,
	})
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, int(blockedUntil+1), 0, time.UTC))
	waitForTick(t, manager, match.MatchID, blockedUntil+1)
	if !doorOpenForTest(manager, match.MatchID, 1) {
		t.Fatalf("expected door toggle to succeed once door-stop block expires")
	}
}

func TestCardGetOutOfJailFreeClearsSolitaryPenalty(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-jail",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardGetOutOfJailFree})
	setPlayerSolitaryUntilForTest(manager, match.MatchID, "p1", 50)
	setPlayerLockedInCellForTest(manager, match.MatchID, "p1", 3)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardGetOutOfJailFree)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if until := playerSolitaryUntilForTest(manager, match.MatchID, "p1"); until != 0 {
		t.Fatalf("expected get-out-of-jail-free to clear solitary, got %d", until)
	}
	if locked := playerLockedInCellForTest(manager, match.MatchID, "p1"); locked != 0 {
		t.Fatalf("expected get-out-of-jail-free to clear locked cell, got %d", locked)
	}
}

func TestCardBulletAddsAmmoAndPreservesCardAtCap(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-bullet",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardBullet})
	setPlayerBulletsForTest(manager, match.MatchID, "p1", 0)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardBullet)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if bullets := playerBulletsForTest(manager, match.MatchID, "p1"); bullets != 1 {
		t.Fatalf("expected bullet card to add one ammo, got %d", bullets)
	}
	if cards := playerCardsForTest(manager, match.MatchID, "p1"); len(cards) != 0 {
		t.Fatalf("expected successful bullet card use to consume card, got %+v", cards)
	}

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardBullet})
	setPlayerBulletsForTest(manager, match.MatchID, "p1", 255)
	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 2, model.CardBullet)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if bullets := playerBulletsForTest(manager, match.MatchID, "p1"); bullets != 255 {
		t.Fatalf("expected max-ammo bullet card use to no-op, got bullets=%d", bullets)
	}
	if cards := playerCardsForTest(manager, match.MatchID, "p1"); len(cards) != 1 || cards[0] != model.CardBullet {
		t.Fatalf("expected failed bullet card use to preserve card, got %+v", cards)
	}
}

func TestCardArmorPlateNoStackAndPhaseExpiryLifecycle(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-armor",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{
		model.CardArmorPlate,
		model.CardArmorPlate,
	})
	setPhaseEndTickForTest(manager, match.MatchID, 3)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardArmorPlate)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if temp := playerTempHeartsForTest(manager, match.MatchID, "p1"); temp != 2 {
		t.Fatalf("expected first armor plate to set +1 temporary heart (2 half-hearts), got %d", temp)
	}
	if end := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectArmorPlate); end != 3 {
		t.Fatalf("expected armor plate effect to end at phase end tick 3, got %d", end)
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 2, model.CardArmorPlate)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if temp := playerTempHeartsForTest(manager, match.MatchID, "p1"); temp != 2 {
		t.Fatalf("expected armor plate to avoid stacking temp hearts, got %d", temp)
	}
	if cards := playerCardsForTest(manager, match.MatchID, "p1"); len(cards) != 1 || cards[0] != model.CardArmorPlate {
		t.Fatalf("expected redundant armor plate use to preserve card, got %+v", cards)
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)

	if temp := playerTempHeartsForTest(manager, match.MatchID, "p1"); temp != 0 {
		t.Fatalf("expected armor plate temp hearts cleanup after expiry, got %d", temp)
	}
	if end := effectEndTickForTest(manager, match.MatchID, "p1", model.EffectArmorPlate); end != 0 {
		t.Fatalf("expected armor plate effect cleanup after expiry, got %d", end)
	}
}

func TestCardLockSnapRepairsDoorStateAfterRound(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-locksnap",
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

	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardLockSnap})
	setDoorStateForTest(manager, match.MatchID, 1, true, false)
	setPhaseEndTickForTest(manager, match.MatchID, 3)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardWithDoorForTest(t, manager, match.MatchID, "p1", 1, model.CardLockSnap, 1)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	locked, open := doorLockStateForTest(manager, match.MatchID, 1)
	if locked || !open {
		t.Fatalf("expected lock_snap to break lock and open door for current round, got locked=%t open=%t", locked, open)
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)
	locked, open = doorLockStateForTest(manager, match.MatchID, 1)
	if locked || !open {
		t.Fatalf("expected lock_snap door state to remain broken before repair tick, got locked=%t open=%t", locked, open)
	}

	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 3, 0, time.UTC))
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 4, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 4)
	locked, open = doorLockStateForTest(manager, match.MatchID, 1)
	if !locked || open {
		t.Fatalf("expected lock_snap repair lifecycle to restore original locked/closed state, got locked=%t open=%t", locked, open)
	}
}

func TestCardItemStealSupportsTargetItemAndRoomGate(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-steal",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "thief", "Thief"); err != nil {
		t.Fatalf("join thief failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "victim", "Victim"); err != nil {
		t.Fatalf("join victim failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "thief", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "victim", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "thief", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "victim", gamemap.RoomCorridorMain)
	setPlayerCardsForTest(manager, match.MatchID, "thief", []model.CardType{model.CardItemSteal})
	setPlayerInventoryForTest(manager, match.MatchID, "victim", []model.ItemStack{
		{Item: model.ItemWood, Quantity: 1},
		{Item: model.ItemMetalSlab, Quantity: 1},
	})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardWithPlayerAndItemForTest(
		t,
		manager,
		match.MatchID,
		"thief",
		1,
		model.CardItemSteal,
		"victim",
		model.ItemMetalSlab,
	)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if !items.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "thief")}, model.ItemMetalSlab, 1) {
		t.Fatalf("expected item_steal target-item flow to transfer requested item to thief")
	}
	victimInventory := playerInventoryForTest(manager, match.MatchID, "victim")
	if !items.HasItem(model.PlayerState{Inventory: victimInventory}, model.ItemWood, 1) ||
		items.HasItem(model.PlayerState{Inventory: victimInventory}, model.ItemMetalSlab, 1) {
		t.Fatalf("expected victim inventory to lose only targeted metal slab, got %+v", victimInventory)
	}

	setPlayerCardsForTest(manager, match.MatchID, "thief", []model.CardType{model.CardItemSteal})
	setPlayerRoomForTest(manager, match.MatchID, "victim", gamemap.RoomCourtyard)
	mustSubmitUseCardWithPlayerAndItemForTest(
		t,
		manager,
		match.MatchID,
		"thief",
		2,
		model.CardItemSteal,
		"victim",
		model.ItemWood,
	)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if cards := playerCardsForTest(manager, match.MatchID, "thief"); len(cards) != 1 || cards[0] != model.CardItemSteal {
		t.Fatalf("expected out-of-room item_steal to preserve card, got %+v", cards)
	}
}

func TestCardItemGrabStealsDeterministicItemFromTarget(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    2,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-grab",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "grabber", "Grabber"); err != nil {
		t.Fatalf("join grabber failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "target", "Target"); err != nil {
		t.Fatalf("join target failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "grabber", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoleAndFactionForTest(manager, match.MatchID, "target", model.RoleGangMember, model.FactionPrisoner)
	setPlayerRoomForTest(manager, match.MatchID, "grabber", gamemap.RoomCorridorMain)
	setPlayerRoomForTest(manager, match.MatchID, "target", gamemap.RoomCorridorMain)
	targetInventory := []model.ItemStack{
		{Item: model.ItemWood, Quantity: 2},
		{Item: model.ItemBullet, Quantity: 1},
	}
	setPlayerInventoryForTest(manager, match.MatchID, "target", targetInventory)
	setPlayerCardsForTest(manager, match.MatchID, "grabber", []model.CardType{model.CardItemGrab})

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	expectedItem := cards.DeterministicGrabFromInventory("grabber", "target", 1, targetInventory)
	if expectedItem == "" {
		t.Fatalf("expected deterministic item grab selection")
	}

	mustSubmitUseCardWithPlayerForTest(t, manager, match.MatchID, "grabber", 1, model.CardItemGrab, "target")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if !items.HasItem(model.PlayerState{Inventory: playerInventoryForTest(manager, match.MatchID, "grabber")}, expectedItem, 1) {
		t.Fatalf("expected item_grab to transfer deterministic random item %s", expectedItem)
	}
	if cards := playerCardsForTest(manager, match.MatchID, "grabber"); len(cards) != 0 {
		t.Fatalf("expected successful item_grab to consume card, got %+v", cards)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "target", nil)
	setPlayerCardsForTest(manager, match.MatchID, "grabber", []model.CardType{model.CardItemGrab})
	mustSubmitUseCardWithPlayerForTest(t, manager, match.MatchID, "grabber", 2, model.CardItemGrab, "target")
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if cards := playerCardsForTest(manager, match.MatchID, "grabber"); len(cards) != 1 || cards[0] != model.CardItemGrab {
		t.Fatalf("expected item_grab to preserve card when target has no inventory, got %+v", cards)
	}
}

func TestCardScrapBundleGrantsMaterialsAndPreservesOnCapacityFailure(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-scrap",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleGangMember, model.FactionPrisoner)
	setPlayerInventoryForTest(manager, match.MatchID, "p1", nil)
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardScrapBundle})
	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardScrapBundle)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	inventory := playerInventoryForTest(manager, match.MatchID, "p1")
	if !items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemWood, 1) ||
		!items.HasItem(model.PlayerState{Inventory: inventory}, model.ItemMetalSlab, 1) {
		t.Fatalf("expected scrap bundle to grant wood and metal slab, got %+v", inventory)
	}

	setPlayerInventoryForTest(manager, match.MatchID, "p1", []model.ItemStack{
		{Item: model.ItemShiv, Quantity: 1},
		{Item: model.ItemBullet, Quantity: 1},
		{Item: model.ItemPistol, Quantity: 1},
	})
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardScrapBundle})
	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 2, model.CardScrapBundle)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	if cards := playerCardsForTest(manager, match.MatchID, "p1"); len(cards) != 1 || cards[0] != model.CardScrapBundle {
		t.Fatalf("expected full-inventory scrap bundle to preserve card, got %+v", cards)
	}
}

func TestCardGetOutOfJailFreeClearsAuthorityPenaltyLock(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    2,
			MatchIDPrefix: "card-jail-auth",
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

	setPlayerRoleAndFactionForTest(manager, match.MatchID, "p1", model.RoleDeputy, model.FactionAuthority)
	setPlayerCardsForTest(manager, match.MatchID, "p1", []model.CardType{model.CardGetOutOfJailFree})
	setPlayerSolitaryUntilForTest(manager, match.MatchID, "p1", 100)
	setPlayerLockedInCellForTest(manager, match.MatchID, "p1", 2)

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitUseCardForTest(t, manager, match.MatchID, "p1", 1, model.CardGetOutOfJailFree)
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if until := playerSolitaryUntilForTest(manager, match.MatchID, "p1"); until != 0 {
		t.Fatalf("expected authority get-out-of-jail-free to clear solitary penalty, got %d", until)
	}
	if locked := playerLockedInCellForTest(manager, match.MatchID, "p1"); locked != 0 {
		t.Fatalf("expected authority get-out-of-jail-free to clear locked cell penalty, got %d", locked)
	}
}

func mustSubmitUseAbilityTargetForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	ability model.AbilityType,
	target model.PlayerID,
) {
	t.Helper()

	raw, err := json.Marshal(model.AbilityUsePayload{
		Ability:        ability,
		TargetPlayerID: target,
	})
	if err != nil {
		t.Fatalf("marshal ability target payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseAbility,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit ability target input failed: %v", err)
	}
}

func mustSubmitUseAbilityWithDoorForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	ability model.AbilityType,
	doorID model.DoorID,
) {
	t.Helper()

	raw, err := json.Marshal(model.AbilityUsePayload{
		Ability:      ability,
		TargetDoorID: doorID,
	})
	if err != nil {
		t.Fatalf("marshal ability door payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseAbility,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit ability door input failed: %v", err)
	}
}

func mustSubmitUseCardForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	card model.CardType,
) {
	t.Helper()

	raw, err := json.Marshal(model.CardUsePayload{
		Card: card,
	})
	if err != nil {
		t.Fatalf("marshal card payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseCard,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit card input failed: %v", err)
	}
}

func mustSubmitUseCardWithDoorForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	card model.CardType,
	doorID model.DoorID,
) {
	t.Helper()

	raw, err := json.Marshal(model.CardUsePayload{
		Card:         card,
		TargetDoorID: doorID,
	})
	if err != nil {
		t.Fatalf("marshal door-card payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseCard,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit door-card input failed: %v", err)
	}
}

func mustSubmitUseCardWithPlayerForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	card model.CardType,
	targetPlayerID model.PlayerID,
) {
	t.Helper()

	raw, err := json.Marshal(model.CardUsePayload{
		Card:           card,
		TargetPlayerID: targetPlayerID,
	})
	if err != nil {
		t.Fatalf("marshal player-card payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseCard,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit player-card input failed: %v", err)
	}
}

func mustSubmitUseCardWithPlayerAndItemForTest(
	t *testing.T,
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	clientSeq uint64,
	card model.CardType,
	targetPlayerID model.PlayerID,
	targetItem model.ItemType,
) {
	t.Helper()

	raw, err := json.Marshal(model.CardUsePayload{
		Card:           card,
		TargetPlayerID: targetPlayerID,
		TargetItem:     targetItem,
	})
	if err != nil {
		t.Fatalf("marshal player-item-card payload: %v", err)
	}

	if _, err := manager.SubmitInput(matchID, model.InputCommand{
		PlayerID:  playerID,
		ClientSeq: clientSeq,
		Type:      model.CmdUseCard,
		Payload:   raw,
	}); err != nil {
		t.Fatalf("submit player-item-card input failed: %v", err)
	}
}

func setPlayerCardsForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	playerCards []model.CardType,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID != playerID {
			continue
		}
		session.gameState.Players[index].Cards = append([]model.CardType(nil), playerCards...)
		return
	}
}

func setDoorStateForTest(
	manager *Manager,
	matchID model.MatchID,
	doorID model.DoorID,
	locked bool,
	open bool,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Map.Doors {
		door := &session.gameState.Map.Doors[index]
		if door.ID != doorID {
			continue
		}
		door.Locked = locked
		door.Open = open
		return
	}
}

func setPhaseEndTickForTest(manager *Manager, matchID model.MatchID, endTick uint64) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	session.gameState.Phase.EndsTick = endTick
}

func playerCardsForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
) []model.CardType {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return append([]model.CardType(nil), player.Cards...)
		}
	}
	return nil
}

func doorLockStateForTest(
	manager *Manager,
	matchID model.MatchID,
	doorID model.DoorID,
) (locked bool, open bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, door := range session.gameState.Map.Doors {
		if door.ID == doorID {
			return door.Locked, door.Open
		}
	}
	return false, false
}

func effectEndTickForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	effect model.EffectType,
) uint64 {
	endTick, _ := effectForTest(manager, matchID, playerID, effect)
	return endTick
}

func effectForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	effect model.EffectType,
) (endTick uint64, exists bool) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID != playerID {
			continue
		}
		for _, candidate := range player.Effects {
			if candidate.Effect == effect {
				return candidate.EndsTick, true
			}
		}
	}
	return 0, false
}

func playerTempHeartsForTest(manager *Manager, matchID model.MatchID, playerID model.PlayerID) uint8 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, player := range session.gameState.Players {
		if player.ID == playerID {
			return player.TempHeartsHalf
		}
	}
	return 0
}

func setPlayerSolitaryUntilForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	untilTick uint64,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID != playerID {
			continue
		}
		session.gameState.Players[index].SolitaryUntilTick = untilTick
		return
	}
}

func setPlayerLockedInCellForTest(
	manager *Manager,
	matchID model.MatchID,
	playerID model.PlayerID,
	cellID model.CellID,
) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	session := manager.matches[matchID]
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID != playerID {
			continue
		}
		session.gameState.Players[index].LockedInCell = cellID
		return
	}
}

func doorBlockedUntilForTest(manager *Manager, matchID model.MatchID, doorID model.DoorID) uint64 {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	session := manager.matches[matchID]
	for _, door := range session.gameState.Map.Doors {
		if door.ID == doorID {
			return door.BlockedUntilTick
		}
	}
	return 0
}
