package roles

import (
	"encoding/binary"
	"hash/fnv"
	"sort"

	"prison-break/internal/shared/model"
)

// ApplyGangLeaderSuccession promotes a living gang member to gang leader when no living gang leader remains.
// It returns the deterministically sorted set of player IDs whose role/alignment/faction changed.
func ApplyGangLeaderSuccession(
	state *model.GameState,
	matchID model.MatchID,
	tickID uint64,
) []model.PlayerID {
	if state == nil || len(state.Players) == 0 {
		return nil
	}

	aliveLeaderExists := false
	deadLeaderIndexes := make([]int, 0, 1)
	eligibleIndexes := make([]int, 0, 4)
	for index := range state.Players {
		player := state.Players[index]
		switch player.Role {
		case model.RoleGangLeader:
			if player.Alive {
				aliveLeaderExists = true
			} else {
				deadLeaderIndexes = append(deadLeaderIndexes, index)
			}
		case model.RoleGangMember:
			if player.Alive {
				eligibleIndexes = append(eligibleIndexes, index)
			}
		}
	}

	if aliveLeaderExists || len(eligibleIndexes) == 0 {
		return nil
	}

	sort.Slice(eligibleIndexes, func(i int, j int) bool {
		return state.Players[eligibleIndexes[i]].ID < state.Players[eligibleIndexes[j]].ID
	})

	changed := make(map[model.PlayerID]struct{}, len(deadLeaderIndexes)+1)
	for _, index := range deadLeaderIndexes {
		player := &state.Players[index]
		if player.Role != model.RoleGangMember {
			player.Role = model.RoleGangMember
			changed[player.ID] = struct{}{}
		}
	}

	selectedPos := deterministicSuccessorIndex(state, matchID, tickID, eligibleIndexes)
	successor := &state.Players[eligibleIndexes[selectedPos]]

	if successor.Role != model.RoleGangLeader {
		successor.Role = model.RoleGangLeader
		changed[successor.ID] = struct{}{}
	}
	if successor.Faction != model.FactionPrisoner {
		successor.Faction = model.FactionPrisoner
		changed[successor.ID] = struct{}{}
	}
	if successor.Alignment != model.AlignmentEvil {
		successor.Alignment = model.AlignmentEvil
		changed[successor.ID] = struct{}{}
	}

	out := make([]model.PlayerID, 0, len(changed))
	for playerID := range changed {
		out = append(out, playerID)
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func deterministicSuccessorIndex(
	state *model.GameState,
	matchID model.MatchID,
	tickID uint64,
	eligibleIndexes []int,
) int {
	if len(eligibleIndexes) <= 1 {
		return 0
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))

	var tickBytes [8]byte
	binary.LittleEndian.PutUint64(tickBytes[:], tickID)
	_, _ = hasher.Write(tickBytes[:])

	for _, index := range eligibleIndexes {
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(state.Players[index].ID))
	}

	return int(hasher.Sum64() % uint64(len(eligibleIndexes)))
}
