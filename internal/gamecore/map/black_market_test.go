package gamemap

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestNightlyBlackMarketCandidatesAreDeterministicAndValid(t *testing.T) {
	candidates := NightlyBlackMarketCandidates()
	if len(candidates) == 0 {
		t.Fatalf("expected at least one nightly black market candidate")
	}

	for _, roomID := range candidates {
		if !IsNightlyBlackMarketCandidate(roomID) {
			t.Fatalf("expected candidate room %q to be recognized", roomID)
		}
	}
	if IsNightlyBlackMarketCandidate(RoomWardenHQ) {
		t.Fatalf("expected warden_hq to be rejected as a nightly black market candidate")
	}
}

func TestDeterministicNightlyBlackMarketRoomIsStableForSameInputs(t *testing.T) {
	matchID := model.MatchID("match-000123")
	first := DeterministicNightlyBlackMarketRoom(matchID, 2, 90)
	second := DeterministicNightlyBlackMarketRoom(matchID, 2, 90)
	if first != second {
		t.Fatalf("expected stable nightly room for same inputs, got %q and %q", first, second)
	}
	if !IsNightlyBlackMarketCandidate(first) {
		t.Fatalf("expected selected room %q to be in candidate set", first)
	}
}

func TestDeterministicNightlyBlackMarketRoomChangesAcrossNightInputs(t *testing.T) {
	matchID := model.MatchID("match-000123")
	rooms := []model.RoomID{
		DeterministicNightlyBlackMarketRoom(matchID, 0, 30),
		DeterministicNightlyBlackMarketRoom(matchID, 0, 60),
		DeterministicNightlyBlackMarketRoom(matchID, 1, 60),
	}

	for _, roomID := range rooms {
		if !IsNightlyBlackMarketCandidate(roomID) {
			t.Fatalf("expected selected room %q to be in candidate set", roomID)
		}
	}

	allEqual := rooms[0] == rooms[1] && rooms[1] == rooms[2]
	if allEqual {
		t.Fatalf("expected varying night inputs to produce at least one different room, got %v", rooms)
	}
}
