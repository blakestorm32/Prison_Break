package abilities

import (
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

type ScopeType string

const (
	ScopeWardenOnly ScopeType = "warden_only"
	ScopeAuthority  ScopeType = "authority"
	ScopePrisoner   ScopeType = "prisoner"
)

type Spec struct {
	Ability         model.AbilityType `json:"ability"`
	Scope           ScopeType         `json:"scope"`
	CooldownSeconds uint16            `json:"cooldown_seconds"`
	OncePerDay      bool              `json:"once_per_day"`
}

var specsByAbility = map[model.AbilityType]Spec{
	model.AbilityAlarm: {
		Ability:         model.AbilityAlarm,
		Scope:           ScopeWardenOnly,
		CooldownSeconds: 5,
		OncePerDay:      true,
	},
	model.AbilitySearch: {
		Ability:         model.AbilitySearch,
		Scope:           ScopeAuthority,
		CooldownSeconds: 8,
		OncePerDay:      true,
	},
	model.AbilityCameraMan: {
		Ability:         model.AbilityCameraMan,
		Scope:           ScopeAuthority,
		CooldownSeconds: 6,
	},
	model.AbilityDetainer: {
		Ability:         model.AbilityDetainer,
		Scope:           ScopeAuthority,
		CooldownSeconds: 6,
	},
	model.AbilityTracker: {
		Ability:         model.AbilityTracker,
		Scope:           ScopeAuthority,
		CooldownSeconds: 4,
	},
	model.AbilityPickPocket: {
		Ability:         model.AbilityPickPocket,
		Scope:           ScopePrisoner,
		CooldownSeconds: 5,
	},
	model.AbilityHacker: {
		Ability:         model.AbilityHacker,
		Scope:           ScopePrisoner,
		CooldownSeconds: 6,
	},
	model.AbilityDisguise: {
		Ability:         model.AbilityDisguise,
		Scope:           ScopePrisoner,
		CooldownSeconds: 8,
	},
	model.AbilityLocksmith: {
		Ability:         model.AbilityLocksmith,
		Scope:           ScopePrisoner,
		CooldownSeconds: 4,
	},
	model.AbilityChameleon: {
		Ability:         model.AbilityChameleon,
		Scope:           ScopePrisoner,
		CooldownSeconds: 8,
	},
}

func IsKnownAbility(ability model.AbilityType) bool {
	_, exists := specsByAbility[ability]
	return exists
}

func SpecFor(ability model.AbilityType) (Spec, bool) {
	spec, exists := specsByAbility[ability]
	if !exists {
		return Spec{}, false
	}
	return spec, true
}

func CooldownTicks(ability model.AbilityType, tickRateHz uint32) uint64 {
	spec, exists := specsByAbility[ability]
	if !exists || tickRateHz == 0 || spec.CooldownSeconds == 0 {
		return 0
	}
	return uint64(spec.CooldownSeconds) * uint64(tickRateHz)
}

func OncePerDay(ability model.AbilityType) bool {
	spec, exists := specsByAbility[ability]
	if !exists {
		return false
	}
	return spec.OncePerDay
}

func CanPlayerUse(player model.PlayerState, ability model.AbilityType) bool {
	spec, exists := specsByAbility[ability]
	if !exists {
		return false
	}

	switch spec.Scope {
	case ScopeWardenOnly:
		return player.Role == model.RoleWarden
	case ScopeAuthority:
		return gamemap.IsAuthorityPlayer(player)
	case ScopePrisoner:
		return gamemap.IsPrisonerPlayer(player)
	default:
		return false
	}
}

func EffectDurationTicks(ability model.AbilityType, tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}

	switch ability {
	case model.AbilityCameraMan:
		return uint64(tickRateHz) * 5
	case model.AbilityDetainer:
		return uint64(tickRateHz) * 3
	case model.AbilityTracker:
		return uint64(tickRateHz) * 8
	case model.AbilityDisguise:
		return uint64(tickRateHz) * 10
	case model.AbilityChameleon:
		return uint64(tickRateHz) * 10
	default:
		return 0
	}
}
