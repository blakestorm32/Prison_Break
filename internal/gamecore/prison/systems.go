package prison

import (
	"sort"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

const (
	AlarmDurationSeconds    uint16 = 5
	GuardShotIntervalSecond uint16 = 1
	GuardShotDamageHalf     uint8  = 2
)

func AlarmDurationTicks(tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}
	return uint64(AlarmDurationSeconds) * uint64(tickRateHz)
}

func GuardShotIntervalTicks(tickRateHz uint32) uint64 {
	if tickRateHz == 0 {
		return 0
	}
	return uint64(GuardShotIntervalSecond) * uint64(tickRateHz)
}

func ApplyPowerState(mapState *model.MapState, powerOn bool) bool {
	if mapState == nil {
		return false
	}

	changed := false
	if mapState.PowerOn != powerOn {
		mapState.PowerOn = powerOn
		changed = true
	}

	for index := range mapState.Doors {
		door := &mapState.Doors[index]
		if powerOn {
			if !door.CanClose {
				door.CanClose = true
				changed = true
			}
			continue
		}

		if door.CanClose {
			door.CanClose = false
			changed = true
		}
		if !door.Open {
			door.Open = true
			changed = true
		}
	}

	return changed
}

func IsRestrictedRoom(roomID model.RoomID, mapState model.MapState) bool {
	if roomID == "" {
		return false
	}

	for _, zone := range mapState.RestrictedZones {
		if zone.Restricted && zone.RoomID == roomID {
			return true
		}
	}
	return false
}

func RestrictedPrisonerIDs(players []model.PlayerState, mapState model.MapState) []model.PlayerID {
	ids := make([]model.PlayerID, 0, len(players))
	for _, player := range players {
		if !player.Alive {
			continue
		}
		if !gamemap.IsPrisonerPlayer(player) {
			continue
		}
		if !IsRestrictedRoom(player.CurrentRoomID, mapState) {
			continue
		}
		ids = append(ids, player.ID)
	}

	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})
	return ids
}
