package gamemap

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestEvaluateRoomEntryWardenHQRestriction(t *testing.T) {
	mapState := DefaultPrisonLayout().ToMapState()

	warden := model.PlayerState{
		ID:      "warden",
		Role:    model.RoleWarden,
		Faction: model.FactionAuthority,
	}
	prisoner := model.PlayerState{
		ID:      "prisoner",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}

	wardenDecision := EvaluateRoomEntry(warden, RoomWardenHQ, mapState)
	if !wardenDecision.Allowed || wardenDecision.Verdict != AccessAllow {
		t.Fatalf("expected warden to enter warden_hq, got %#v", wardenDecision)
	}

	prisonerDecision := EvaluateRoomEntry(prisoner, RoomWardenHQ, mapState)
	if prisonerDecision.Allowed {
		t.Fatalf("expected non-warden denied for warden_hq")
	}
	if prisonerDecision.Verdict != AccessDenyWardenHQRestricted {
		t.Fatalf("unexpected verdict for non-warden warden_hq access: %#v", prisonerDecision)
	}
}

func TestEvaluateRoomEntryCameraRoomAuthorityAndPowerRules(t *testing.T) {
	layout := DefaultPrisonLayout()
	mapState := layout.ToMapState()

	authority := model.PlayerState{
		ID:      "authority",
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
	}
	prisoner := model.PlayerState{
		ID:      "prisoner",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}

	authorityDecision := EvaluateRoomEntry(authority, RoomCameraRoom, mapState)
	if !authorityDecision.Allowed {
		t.Fatalf("expected authority to enter camera room while power on, got %#v", authorityDecision)
	}

	prisonerDecision := EvaluateRoomEntry(prisoner, RoomCameraRoom, mapState)
	if prisonerDecision.Allowed || prisonerDecision.Verdict != AccessDenyCameraAuthorityOnly {
		t.Fatalf("expected prisoner denied camera room by authority rule, got %#v", prisonerDecision)
	}

	mapState.PowerOn = false
	powerOffDecision := EvaluateRoomEntry(authority, RoomCameraRoom, mapState)
	if powerOffDecision.Allowed || powerOffDecision.Verdict != AccessDenyCameraPowerOff {
		t.Fatalf("expected authority denied camera room when power off, got %#v", powerOffDecision)
	}
}

func TestEvaluateRoomEntryAmmoRoomPowerRule(t *testing.T) {
	layout := DefaultPrisonLayout()
	mapState := layout.ToMapState()

	player := model.PlayerState{
		ID:      "player",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}

	powerOnDecision := EvaluateRoomEntry(player, RoomAmmoRoom, mapState)
	if !powerOnDecision.Allowed {
		t.Fatalf("expected ammo room to be enterable while power on, got %#v", powerOnDecision)
	}

	mapState.PowerOn = false
	powerOffDecision := EvaluateRoomEntry(player, RoomAmmoRoom, mapState)
	if powerOffDecision.Allowed || powerOffDecision.Verdict != AccessDenyAmmoPowerOff {
		t.Fatalf("expected ammo room denied when power off, got %#v", powerOffDecision)
	}
}

func TestEvaluateRoomEntryBlackMarketPrisonerOnlyRule(t *testing.T) {
	mapState := DefaultPrisonLayout().ToMapState()

	prisoner := model.PlayerState{
		ID:      "prisoner",
		Role:    model.RoleGangLeader,
		Faction: model.FactionPrisoner,
	}
	authority := model.PlayerState{
		ID:      "authority",
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
	}

	allowed := EvaluateRoomEntry(prisoner, mapState.BlackMarketRoomID, mapState)
	if !allowed.Allowed {
		t.Fatalf("expected prisoner allowed in black market room, got %#v", allowed)
	}

	denied := EvaluateRoomEntry(authority, mapState.BlackMarketRoomID, mapState)
	if denied.Allowed || denied.Verdict != AccessDenyBlackMarketPrisonersOnly {
		t.Fatalf("expected authority denied for black market room, got %#v", denied)
	}
}

func TestEvaluateCellDoorOperationOwnerAndAuthorityPrivileges(t *testing.T) {
	cell := model.CellState{
		ID:            2,
		OwnerPlayerID: "owner",
		DoorID:        202,
	}

	owner := model.PlayerState{
		ID:      "owner",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}
	authority := model.PlayerState{
		ID:      "deputy",
		Role:    model.RoleDeputy,
		Faction: model.FactionAuthority,
	}
	otherPrisoner := model.PlayerState{
		ID:      "other",
		Role:    model.RoleNeutralPrisoner,
		Faction: model.FactionPrisoner,
	}

	ownerDecision := EvaluateCellDoorOperation(owner, cell)
	if !ownerDecision.Allowed {
		t.Fatalf("expected cell owner to operate door, got %#v", ownerDecision)
	}

	authorityDecision := EvaluateCellDoorOperation(authority, cell)
	if !authorityDecision.Allowed {
		t.Fatalf("expected authority to operate any cell door, got %#v", authorityDecision)
	}

	otherDecision := EvaluateCellDoorOperation(otherPrisoner, cell)
	if otherDecision.Allowed || otherDecision.Verdict != AccessDenyCellDoorForbidden {
		t.Fatalf("expected non-owner prisoner denied, got %#v", otherDecision)
	}
}

func TestRoleBasedFallbacksForAuthorityAndPrisonerClassification(t *testing.T) {
	deputyWithoutFaction := model.PlayerState{
		ID:   "dep",
		Role: model.RoleDeputy,
	}
	gangWithoutFaction := model.PlayerState{
		ID:   "gang",
		Role: model.RoleGangMember,
	}
	unknown := model.PlayerState{ID: "unknown"}

	if !IsAuthorityPlayer(deputyWithoutFaction) {
		t.Fatalf("expected deputy role to classify as authority even without faction")
	}
	if !IsPrisonerPlayer(gangWithoutFaction) {
		t.Fatalf("expected gang member role to classify as prisoner even without faction")
	}
	if IsAuthorityPlayer(unknown) || IsPrisonerPlayer(unknown) {
		t.Fatalf("expected unknown player with no role/faction to not classify as authority/prisoner")
	}
}
