package gamemap

import (
	"encoding/binary"
	"hash/fnv"

	"prison-break/internal/shared/model"
)

var nightlyBlackMarketCandidates = []model.RoomID{
	RoomBlackMarket,
	RoomCourtyard,
	RoomCafeteria,
	RoomMailRoom,
	RoomRoofLookout,
}

func NightlyBlackMarketCandidates() []model.RoomID {
	out := make([]model.RoomID, len(nightlyBlackMarketCandidates))
	copy(out, nightlyBlackMarketCandidates)
	return out
}

func IsNightlyBlackMarketCandidate(roomID model.RoomID) bool {
	for _, candidate := range nightlyBlackMarketCandidates {
		if candidate == roomID {
			return true
		}
	}
	return false
}

func DeterministicNightlyBlackMarketRoom(
	matchID model.MatchID,
	cycle uint8,
	nightStartTick uint64,
) model.RoomID {
	if len(nightlyBlackMarketCandidates) == 0 {
		return RoomBlackMarket
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))

	var cycleByte [1]byte
	cycleByte[0] = cycle
	_, _ = hasher.Write(cycleByte[:])

	var tickBytes [8]byte
	binary.LittleEndian.PutUint64(tickBytes[:], nightStartTick)
	_, _ = hasher.Write(tickBytes[:])

	index := hasher.Sum64() % uint64(len(nightlyBlackMarketCandidates))
	return nightlyBlackMarketCandidates[index]
}
