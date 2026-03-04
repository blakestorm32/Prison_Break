package combat

import (
	"math"

	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

const (
	WeaponBaton model.ItemType = "baton"

	ShivDamageHalf         uint8 = 1
	FirearmDamageHalf      uint8 = 2
	GoldenBulletDamageHalf uint8 = 4

	AimAssistRadiusTiles float32 = 1.25

	BatonRangeTiles        float32 = 1.50
	ShivRangeTiles         float32 = 1.50
	PistolRangeTiles       float32 = 8.00
	HuntingRifleRangeTiles float32 = 12.0

	BatonKnockbackMagnitude float32 = 1.50
	BatonStunDurationSecond uint16  = 3
)

type DamageResult struct {
	AppliedHalf        uint8 `json:"applied_half"`
	TempConsumedHalf   uint8 `json:"temp_consumed_half"`
	HealthConsumedHalf uint8 `json:"health_consumed_half"`
	Eliminated         bool  `json:"eliminated"`
}

func ApplyRoleLoadouts(state *model.GameState) {
	if state == nil {
		return
	}

	for index := range state.Players {
		player := &state.Players[index]
		loadout := loadoutForRole(player.Role)
		player.HeartsHalf = loadout.HeartsHalf
		player.Bullets = loadout.Bullets
		player.TempHeartsHalf = 0
		player.Alive = loadout.HeartsHalf > 0
		player.StunnedUntilTick = 0
		player.SolitaryUntilTick = 0
		player.LockedInCell = 0
	}
}

func MaxHeartsHalfForRole(role model.RoleType) uint8 {
	return loadoutForRole(role).HeartsHalf
}

func IsSupportedWeapon(weapon model.ItemType) bool {
	switch weapon {
	case WeaponBaton, model.ItemShiv, model.ItemPistol, model.ItemHuntingRifle:
		return true
	default:
		return false
	}
}

func IsFirearm(weapon model.ItemType) bool {
	switch weapon {
	case model.ItemPistol, model.ItemHuntingRifle:
		return true
	default:
		return false
	}
}

func WeaponRangeTiles(weapon model.ItemType) float32 {
	switch weapon {
	case WeaponBaton:
		return BatonRangeTiles
	case model.ItemShiv:
		return ShivRangeTiles
	case model.ItemPistol:
		return PistolRangeTiles
	case model.ItemHuntingRifle:
		return HuntingRifleRangeTiles
	default:
		return 0
	}
}

func CanUseWeapon(player model.PlayerState, weapon model.ItemType) bool {
	if !player.Alive || !IsSupportedWeapon(weapon) {
		return false
	}

	switch weapon {
	case WeaponBaton:
		return gamemap.IsAuthorityPlayer(player)
	case model.ItemShiv:
		return items.HasItem(player, model.ItemShiv, 1)
	case model.ItemPistol:
		if gamemap.IsAuthorityPlayer(player) {
			return true
		}
		return items.HasItem(player, model.ItemPistol, 1)
	case model.ItemHuntingRifle:
		if gamemap.IsAuthorityPlayer(player) {
			return true
		}
		return items.HasItem(player, model.ItemHuntingRifle, 1)
	default:
		return false
	}
}

func IsActionBlocked(player model.PlayerState, tickID uint64) bool {
	if !player.Alive {
		return true
	}
	if player.StunnedUntilTick != 0 && tickID <= player.StunnedUntilTick {
		return true
	}
	if player.SolitaryUntilTick != 0 && tickID <= player.SolitaryUntilTick {
		return true
	}
	return false
}

func ConsumeShotCostAndResolveDamage(
	shooter *model.PlayerState,
	weapon model.ItemType,
	useGoldenRound bool,
) (uint8, bool) {
	if shooter == nil || !CanUseWeapon(*shooter, weapon) {
		return 0, false
	}

	switch weapon {
	case WeaponBaton:
		if useGoldenRound {
			return 0, false
		}
		return 0, true
	case model.ItemShiv:
		if useGoldenRound {
			return 0, false
		}
		return ShivDamageHalf, true
	case model.ItemPistol, model.ItemHuntingRifle:
		if useGoldenRound {
			if !items.RemoveItem(shooter, model.ItemGoldenBullet, 1) {
				return 0, false
			}
			return GoldenBulletDamageHalf, true
		}
		if shooter.Bullets == 0 {
			return 0, false
		}
		shooter.Bullets--
		return FirearmDamageHalf, true
	default:
		return 0, false
	}
}

func SelectTarget(
	players []model.PlayerState,
	shooterID model.PlayerID,
	aim model.Vector2,
	maxRangeTiles float32,
	aimAssistRadiusTiles float32,
) (model.PlayerID, bool) {
	if len(players) == 0 || shooterID == "" || maxRangeTiles <= 0 || aimAssistRadiusTiles <= 0 {
		return "", false
	}

	shooterIndex := -1
	for index := range players {
		if players[index].ID == shooterID {
			shooterIndex = index
			break
		}
	}
	if shooterIndex < 0 || !players[shooterIndex].Alive {
		return "", false
	}

	shooterPos := players[shooterIndex].Position
	maxRangeSquared := maxRangeTiles * maxRangeTiles
	maxAimSquared := aimAssistRadiusTiles * aimAssistRadiusTiles

	bestTargetID := model.PlayerID("")
	bestAimDistance := float32(math.MaxFloat32)

	for index := range players {
		candidate := players[index]
		if candidate.ID == shooterID || !candidate.Alive {
			continue
		}

		shooterDistanceSquared := distanceSquared(shooterPos, candidate.Position)
		if shooterDistanceSquared > maxRangeSquared {
			continue
		}

		aimDistanceSquared := distanceSquared(aim, candidate.Position)
		if aimDistanceSquared > maxAimSquared {
			continue
		}

		if aimDistanceSquared < bestAimDistance {
			bestAimDistance = aimDistanceSquared
			bestTargetID = candidate.ID
			continue
		}

		if aimDistanceSquared == bestAimDistance && (bestTargetID == "" || candidate.ID < bestTargetID) {
			bestTargetID = candidate.ID
		}
	}

	if bestTargetID == "" {
		return "", false
	}
	return bestTargetID, true
}

func BatonImpulse(attacker model.PlayerState, target model.PlayerState) model.Vector2 {
	x := target.Position.X - attacker.Position.X
	y := target.Position.Y - attacker.Position.Y

	if x == 0 && y == 0 {
		x = attacker.Facing.X
		y = attacker.Facing.Y
	}
	if x == 0 && y == 0 {
		x = 1
	}

	magnitude := float32(math.Sqrt(float64(x*x + y*y)))
	if magnitude == 0 {
		return model.Vector2{X: BatonKnockbackMagnitude}
	}

	scale := BatonKnockbackMagnitude / magnitude
	return model.Vector2{
		X: x * scale,
		Y: y * scale,
	}
}

func BatonStunDurationTicks(tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}
	return uint64(tickRateHz) * uint64(BatonStunDurationSecond)
}

func ApplyDamage(target *model.PlayerState, damageHalf uint8) DamageResult {
	if target == nil || damageHalf == 0 || !target.Alive {
		return DamageResult{}
	}

	remaining := damageHalf
	result := DamageResult{}

	if target.TempHeartsHalf > 0 && remaining > 0 {
		tempConsumed := remaining
		if tempConsumed > target.TempHeartsHalf {
			tempConsumed = target.TempHeartsHalf
		}
		target.TempHeartsHalf -= tempConsumed
		remaining -= tempConsumed
		result.TempConsumedHalf = tempConsumed
	}

	if target.HeartsHalf > 0 && remaining > 0 {
		healthConsumed := remaining
		if healthConsumed > target.HeartsHalf {
			healthConsumed = target.HeartsHalf
		}
		target.HeartsHalf -= healthConsumed
		remaining -= healthConsumed
		result.HealthConsumedHalf = healthConsumed
	}

	result.AppliedHalf = result.TempConsumedHalf + result.HealthConsumedHalf
	if target.HeartsHalf == 0 {
		target.Alive = false
		target.Velocity = model.Vector2{}
		target.StunnedUntilTick = 0
		result.Eliminated = true
	}

	return result
}

func IsUnjustAuthorityShot(target model.PlayerState, mapState model.MapState) bool {
	if !mapState.PowerOn {
		return false
	}
	if isRestrictedRoom(target.CurrentRoomID, mapState) {
		return false
	}
	if hasPenaltyExemptContraband(target) {
		return false
	}
	return true
}

func ApplyUnjustShotPenalty(
	shooter *model.PlayerState,
	phase model.PhaseState,
	tickID uint64,
) bool {
	if shooter == nil || !gamemap.IsAuthorityPlayer(*shooter) {
		return false
	}
	if phase.Current != model.PhaseDay {
		return false
	}
	if phase.EndsTick == 0 || phase.EndsTick <= tickID {
		return false
	}

	changed := false
	penaltyEnds := phase.EndsTick - 1
	if penaltyEnds > shooter.SolitaryUntilTick {
		shooter.SolitaryUntilTick = penaltyEnds
		changed = true
	}
	if shooter.AssignedCell != 0 && shooter.LockedInCell != shooter.AssignedCell {
		shooter.LockedInCell = shooter.AssignedCell
		changed = true
	}

	return changed
}

func loadoutForRole(role model.RoleType) struct {
	HeartsHalf uint8
	Bullets    uint8
} {
	switch role {
	case model.RoleWarden:
		return struct {
			HeartsHalf uint8
			Bullets    uint8
		}{HeartsHalf: 10, Bullets: 3}
	case model.RoleDeputy:
		return struct {
			HeartsHalf uint8
			Bullets    uint8
		}{HeartsHalf: 8, Bullets: 3}
	default:
		return struct {
			HeartsHalf uint8
			Bullets    uint8
		}{HeartsHalf: 6, Bullets: 0}
	}
}

func isRestrictedRoom(roomID model.RoomID, mapState model.MapState) bool {
	if roomID == "" {
		return false
	}
	for _, zone := range mapState.RestrictedZones {
		if !zone.Restricted {
			continue
		}
		if zone.RoomID == roomID {
			return true
		}
	}
	return false
}

func hasPenaltyExemptContraband(target model.PlayerState) bool {
	return items.HasItem(target, model.ItemPistol, 1) ||
		items.HasItem(target, model.ItemHuntingRifle, 1) ||
		items.HasItem(target, model.ItemShiv, 1) ||
		items.HasItem(target, model.ItemLadder, 1) ||
		items.HasItem(target, model.ItemShovel, 1) ||
		items.HasItem(target, model.ItemWireCutters, 1)
}

func distanceSquared(a model.Vector2, b model.Vector2) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return dx*dx + dy*dy
}
