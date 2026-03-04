package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"prison-break/internal/release/smoke"
)

func main() {
	var (
		matches         int
		playersPerMatch int
		tickRateHz      uint
		timeout         time.Duration
	)
	flag.IntVar(&matches, "matches", 4, "number of matches to create")
	flag.IntVar(&playersPerMatch, "players", 4, "players per match")
	flag.UintVar(&tickRateHz, "tickrate", 30, "tick rate for smoke simulation")
	flag.DurationVar(&timeout, "timeout", 5*time.Second, "max smoke validation duration")
	flag.Parse()

	result, err := smoke.Run(context.Background(), smoke.Config{
		Matches:         matches,
		PlayersPerMatch: playersPerMatch,
		TickRateHz:      uint32(tickRateHz),
		Timeout:         timeout,
	})
	if err != nil {
		log.Fatalf("smoke run failed: %v", err)
	}

	fmt.Printf(
		"smoke passed: matches_created=%d matches_started=%d running_matches=%d\n",
		result.MatchesCreated,
		result.MatchesStarted,
		result.RunningMatches,
	)
}
