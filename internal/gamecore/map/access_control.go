package gamemap

import "prison-break/internal/shared/model"

type AccessVerdict string

const (
	AccessAllow                        AccessVerdict = "allow"
	AccessDenyUnknownRoom              AccessVerdict = "deny_unknown_room"
	AccessDenyWardenHQRestricted       AccessVerdict = "deny_warden_hq_restricted"
	AccessDenyCameraAuthorityOnly      AccessVerdict = "deny_camera_authority_only"
	AccessDenyCameraPowerOff           AccessVerdict = "deny_camera_power_off"
	AccessDenyPowerRoomAuthorityOnly   AccessVerdict = "deny_power_room_authority_only"
	AccessDenyAmmoAuthorityOnly        AccessVerdict = "deny_ammo_authority_only"
	AccessDenyAmmoPowerOff             AccessVerdict = "deny_ammo_power_off"
	AccessDenyBlackMarketPrisonersOnly AccessVerdict = "deny_black_market_prisoners_only"
	AccessDenyCellDoorForbidden        AccessVerdict = "deny_cell_door_forbidden"
)

type RoomEntryDecision struct {
	RoomID  model.RoomID  `json:"room_id"`
	Allowed bool          `json:"allowed"`
	Verdict AccessVerdict `json:"verdict"`
}

type CellDoorDecision struct {
	CellID  model.CellID  `json:"cell_id"`
	Allowed bool          `json:"allowed"`
	Verdict AccessVerdict `json:"verdict"`
}

func IsAuthorityPlayer(player model.PlayerState) bool {
	if player.Faction == model.FactionAuthority {
		return true
	}
	return player.Role == model.RoleWarden || player.Role == model.RoleDeputy
}

func IsPrisonerPlayer(player model.PlayerState) bool {
	if player.Faction == model.FactionPrisoner {
		return true
	}
	switch player.Role {
	case model.RoleGangLeader, model.RoleGangMember, model.RoleSnitch, model.RoleNeutralPrisoner:
		return true
	default:
		return false
	}
}

func EvaluateRoomEntry(player model.PlayerState, roomID model.RoomID, mapState model.MapState) RoomEntryDecision {
	decision := RoomEntryDecision{
		RoomID:  roomID,
		Allowed: true,
		Verdict: AccessAllow,
	}

	switch roomID {
	case "":
		decision.Allowed = false
		decision.Verdict = AccessDenyUnknownRoom
		return decision
	case RoomWardenHQ:
		if player.Role != model.RoleWarden {
			decision.Allowed = false
			decision.Verdict = AccessDenyWardenHQRestricted
		}
		return decision
	case RoomCameraRoom:
		if !mapState.PowerOn {
			decision.Allowed = false
			decision.Verdict = AccessDenyCameraPowerOff
			return decision
		}
		if !IsAuthorityPlayer(player) {
			decision.Allowed = false
			decision.Verdict = AccessDenyCameraAuthorityOnly
			return decision
		}
		return decision
	case RoomPowerRoom:
		if !IsAuthorityPlayer(player) {
			decision.Allowed = false
			decision.Verdict = AccessDenyPowerRoomAuthorityOnly
			return decision
		}
		return decision
	case RoomAmmoRoom:
		if !mapState.PowerOn {
			decision.Allowed = false
			decision.Verdict = AccessDenyAmmoPowerOff
			return decision
		}
		if !IsAuthorityPlayer(player) {
			decision.Allowed = false
			decision.Verdict = AccessDenyAmmoAuthorityOnly
			return decision
		}
		return decision
	}

	if mapState.BlackMarketRoomID != "" && roomID == mapState.BlackMarketRoomID {
		if !IsPrisonerPlayer(player) {
			decision.Allowed = false
			decision.Verdict = AccessDenyBlackMarketPrisonersOnly
			return decision
		}
	}

	return decision
}

func CanEnterRoom(player model.PlayerState, roomID model.RoomID, mapState model.MapState) bool {
	return EvaluateRoomEntry(player, roomID, mapState).Allowed
}

func EvaluateCellDoorOperation(player model.PlayerState, cell model.CellState) CellDoorDecision {
	decision := CellDoorDecision{
		CellID:  cell.ID,
		Allowed: false,
		Verdict: AccessDenyCellDoorForbidden,
	}

	if IsAuthorityPlayer(player) {
		decision.Allowed = true
		decision.Verdict = AccessAllow
		return decision
	}

	if cell.OwnerPlayerID != "" && cell.OwnerPlayerID == player.ID {
		decision.Allowed = true
		decision.Verdict = AccessAllow
		return decision
	}

	decision.Verdict = AccessDenyCellDoorForbidden
	return decision
}

func CanOperateCellDoor(player model.PlayerState, cell model.CellState) bool {
	return EvaluateCellDoorOperation(player, cell).Allowed
}
