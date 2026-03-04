package smoke

import (
	"context"
	"testing"
	"time"
)

func TestRunStartsRequestedMatches(t *testing.T) {
	result, err := Run(context.Background(), Config{
		Matches:         3,
		PlayersPerMatch: 2,
		TickRateHz:      30,
		Timeout:         2 * time.Second,
	})
	if err != nil {
		t.Fatalf("smoke run failed: %v", err)
	}
	if result.MatchesCreated != 3 || result.MatchesStarted != 3 || result.RunningMatches != 3 {
		t.Fatalf("unexpected smoke run result: %+v", result)
	}
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	_, err := Run(context.Background(), Config{Matches: 0, PlayersPerMatch: 2})
	if err == nil {
		t.Fatalf("expected invalid config error for zero matches")
	}

	_, err = Run(context.Background(), Config{Matches: 1, PlayersPerMatch: 0})
	if err == nil {
		t.Fatalf("expected invalid config error for zero players")
	}
}
