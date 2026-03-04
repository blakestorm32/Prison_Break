package matchmaking

import (
	"fmt"
	"testing"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
)

func BenchmarkMatchmakingRegionAllocation(b *testing.B) {
	manager := game.NewManager(game.Config{
		MinPlayers:    2,
		MaxPlayers:    8,
		TickRateHz:    30,
		MatchIDPrefix: "bench-mm",
	})
	service := NewService(manager)
	regions := []string{"us-east", "us-west", "eu-central"}

	// Pre-seed enough lobbies to make region/latency sorting meaningful.
	excluded := make([]model.MatchID, 0, 96)
	for i := 0; i < 96; i++ {
		lobby := service.FindOrCreateLobbyForRequest(QueueRequest{
			PlayerID:        model.PlayerID(fmt.Sprintf("seed-owner-%03d", i)),
			PreferredRegion: regions[i%len(regions)],
			ExcludeMatchIDs: append([]model.MatchID(nil), excluded...),
		})
		excluded = append(excluded, lobby.MatchID)

		for p := 0; p < 3; p++ {
			_, _ = manager.JoinMatch(
				lobby.MatchID,
				model.PlayerID(fmt.Sprintf("seed-%03d-%d", i, p)),
				fmt.Sprintf("seed-%d", p),
			)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		request := QueueRequest{
			PlayerID:        model.PlayerID(fmt.Sprintf("bench-player-%d", i)),
			PreferredRegion: regions[i%len(regions)],
			RegionLatencyMS: map[string]uint16{
				"us-east":    uint16(35 + (i % 12)),
				"us-west":    uint16(50 + (i % 16)),
				"eu-central": uint16(82 + (i % 20)),
			},
		}
		_ = service.FindOrCreateLobbyForRequest(request)
	}
}
