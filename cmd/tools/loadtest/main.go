package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
)

func main() {
	var (
		matchCount      int
		playersPerMatch int
		simSeconds      int
		tickRateHz      uint
	)
	flag.IntVar(&matchCount, "matches", 12, "number of matches to create")
	flag.IntVar(&playersPerMatch, "players", 8, "players per match")
	flag.IntVar(&simSeconds, "seconds", 10, "simulation runtime in seconds")
	flag.UintVar(&tickRateHz, "tickrate", 30, "server tick rate")
	flag.Parse()

	if matchCount <= 0 || playersPerMatch <= 0 || simSeconds <= 0 {
		log.Fatalf("matches, players, and seconds must all be positive")
	}

	manager := game.NewManager(game.Config{
		MinPlayers:    uint8(playersPerMatch),
		MaxPlayers:    uint8(playersPerMatch),
		TickRateHz:    uint32(tickRateHz),
		MatchIDPrefix: "loadtest",
	})
	defer manager.Close()

	matchIDs := make([]model.MatchID, 0, matchCount)
	for matchIndex := 0; matchIndex < matchCount; matchIndex++ {
		match := manager.CreateMatch()
		matchIDs = append(matchIDs, match.MatchID)

		for playerIndex := 0; playerIndex < playersPerMatch; playerIndex++ {
			playerID := model.PlayerID(fmt.Sprintf("m%03d-p%02d", matchIndex, playerIndex))
			if _, err := manager.JoinMatch(match.MatchID, playerID, string(playerID)); err != nil {
				log.Fatalf("join player %s to %s failed: %v", playerID, match.MatchID, err)
			}
		}
		if _, err := manager.StartMatch(match.MatchID); err != nil {
			log.Fatalf("start match %s failed: %v", match.MatchID, err)
		}
	}

	start := time.Now()
	time.Sleep(time.Duration(simSeconds) * time.Second)
	elapsed := time.Since(start)

	running := 0
	for _, matchID := range matchIDs {
		snapshot, exists := manager.MatchSnapshot(matchID)
		if exists && snapshot.Status == model.MatchStatusRunning {
			running++
		}
	}

	fmt.Printf(
		"loadtest complete: matches=%d players_per_match=%d runtime=%s running_matches=%d\n",
		matchCount,
		playersPerMatch,
		elapsed.Round(time.Millisecond),
		running,
	)
}
