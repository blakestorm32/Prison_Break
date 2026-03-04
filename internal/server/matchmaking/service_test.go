package matchmaking

import (
	"fmt"
	"testing"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
)

type fakeManager struct {
	minPlayers uint8
	maxPlayers uint8

	snapshots []game.MatchSnapshot
	nextID    uint64
}

func (m *fakeManager) CreateMatch() game.MatchSnapshot {
	m.nextID++
	snapshot := game.MatchSnapshot{
		MatchID:   model.MatchID(fmt.Sprintf("created-%06d", m.nextID)),
		Status:    model.MatchStatusLobby,
		CreatedAt: time.Date(2026, 2, 22, 12, 0, 0, int(m.nextID), time.UTC),
	}
	m.snapshots = append(m.snapshots, snapshot)
	return snapshot
}

func (m *fakeManager) ListMatchSnapshots() []game.MatchSnapshot {
	out := make([]game.MatchSnapshot, len(m.snapshots))
	copy(out, m.snapshots)
	return out
}

func (m *fakeManager) MatchConstraints() (uint8, uint8) {
	return m.minPlayers, m.maxPlayers
}

func TestFindOrCreateLobbyPrefersMostPopulatedJoinableLobby(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 3,
		maxPlayers: 6,
		snapshots: []game.MatchSnapshot{
			{
				MatchID:   "lobby-a",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
				},
			},
			{
				MatchID:   "lobby-b",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 2, 22, 12, 1, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p2"},
					{PlayerID: "p3"},
				},
			},
			{
				MatchID:   "running-x",
				Status:    model.MatchStatusRunning,
				CreatedAt: time.Date(2026, 2, 22, 11, 59, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "r1"},
					{PlayerID: "r2"},
					{PlayerID: "r3"},
				},
			},
		},
	}

	service := NewService(manager)
	lobby := service.FindOrCreateLobby()

	if lobby.MatchID != "lobby-b" {
		t.Fatalf("expected to pick most populated joinable lobby, got %s", lobby.MatchID)
	}
	if !lobby.Joinable {
		t.Fatalf("expected selected lobby to be joinable")
	}
	if lobby.Region != DefaultRegionID {
		t.Fatalf("expected default region %q, got %q", DefaultRegionID, lobby.Region)
	}
}

func TestFindOrCreateLobbyCreatesWhenNoJoinableLobbyExists(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 2,
		snapshots: []game.MatchSnapshot{
			{
				MatchID:   "running-a",
				Status:    model.MatchStatusRunning,
				CreatedAt: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
					{PlayerID: "p2"},
				},
			},
			{
				MatchID:   "ended-a",
				Status:    model.MatchStatusGameOver,
				CreatedAt: time.Date(2026, 2, 22, 12, 1, 0, 0, time.UTC),
			},
		},
	}

	service := NewService(manager)
	lobby := service.FindOrCreateLobby()

	if lobby.MatchID == "" {
		t.Fatalf("expected created lobby to have a match id")
	}
	if lobby.Status != model.MatchStatusLobby {
		t.Fatalf("expected created lobby status to be lobby, got %s", lobby.Status)
	}
	if !lobby.Joinable {
		t.Fatalf("expected created lobby to be joinable")
	}
	if lobby.Region != DefaultRegionID {
		t.Fatalf("expected created lobby to use default region %q, got %q", DefaultRegionID, lobby.Region)
	}
}

func TestListLobbiesSortsJoinableBeforeNonJoinableAndByPopulation(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 3,
		maxPlayers: 3,
		snapshots: []game.MatchSnapshot{
			{
				MatchID:   "lobby-low",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
				},
			},
			{
				MatchID:   "lobby-high",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 2, 22, 12, 1, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
					{PlayerID: "p2"},
				},
			},
			{
				MatchID:   "lobby-full",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 2, 22, 12, 2, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
					{PlayerID: "p2"},
					{PlayerID: "p3"},
				},
			},
		},
	}

	service := NewService(manager)
	lobbies := service.ListLobbies(false)

	if len(lobbies) != 3 {
		t.Fatalf("expected 3 lobby entries, got %d", len(lobbies))
	}
	if lobbies[0].MatchID != "lobby-high" {
		t.Fatalf("expected highest-population joinable lobby first, got %s", lobbies[0].MatchID)
	}
	if lobbies[1].MatchID != "lobby-low" {
		t.Fatalf("expected lower-population joinable lobby second, got %s", lobbies[1].MatchID)
	}
	if lobbies[2].MatchID != "lobby-full" {
		t.Fatalf("expected full lobby last (non-joinable), got %s", lobbies[2].MatchID)
	}
	if lobbies[2].Joinable {
		t.Fatalf("expected full lobby to be marked non-joinable")
	}
}

func TestFindOrCreateLobbyForRequestCreatesPreferredRegionLobby(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 6,
	}

	service := NewService(manager)
	lobby := service.FindOrCreateLobbyForRequest(QueueRequest{
		PlayerID:        "p1",
		PreferredRegion: "US-East",
	})

	if lobby.Region != "us-east" {
		t.Fatalf("expected preferred region us-east, got %q", lobby.Region)
	}
}

func TestFindOrCreateLobbyForRequestUsesLowestLatencyRegionWhenNoPreference(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 6,
	}

	service := NewService(manager)
	lobby := service.FindOrCreateLobbyForRequest(QueueRequest{
		PlayerID: "p1",
		RegionLatencyMS: map[string]uint16{
			"us-west": 45,
			"us-east": 70,
		},
	})
	if lobby.Region != "us-west" {
		t.Fatalf("expected lowest latency region us-west, got %q", lobby.Region)
	}
}

func TestListLobbiesForRequestPrioritizesPreferredRegionThenLatency(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 6,
		snapshots: []game.MatchSnapshot{
			{
				MatchID:   "lobby-us-east",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
				Players:   []game.PlayerSession{{PlayerID: "p1"}},
			},
			{
				MatchID:   "lobby-us-west",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 3, 4, 10, 0, 1, 0, time.UTC),
				Players:   []game.PlayerSession{{PlayerID: "p2"}},
			},
			{
				MatchID:   "lobby-eu",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 3, 4, 10, 0, 2, 0, time.UTC),
				Players:   []game.PlayerSession{{PlayerID: "p3"}},
			},
		},
	}

	service := NewService(manager)
	service.matchRegions["lobby-us-east"] = "us-east"
	service.matchRegions["lobby-us-west"] = "us-west"
	service.matchRegions["lobby-eu"] = "eu-central"

	lobbies := service.ListLobbiesForRequest(false, QueueRequest{
		PreferredRegion: "us-east",
		RegionLatencyMS: map[string]uint16{
			"us-east":    70,
			"us-west":    40,
			"eu-central": 130,
		},
	})
	if len(lobbies) != 3 {
		t.Fatalf("expected 3 lobbies, got %d", len(lobbies))
	}
	if lobbies[0].Region != "us-east" {
		t.Fatalf("expected preferred region lobby first, got region=%q", lobbies[0].Region)
	}
	if lobbies[1].Region != "us-west" {
		t.Fatalf("expected lower-latency non-preferred region second, got region=%q", lobbies[1].Region)
	}
	if lobbies[2].Region != "eu-central" {
		t.Fatalf("expected highest-latency region last, got region=%q", lobbies[2].Region)
	}
}

func TestFindOrCreateLobbyForRequestSupportsExcludeMatchIDs(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 6,
		snapshots: []game.MatchSnapshot{
			{
				MatchID:   "lobby-a",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p1"},
					{PlayerID: "p2"},
				},
			},
			{
				MatchID:   "lobby-b",
				Status:    model.MatchStatusLobby,
				CreatedAt: time.Date(2026, 3, 4, 10, 0, 1, 0, time.UTC),
				Players: []game.PlayerSession{
					{PlayerID: "p3"},
				},
			},
		},
	}

	service := NewService(manager)
	lobby := service.FindOrCreateLobbyForRequest(QueueRequest{
		PlayerID:        "p4",
		ExcludeMatchIDs: []model.MatchID{"lobby-a"},
	})
	if lobby.MatchID != "lobby-b" {
		t.Fatalf("expected excluded lobby to be skipped, got %s", lobby.MatchID)
	}
}

func TestValidateQueueRequestRejectsInvalidRegionLatencyData(t *testing.T) {
	err := ValidateQueueRequest(QueueRequest{
		PreferredRegion: "bad region!",
	})
	if err == nil {
		t.Fatalf("expected invalid preferred region to be rejected")
	}

	err = ValidateQueueRequest(QueueRequest{
		RegionLatencyMS: map[string]uint16{
			"us-east": 0,
		},
	})
	if err == nil {
		t.Fatalf("expected zero latency entry to be rejected")
	}
}

func TestQueueMetricsTrackQueuedAndAllocatedEntries(t *testing.T) {
	manager := &fakeManager{
		minPlayers: 2,
		maxPlayers: 6,
	}

	service := NewService(manager)
	_ = service.FindOrCreateLobbyForRequest(QueueRequest{PlayerID: "p1", PreferredRegion: "us-east"})
	_ = service.FindOrCreateLobbyForRequest(QueueRequest{PlayerID: "p2", PreferredRegion: "us-east"})

	metrics := service.QueueMetrics()
	if metrics.QueuedTotal != 2 {
		t.Fatalf("expected queued total 2, got %d", metrics.QueuedTotal)
	}
	if metrics.AllocatedTotal != 2 {
		t.Fatalf("expected allocated total 2, got %d", metrics.AllocatedTotal)
	}
	if metrics.CurrentDepth != 0 {
		t.Fatalf("expected current queue depth 0 after allocations, got %d", metrics.CurrentDepth)
	}
	if metrics.MaxDepth < 1 {
		t.Fatalf("expected max queue depth at least 1, got %d", metrics.MaxDepth)
	}
}
