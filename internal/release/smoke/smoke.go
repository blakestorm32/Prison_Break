package smoke

import (
	"context"
	"errors"
	"fmt"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type Config struct {
	Matches         int
	PlayersPerMatch int
	TickRateHz      uint32
	Timeout         time.Duration
}

type Result struct {
	MatchesCreated int
	MatchesStarted int
	RunningMatches int
}

func Run(ctx context.Context, config Config) (Result, error) {
	normalized, err := normalizeConfig(config)
	if err != nil {
		return Result{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	manager := game.NewManager(game.Config{
		MinPlayers:    uint8(normalized.PlayersPerMatch),
		MaxPlayers:    uint8(normalized.PlayersPerMatch),
		TickRateHz:    normalized.TickRateHz,
		MatchIDPrefix: "smoke",
	})
	defer manager.Close()

	matchIDs := make([]model.MatchID, 0, normalized.Matches)
	for matchIndex := 0; matchIndex < normalized.Matches; matchIndex++ {
		match := manager.CreateMatch()
		matchIDs = append(matchIDs, match.MatchID)

		for playerIndex := 0; playerIndex < normalized.PlayersPerMatch; playerIndex++ {
			playerID := model.PlayerID(fmt.Sprintf("smoke-m%02d-p%02d", matchIndex, playerIndex))
			if _, joinErr := manager.JoinMatch(match.MatchID, playerID, string(playerID)); joinErr != nil {
				return Result{}, fmt.Errorf("join %s to %s: %w", playerID, match.MatchID, joinErr)
			}
		}

		if _, startErr := manager.StartMatch(match.MatchID); startErr != nil {
			return Result{}, fmt.Errorf("start match %s: %w", match.MatchID, startErr)
		}
	}

	deadline := time.Now().Add(normalized.Timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		runningMatches := 0
		for _, matchID := range matchIDs {
			snapshot, exists := manager.MatchSnapshot(matchID)
			if exists && snapshot.Status == model.MatchStatusRunning {
				runningMatches++
			}
		}
		if runningMatches == len(matchIDs) {
			return Result{
				MatchesCreated: len(matchIDs),
				MatchesStarted: len(matchIDs),
				RunningMatches: runningMatches,
			}, nil
		}

		time.Sleep(20 * time.Millisecond)
	}

	return Result{}, errors.New("smoke run timed out before all matches reached running status")
}

func normalizeConfig(config Config) (Config, error) {
	out := config
	if out.Matches <= 0 {
		return Config{}, errors.New("matches must be > 0")
	}
	if out.PlayersPerMatch <= 0 {
		return Config{}, errors.New("players_per_match must be > 0")
	}
	if out.PlayersPerMatch > int(constants.MaxPlayers) {
		return Config{}, fmt.Errorf("players_per_match must be <= %d", constants.MaxPlayers)
	}
	if out.TickRateHz == 0 {
		out.TickRateHz = 30
	}
	if out.Timeout <= 0 {
		out.Timeout = 3 * time.Second
	}
	return out, nil
}
