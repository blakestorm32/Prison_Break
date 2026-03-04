package prison

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestAlarmAndGuardTickDurations(t *testing.T) {
	if got := AlarmDurationTicks(30); got != 150 {
		t.Fatalf("expected 5s alarm duration to be 150 ticks at 30Hz, got %d", got)
	}
	if got := GuardShotIntervalTicks(30); got != 30 {
		t.Fatalf("expected 1s guard interval to be 30 ticks at 30Hz, got %d", got)
	}
	if got := AlarmDurationTicks(0); got != 0 {
		t.Fatalf("expected zero tick-rate alarm duration to be 0, got %d", got)
	}
}

func TestApplyPowerStateUpdatesDoorsForPowerOffAndOn(t *testing.T) {
	layout := gamemap.DefaultPrisonLayout()
	mapState := layout.ToMapState()

	mapState.PowerOn = true
	mapState.Doors[0].Open = false
	mapState.Doors[0].CanClose = true
	mapState.Doors[1].Open = true
	mapState.Doors[1].CanClose = true

	if changed := ApplyPowerState(&mapState, false); !changed {
		t.Fatalf("expected power-off transition to report changed")
	}
	if mapState.PowerOn {
		t.Fatalf("expected power to be off")
	}
	for _, door := range mapState.Doors {
		if !door.Open {
			t.Fatalf("expected all doors to force open when power off, found closed door %d", door.ID)
		}
		if door.CanClose {
			t.Fatalf("expected all doors to disallow close when power off, door %d still closable", door.ID)
		}
	}

	if changed := ApplyPowerState(&mapState, true); !changed {
		t.Fatalf("expected power-on transition to report changed")
	}
	if !mapState.PowerOn {
		t.Fatalf("expected power to be on")
	}
	for _, door := range mapState.Doors {
		if !door.CanClose {
			t.Fatalf("expected all doors to become closable when power on, door %d not closable", door.ID)
		}
	}

	if changed := ApplyPowerState(&mapState, true); changed {
		t.Fatalf("expected idempotent same-state power apply to report unchanged")
	}
}

func TestRestrictedPrisonerIDsDeterministicFiltering(t *testing.T) {
	mapState := gamemap.DefaultPrisonLayout().ToMapState()

	players := []model.PlayerState{
		{
			ID:            "c",
			Alive:         true,
			Role:          model.RoleGangMember,
			Faction:       model.FactionPrisoner,
			CurrentRoomID: gamemap.RoomPowerRoom,
		},
		{
			ID:            "a",
			Alive:         true,
			Role:          model.RoleNeutralPrisoner,
			Faction:       model.FactionNeutral,
			CurrentRoomID: gamemap.RoomAmmoRoom,
		},
		{
			ID:            "b",
			Alive:         true,
			Role:          model.RoleDeputy,
			Faction:       model.FactionAuthority,
			CurrentRoomID: gamemap.RoomPowerRoom,
		},
		{
			ID:            "dead",
			Alive:         false,
			Role:          model.RoleGangMember,
			Faction:       model.FactionPrisoner,
			CurrentRoomID: gamemap.RoomPowerRoom,
		},
		{
			ID:            "free",
			Alive:         true,
			Role:          model.RoleGangMember,
			Faction:       model.FactionPrisoner,
			CurrentRoomID: gamemap.RoomCorridorMain,
		},
	}

	got := RestrictedPrisonerIDs(players, mapState)
	want := []model.PlayerID{"a", "c"}
	if len(got) != len(want) {
		t.Fatalf("unexpected restricted prisoner count: got=%v want=%v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unexpected restricted prisoner ordering/value: got=%v want=%v", got, want)
		}
	}
}
