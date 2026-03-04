package abilities

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestIsKnownAbilityAndSpecLookup(t *testing.T) {
	if !IsKnownAbility(model.AbilityAlarm) {
		t.Fatalf("expected alarm to be a known ability")
	}
	spec, exists := SpecFor(model.AbilitySearch)
	if !exists {
		t.Fatalf("expected search ability spec to exist")
	}
	if spec.Scope != ScopeAuthority {
		t.Fatalf("expected search scope authority, got %s", spec.Scope)
	}
	if IsKnownAbility(model.AbilityType("invalid_ability")) {
		t.Fatalf("expected invalid ability to be unknown")
	}
}

func TestCooldownTicksAndOncePerDayFlags(t *testing.T) {
	if got := CooldownTicks(model.AbilityTracker, 30); got == 0 {
		t.Fatalf("expected tracker to have non-zero cooldown at 30Hz")
	}
	if got := CooldownTicks(model.AbilityType("invalid_ability"), 30); got != 0 {
		t.Fatalf("expected unknown ability cooldown to be zero, got %d", got)
	}

	if !OncePerDay(model.AbilityAlarm) {
		t.Fatalf("expected alarm to be once-per-day")
	}
	if OncePerDay(model.AbilityTracker) {
		t.Fatalf("expected tracker to not be once-per-day")
	}
}

func TestCanPlayerUseScopeRules(t *testing.T) {
	warden := model.PlayerState{
		Role:    model.RoleWarden,
		Faction: model.FactionAuthority,
	}
	deputy := model.PlayerState{
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
	}
	prisoner := model.PlayerState{
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}

	if !CanPlayerUse(warden, model.AbilityAlarm) {
		t.Fatalf("expected warden to use alarm")
	}
	if CanPlayerUse(deputy, model.AbilityAlarm) {
		t.Fatalf("expected deputy to be blocked from warden-only alarm")
	}
	if !CanPlayerUse(deputy, model.AbilitySearch) {
		t.Fatalf("expected authority to use search")
	}
	if CanPlayerUse(prisoner, model.AbilitySearch) {
		t.Fatalf("expected prisoner blocked from authority search")
	}
	if !CanPlayerUse(prisoner, model.AbilityPickPocket) {
		t.Fatalf("expected prisoner to use pick-pocket")
	}
	if CanPlayerUse(deputy, model.AbilityPickPocket) {
		t.Fatalf("expected authority blocked from prisoner pick-pocket")
	}
}

func TestEffectDurationTicksForTimedAbilities(t *testing.T) {
	if got := EffectDurationTicks(model.AbilityDetainer, 30); got != 90 {
		t.Fatalf("expected detainer duration 90 ticks at 30Hz, got %d", got)
	}
	if got := EffectDurationTicks(model.AbilityType("invalid_ability"), 30); got != 0 {
		t.Fatalf("expected unknown ability duration 0, got %d", got)
	}
}
