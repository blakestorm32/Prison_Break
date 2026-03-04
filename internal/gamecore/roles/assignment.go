package roles

import (
	"errors"
	"hash/fnv"
	"sort"

	"prison-break/internal/shared/model"
)

var (
	ErrNoPlayersProvided = errors.New("roles: no players provided")
	ErrDuplicatePlayerID = errors.New("roles: duplicate player id")
)

type Assignment struct {
	PlayerID  model.PlayerID      `json:"player_id"`
	Role      model.RoleType      `json:"role"`
	Faction   model.FactionType   `json:"faction"`
	Alignment model.AlignmentType `json:"alignment"`
}

type roleProfile struct {
	Role      model.RoleType
	Faction   model.FactionType
	Alignment model.AlignmentType
}

func Assign(playerIDs []model.PlayerID, matchID model.MatchID) ([]Assignment, error) {
	if len(playerIDs) == 0 {
		return nil, ErrNoPlayersProvided
	}

	ordered := append([]model.PlayerID(nil), playerIDs...)
	sort.Slice(ordered, func(i int, j int) bool {
		return ordered[i] < ordered[j]
	})

	for idx := 1; idx < len(ordered); idx++ {
		if ordered[idx-1] == ordered[idx] {
			return nil, ErrDuplicatePlayerID
		}
	}

	profiles := profilesForCount(len(ordered))
	seed := hashSeed(matchID, ordered)
	templateIndex := 0
	if len(profiles) > 1 {
		templateIndex = int(seed % uint64(len(profiles)))
	}

	selected := append([]roleProfile(nil), profiles[templateIndex]...)
	rng := newDeterministicRNG(seed ^ 0x9e3779b97f4a7c15)
	shuffleRoleProfiles(selected, rng)

	out := make([]Assignment, 0, len(ordered))
	for idx, playerID := range ordered {
		profile := selected[idx]
		out = append(out, Assignment{
			PlayerID:  playerID,
			Role:      profile.Role,
			Faction:   profile.Faction,
			Alignment: profile.Alignment,
		})
	}
	return out, nil
}

func ApplyAssignments(state *model.GameState, matchID model.MatchID) error {
	if state == nil || len(state.Players) == 0 {
		return nil
	}

	playerIDs := make([]model.PlayerID, 0, len(state.Players))
	for _, player := range state.Players {
		playerIDs = append(playerIDs, player.ID)
	}

	assignments, err := Assign(playerIDs, matchID)
	if err != nil {
		return err
	}

	byID := make(map[model.PlayerID]Assignment, len(assignments))
	for _, assignment := range assignments {
		byID[assignment.PlayerID] = assignment
	}

	for idx := range state.Players {
		assignment := byID[state.Players[idx].ID]
		state.Players[idx].Role = assignment.Role
		state.Players[idx].Faction = assignment.Faction
		state.Players[idx].Alignment = assignment.Alignment
	}

	return nil
}

func profilesForCount(playerCount int) [][]roleProfile {
	if templates, exists := roleTemplatesByCount[playerCount]; exists && len(templates) > 0 {
		return templates
	}

	// Fallback for low test-count matches. Production path is 6-12 players.
	profile := make([]roleProfile, 0, playerCount)
	if playerCount >= 1 {
		profile = append(profile, roleProfile{
			Role:      model.RoleWarden,
			Faction:   model.FactionAuthority,
			Alignment: model.AlignmentGood,
		})
	}
	if playerCount >= 2 {
		profile = append(profile, roleProfile{
			Role:      model.RoleDeputy,
			Faction:   model.FactionAuthority,
			Alignment: model.AlignmentGood,
		})
	}
	if playerCount >= 3 {
		profile = append(profile, roleProfile{
			Role:      model.RoleGangLeader,
			Faction:   model.FactionPrisoner,
			Alignment: model.AlignmentEvil,
		})
	}
	for len(profile) < playerCount {
		profile = append(profile, roleProfile{
			Role:      model.RoleNeutralPrisoner,
			Faction:   model.FactionNeutral,
			Alignment: model.AlignmentNeutral,
		})
	}

	return [][]roleProfile{profile}
}

func hashSeed(matchID model.MatchID, orderedPlayers []model.PlayerID) uint64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))
	for _, playerID := range orderedPlayers {
		_, _ = hasher.Write([]byte{0})
		_, _ = hasher.Write([]byte(playerID))
	}
	return hasher.Sum64()
}

func shuffleRoleProfiles(profiles []roleProfile, rng *deterministicRNG) {
	for idx := len(profiles) - 1; idx > 0; idx-- {
		swapIdx := rng.Intn(idx + 1)
		profiles[idx], profiles[swapIdx] = profiles[swapIdx], profiles[idx]
	}
}

type deterministicRNG struct {
	state uint64
}

func newDeterministicRNG(seed uint64) *deterministicRNG {
	if seed == 0 {
		seed = 0x6a09e667f3bcc909
	}
	return &deterministicRNG{state: seed}
}

func (r *deterministicRNG) next() uint64 {
	x := r.state
	x ^= x << 13
	x ^= x >> 7
	x ^= x << 17
	r.state = x
	return x
}

func (r *deterministicRNG) Intn(limit int) int {
	if limit <= 1 {
		return 0
	}
	return int(r.next() % uint64(limit))
}

var roleTemplatesByCount = map[int][][]roleProfile{
	6: {
		// A: 2 authorities good, 3 gang, 1 snitch.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
		},
		// B: 2 authorities good, 2 gang, 2 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// C: 1 authority good, 1 authority bad, 2 gang, 1 snitch, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	7: {
		// A: 3 authorities good, 3 gang, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// B: 2 authorities good, 1 authority bad, 2 gang, 1 snitch, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	8: {
		// A: 3 authorities good, 4 gang, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// B: 3 authorities (2 good, 1 bad), 3 gang, 1 snitch, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	9: {
		// 8A + 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// 8B + 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	10: {
		// A: 4 authorities (3 good, 1 bad), 4 gang, 1 snitch, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// B: 4 authorities good, 5 gang, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// C: 4 authorities (2 good, 2 bad), 2 gang, 2 snitches, 2 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	11: {
		// 10A + 1 gang.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// 10B + 1 gang.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// 10C + 1 gang.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
	12: {
		// A: 5 authorities (3 good, 2 bad), 4 gang, 2 snitches, 1 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
		// B: 5 authorities good, 7 gang.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
		},
		// C: 5 authorities (3 good, 2 bad), 3 gang, 2 snitches, 2 neutral.
		{
			{Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			{Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
		},
	},
}
