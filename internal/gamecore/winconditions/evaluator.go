package winconditions

import (
	"sort"

	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

const (
	EscapedRoomID model.RoomID = "escaped"
)

type Config struct {
	MaxCycles uint8
}

func DefaultConfig() Config {
	return Config{
		MaxCycles: constants.MaxDayNightCycles,
	}
}

func (c Config) normalized() Config {
	if c.MaxCycles == 0 {
		c.MaxCycles = constants.MaxDayNightCycles
	}
	return c
}

func Evaluate(state model.GameState, config Config) *model.GameOverState {
	normalized := config.normalized()
	reason := detectReason(state, normalized)
	if reason == "" {
		return nil
	}

	outcome := ResolveOutcome(state, reason, state.TickID)
	return &outcome
}

func ResolveOutcome(state model.GameState, reason model.WinReason, endedTick uint64) model.GameOverState {
	winners := winnersForReason(state, reason)
	sort.Slice(winners, func(i int, j int) bool {
		return winners[i] < winners[j]
	})

	return model.GameOverState{
		Reason:          reason,
		EndedTick:       endedTick,
		WinnerPlayerIDs: winners,
	}
}

func detectReason(state model.GameState, config Config) model.WinReason {
	// Priority order is deterministic when multiple terminal conditions are true on the same tick.
	if hasEscapedGangLeader(state) {
		return model.WinReasonGangLeaderEscaped
	}
	if hasDeadWarden(state) {
		return model.WinReasonWardenDied
	}
	if hasNoAliveGangMembers(state) {
		return model.WinReasonAllGangMembersDead
	}
	if state.CycleCount >= config.MaxCycles {
		return model.WinReasonMaxCyclesReached
	}
	return ""
}

func hasEscapedGangLeader(state model.GameState) bool {
	for _, player := range state.Players {
		if player.Role != model.RoleGangLeader {
			continue
		}
		if player.Alive && player.CurrentRoomID == EscapedRoomID {
			return true
		}
	}
	return false
}

func hasDeadWarden(state model.GameState) bool {
	var hasWarden bool
	for _, player := range state.Players {
		if player.Role != model.RoleWarden {
			continue
		}
		hasWarden = true
		if player.Alive {
			return false
		}
	}
	return hasWarden
}

func hasNoAliveGangMembers(state model.GameState) bool {
	var hasGangRole bool
	for _, player := range state.Players {
		if player.Role != model.RoleGangLeader && player.Role != model.RoleGangMember {
			continue
		}
		hasGangRole = true
		if player.Alive {
			return false
		}
	}
	return hasGangRole
}

func winnersForReason(state model.GameState, reason model.WinReason) []model.PlayerID {
	winners := make([]model.PlayerID, 0, len(state.Players))
	for _, player := range state.Players {
		if !player.Alive {
			continue
		}

		switch reason {
		case model.WinReasonGangLeaderEscaped:
			if player.Role == model.RoleGangLeader || player.Role == model.RoleGangMember {
				winners = append(winners, player.ID)
				continue
			}
			if player.Faction == model.FactionAuthority && player.Alignment == model.AlignmentEvil {
				winners = append(winners, player.ID)
				continue
			}

		case model.WinReasonWardenDied:
			if player.Role == model.RoleGangLeader || player.Role == model.RoleGangMember {
				winners = append(winners, player.ID)
				continue
			}

		case model.WinReasonAllGangMembersDead,
			model.WinReasonMaxCyclesReached,
			model.WinReasonNoEscapesAtTimeLimit:
			if (player.Faction == model.FactionAuthority && player.Alignment == model.AlignmentGood) || player.Role == model.RoleSnitch {
				winners = append(winners, player.ID)
				continue
			}

		case model.WinReasonHitmanTargetEliminated,
			model.WinReasonEscapeArtistFirstEscape:
			if player.Role == model.RoleNeutralPrisoner {
				winners = append(winners, player.ID)
				continue
			}
		}
	}

	return winners
}
