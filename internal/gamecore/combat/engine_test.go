package combat

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestApplyRoleLoadoutsByRole(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{ID: "w", Role: model.RoleWarden, Alive: true, HeartsHalf: 1, Bullets: 99},
			{ID: "d", Role: model.RoleDeputy, Alive: true, HeartsHalf: 1, Bullets: 99},
			{ID: "g", Role: model.RoleGangMember, Alive: true, HeartsHalf: 1, Bullets: 99},
		},
	}

	ApplyRoleLoadouts(&state)

	if got := state.Players[0].HeartsHalf; got != 10 {
		t.Fatalf("expected warden hearts 10 half, got %d", got)
	}
	if got := state.Players[0].Bullets; got != 3 {
		t.Fatalf("expected warden bullets 3, got %d", got)
	}

	if got := state.Players[1].HeartsHalf; got != 8 {
		t.Fatalf("expected deputy hearts 8 half, got %d", got)
	}
	if got := state.Players[1].Bullets; got != 3 {
		t.Fatalf("expected deputy bullets 3, got %d", got)
	}

	if got := state.Players[2].HeartsHalf; got != 6 {
		t.Fatalf("expected prisoner hearts 6 half, got %d", got)
	}
	if got := state.Players[2].Bullets; got != 0 {
		t.Fatalf("expected prisoner bullets 0, got %d", got)
	}
	for _, player := range state.Players {
		if player.LivesRemaining != DefaultPlayerLives {
			t.Fatalf("expected player %s to start with %d lives, got %d", player.ID, DefaultPlayerLives, player.LivesRemaining)
		}
	}
}

func TestSelectTargetDeterministicByAimDistanceThenID(t *testing.T) {
	players := []model.PlayerState{
		{ID: "shooter", Alive: true, Position: model.Vector2{X: 5, Y: 5}},
		{ID: "b", Alive: true, Position: model.Vector2{X: 7, Y: 5}},
		{ID: "a", Alive: true, Position: model.Vector2{X: 7, Y: 5}},
	}

	targetID, ok := SelectTarget(
		players,
		"shooter",
		model.Vector2{X: 7, Y: 5},
		8,
		AimAssistRadiusTiles,
	)
	if !ok {
		t.Fatalf("expected target selection to succeed")
	}
	if targetID != "a" {
		t.Fatalf("expected lexical tie-break winner a, got %s", targetID)
	}

	if _, ok := SelectTarget(players, "shooter", model.Vector2{X: 20, Y: 20}, 8, AimAssistRadiusTiles); ok {
		t.Fatalf("expected out-of-radius aim to miss all targets")
	}
}

func TestCanUseWeaponRules(t *testing.T) {
	authority := model.PlayerState{
		ID:      "auth",
		Alive:   true,
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
	}
	prisoner := model.PlayerState{
		ID:    "pris",
		Alive: true,
		Role:  model.RoleGangMember,
	}
	withShiv := prisoner
	withShiv.Inventory = []model.ItemStack{{Item: model.ItemShiv, Quantity: 1}}
	withRifle := prisoner
	withRifle.Inventory = []model.ItemStack{{Item: model.ItemHuntingRifle, Quantity: 1}}

	if !CanUseWeapon(authority, WeaponBaton) {
		t.Fatalf("expected authority to be allowed to use baton")
	}
	if CanUseWeapon(prisoner, WeaponBaton) {
		t.Fatalf("expected prisoner baton usage to be blocked")
	}
	if CanUseWeapon(prisoner, model.ItemShiv) {
		t.Fatalf("expected shiv use to require shiv inventory")
	}
	if !CanUseWeapon(withShiv, model.ItemShiv) {
		t.Fatalf("expected player with shiv inventory to use shiv")
	}
	if CanUseWeapon(prisoner, model.ItemHuntingRifle) {
		t.Fatalf("expected rifle use to require rifle inventory for non-authority")
	}
	if !CanUseWeapon(withRifle, model.ItemHuntingRifle) {
		t.Fatalf("expected rifle use with rifle inventory to be allowed")
	}
}

func TestConsumeShotCostAndResolveDamage(t *testing.T) {
	shooter := model.PlayerState{
		ID:      "s",
		Alive:   true,
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
		Bullets: 2,
		Inventory: []model.ItemStack{
			{Item: model.ItemGoldenBullet, Quantity: 1},
			{Item: model.ItemShiv, Quantity: 1},
		},
	}

	damage, ok := ConsumeShotCostAndResolveDamage(&shooter, model.ItemPistol, false)
	if !ok || damage != FirearmDamageHalf {
		t.Fatalf("expected standard firearm shot cost to succeed with 2-half damage, got ok=%v damage=%d", ok, damage)
	}
	if shooter.Bullets != 1 {
		t.Fatalf("expected bullets to decrement to 1, got %d", shooter.Bullets)
	}

	damage, ok = ConsumeShotCostAndResolveDamage(&shooter, model.ItemPistol, true)
	if !ok || damage != GoldenBulletDamageHalf {
		t.Fatalf("expected golden shot to succeed with 4-half damage, got ok=%v damage=%d", ok, damage)
	}
	if shooter.Bullets != 1 {
		t.Fatalf("expected golden round not to consume standard bullets, got %d", shooter.Bullets)
	}
	if len(shooter.Inventory) != 1 || shooter.Inventory[0].Item != model.ItemShiv {
		t.Fatalf("expected golden bullet inventory to be consumed, got %#v", shooter.Inventory)
	}

	if _, ok := ConsumeShotCostAndResolveDamage(&shooter, WeaponBaton, true); ok {
		t.Fatalf("expected baton with golden-round flag to be invalid")
	}
}

func TestApplyDamageConsumesTempThenHealthAndCanEliminate(t *testing.T) {
	target := model.PlayerState{
		ID:             "t",
		Alive:          true,
		HeartsHalf:     6,
		TempHeartsHalf: 2,
		Velocity:       model.Vector2{X: 1},
	}

	result := ApplyDamage(&target, 3)
	if result.TempConsumedHalf != 2 || result.HealthConsumedHalf != 1 || result.AppliedHalf != 3 {
		t.Fatalf("unexpected damage accounting: %+v", result)
	}
	if target.TempHeartsHalf != 0 || target.HeartsHalf != 5 {
		t.Fatalf("unexpected target hearts after damage: temp=%d hearts=%d", target.TempHeartsHalf, target.HeartsHalf)
	}
	if !target.Alive {
		t.Fatalf("expected target to remain alive")
	}

	result = ApplyDamage(&target, 6)
	if !result.Eliminated {
		t.Fatalf("expected target elimination on lethal damage")
	}
	if target.Alive || target.HeartsHalf != 0 {
		t.Fatalf("expected target to be dead with zero hearts")
	}
	if target.Velocity != (model.Vector2{}) {
		t.Fatalf("expected dead target velocity reset, got %+v", target.Velocity)
	}
}

func TestIsUnjustAuthorityShotExceptions(t *testing.T) {
	target := model.PlayerState{
		ID:            "t",
		Alive:         true,
		CurrentRoomID: gamemap.RoomCorridorMain,
	}
	mapState := model.MapState{
		PowerOn: true,
		RestrictedZones: []model.ZoneState{
			{ID: 1, RoomID: gamemap.RoomPowerRoom, Restricted: true},
		},
	}

	if !IsUnjustAuthorityShot(target, mapState) {
		t.Fatalf("expected normal corridor shot to be unjust")
	}

	target.CurrentRoomID = gamemap.RoomPowerRoom
	if IsUnjustAuthorityShot(target, mapState) {
		t.Fatalf("expected restricted-room shot to be exempt from penalty")
	}

	target.CurrentRoomID = gamemap.RoomCorridorMain
	mapState.PowerOn = false
	if IsUnjustAuthorityShot(target, mapState) {
		t.Fatalf("expected power-off shot to be exempt from penalty")
	}

	mapState.PowerOn = true
	target.Inventory = []model.ItemStack{{Item: model.ItemShiv, Quantity: 1}}
	if IsUnjustAuthorityShot(target, mapState) {
		t.Fatalf("expected contraband-carrying target shot to be exempt from penalty")
	}
}

func TestApplyUnjustShotPenaltyDayOnly(t *testing.T) {
	shooter := model.PlayerState{
		ID:           "auth",
		Alive:        true,
		Role:         model.RoleDeputy,
		Faction:      model.FactionAuthority,
		AssignedCell: 3,
	}

	changed := ApplyUnjustShotPenalty(&shooter, model.PhaseState{
		Current:  model.PhaseDay,
		EndsTick: 20,
	}, 10)
	if !changed {
		t.Fatalf("expected day-phase unjust-shot penalty to apply")
	}
	if shooter.SolitaryUntilTick != 19 {
		t.Fatalf("expected solitary until day-end tick 19, got %d", shooter.SolitaryUntilTick)
	}
	if shooter.LockedInCell != 3 {
		t.Fatalf("expected shooter locked in assigned cell 3, got %d", shooter.LockedInCell)
	}
	if !IsActionBlocked(shooter, 15) {
		t.Fatalf("expected solitary shooter actions to be blocked before expiry")
	}
	if IsActionBlocked(shooter, 20) {
		t.Fatalf("expected solitary action block to expire by day-end boundary")
	}

	shooter.SolitaryUntilTick = 0
	shooter.LockedInCell = 0
	changed = ApplyUnjustShotPenalty(&shooter, model.PhaseState{
		Current:  model.PhaseNight,
		EndsTick: 30,
	}, 10)
	if changed {
		t.Fatalf("expected no unjust-shot penalty to apply at night")
	}
}
