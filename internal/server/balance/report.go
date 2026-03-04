package balance

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

const (
	DefaultWinRateTarget           = 0.50
	MaxAllowedWinRateDeltaFromFair = 0.15
)

type Report struct {
	GeneratedAt time.Time `json:"generated_at"`

	MatchCount          int `json:"match_count"`
	CompletedMatchCount int `json:"completed_match_count"`

	PacingTargets PacingTargets `json:"pacing_targets"`

	AverageMatchDurationSeconds    float64 `json:"average_match_duration_seconds"`
	AverageCycles                  float64 `json:"average_cycles"`
	AverageFireCommandsPerMatch    float64 `json:"average_fire_commands_per_match"`
	AverageMarketPurchasesPerMatch float64 `json:"average_market_purchases_per_match"`

	WinnerFactionCounts   map[model.FactionType]int     `json:"winner_faction_counts"`
	WinnerFactionWinRates map[model.FactionType]float64 `json:"winner_faction_win_rates"`

	CommandUsage         map[model.InputCommandType]uint64 `json:"command_usage"`
	WeaponUsage          map[model.ItemType]uint64         `json:"weapon_usage"`
	MarketPurchaseCounts map[model.ItemType]uint64         `json:"market_purchase_counts"`

	Recommendations []string `json:"recommendations"`
}

type PacingTargets struct {
	DayPhaseSeconds   uint16 `json:"day_phase_seconds"`
	NightPhaseSeconds uint16 `json:"night_phase_seconds"`
	MaxCycles         uint8  `json:"max_cycles"`
}

func BuildReport(
	generatedAt time.Time,
	matchSnapshots []game.MatchSnapshot,
	fullStateByMatch map[model.MatchID]model.GameState,
	replayByMatch map[model.MatchID]game.MatchReplay,
) Report {
	report := Report{
		GeneratedAt: generatedAt.UTC(),
		MatchCount:  len(matchSnapshots),
		PacingTargets: PacingTargets{
			DayPhaseSeconds:   constants.DayPhaseDurationSeconds,
			NightPhaseSeconds: constants.NightPhaseDurationSeconds,
			MaxCycles:         constants.MaxDayNightCycles,
		},
		WinnerFactionCounts:   make(map[model.FactionType]int, 2),
		WinnerFactionWinRates: make(map[model.FactionType]float64, 2),
		CommandUsage:          make(map[model.InputCommandType]uint64, 16),
		WeaponUsage:           make(map[model.ItemType]uint64, 8),
		MarketPurchaseCounts:  make(map[model.ItemType]uint64, 8),
		Recommendations:       make([]string, 0, 8),
	}

	var (
		durationSecondsTotal float64
		durationSamples      int
		cyclesTotal          int
		cycleSamples         int
		fireCommandsTotal    uint64
		marketBuyTotal       uint64
	)

	for _, snapshot := range matchSnapshots {
		matchID := snapshot.MatchID
		fullState, hasState := fullStateByMatch[matchID]
		replay, hasReplay := replayByMatch[matchID]

		if snapshot.Status == model.MatchStatusGameOver {
			report.CompletedMatchCount++
		}

		if hasState && fullState.GameOver != nil {
			winnerFactions := winnerFactionsForState(fullState)
			for faction := range winnerFactions {
				report.WinnerFactionCounts[faction]++
			}
		}

		if hasState {
			cyclesTotal += int(fullState.CycleCount)
			cycleSamples++
		}

		if durationSeconds, ok := matchDurationSeconds(snapshot, fullState, hasState, replay, hasReplay); ok {
			durationSecondsTotal += durationSeconds
			durationSamples++
		}

		if !hasReplay {
			continue
		}

		for _, entry := range replay.Entries {
			report.CommandUsage[entry.Command.Type]++
			switch entry.Command.Type {
			case model.CmdFireWeapon:
				fireCommandsTotal++
				var payload model.FireWeaponPayload
				if err := json.Unmarshal(entry.Command.Payload, &payload); err == nil && payload.Weapon != "" {
					report.WeaponUsage[payload.Weapon]++
				}
			case model.CmdBlackMarketBuy:
				marketBuyTotal++
				var payload model.BlackMarketPurchasePayload
				if err := json.Unmarshal(entry.Command.Payload, &payload); err == nil && payload.Item != "" {
					report.MarketPurchaseCounts[payload.Item]++
				}
			}
		}
	}

	if durationSamples > 0 {
		report.AverageMatchDurationSeconds = durationSecondsTotal / float64(durationSamples)
	}
	if cycleSamples > 0 {
		report.AverageCycles = float64(cyclesTotal) / float64(cycleSamples)
	}
	if report.MatchCount > 0 {
		report.AverageFireCommandsPerMatch = float64(fireCommandsTotal) / float64(report.MatchCount)
		report.AverageMarketPurchasesPerMatch = float64(marketBuyTotal) / float64(report.MatchCount)
	}
	if report.CompletedMatchCount > 0 {
		for faction, wins := range report.WinnerFactionCounts {
			report.WinnerFactionWinRates[faction] = float64(wins) / float64(report.CompletedMatchCount)
		}
	}

	report.Recommendations = buildRecommendations(report)
	return report
}

func winnerFactionsForState(state model.GameState) map[model.FactionType]struct{} {
	out := make(map[model.FactionType]struct{}, 2)
	if state.GameOver == nil || len(state.GameOver.WinnerPlayerIDs) == 0 {
		return out
	}

	factionByPlayer := make(map[model.PlayerID]model.FactionType, len(state.Players))
	for _, player := range state.Players {
		if player.ID != "" && player.Faction != "" {
			factionByPlayer[player.ID] = player.Faction
		}
	}
	for _, winnerID := range state.GameOver.WinnerPlayerIDs {
		if faction, exists := factionByPlayer[winnerID]; exists && faction != "" {
			out[faction] = struct{}{}
		}
	}
	return out
}

func matchDurationSeconds(
	snapshot game.MatchSnapshot,
	fullState model.GameState,
	hasState bool,
	replay game.MatchReplay,
	hasReplay bool,
) (float64, bool) {
	if snapshot.StartedAt != nil && snapshot.EndedAt != nil {
		duration := snapshot.EndedAt.Sub(*snapshot.StartedAt).Seconds()
		if duration > 0 {
			return duration, true
		}
	}
	if hasReplay && replay.StartedAt != nil && replay.EndedAt != nil {
		duration := replay.EndedAt.Sub(*replay.StartedAt).Seconds()
		if duration > 0 {
			return duration, true
		}
	}

	if hasReplay && replay.TickRateHz > 0 {
		endTick := snapshot.TickID
		if hasState && fullState.TickID > endTick {
			endTick = fullState.TickID
		}
		if endTick > 0 {
			return float64(endTick) / float64(replay.TickRateHz), true
		}
	}
	return 0, false
}

func buildRecommendations(report Report) []string {
	recommendations := make([]string, 0, 6)
	if report.MatchCount < 5 {
		recommendations = append(
			recommendations,
			"Collect at least 5 completed playtest matches before locking balance decisions.",
		)
	}

	authorityRate := report.WinnerFactionWinRates[model.FactionAuthority]
	prisonerRate := report.WinnerFactionWinRates[model.FactionPrisoner]
	if authorityRate > 0 || prisonerRate > 0 {
		delta := math.Abs(authorityRate - prisonerRate)
		if delta > MaxAllowedWinRateDeltaFromFair {
			strongerSide := model.FactionAuthority
			if prisonerRate > authorityRate {
				strongerSide = model.FactionPrisoner
			}
			recommendations = append(
				recommendations,
				fmt.Sprintf(
					"Win-rate imbalance detected (authority=%.2f prisoner=%.2f). Nerf %s advantage or buff counterplay.",
					authorityRate,
					prisonerRate,
					strongerSide,
				),
			)
		}
	}

	if report.AverageCycles > 0 {
		if report.AverageCycles < float64(constants.MaxDayNightCycles)*0.45 {
			recommendations = append(
				recommendations,
				"Matches end too quickly; reduce early lethality or increase defensive access.",
			)
		}
		if report.AverageCycles > float64(constants.MaxDayNightCycles)*0.90 {
			recommendations = append(
				recommendations,
				"Matches trend too long; increase escape pressure or reduce stall mechanics.",
			)
		}
	}

	if report.AverageMarketPurchasesPerMatch < 1.0 {
		recommendations = append(
			recommendations,
			"Black-market engagement is low; tune money-card availability or offer costs.",
		)
	}
	if report.AverageFireCommandsPerMatch < 3.0 {
		recommendations = append(
			recommendations,
			"Combat engagement is low; review map flow and objective incentives to create conflict.",
		)
	}

	if len(recommendations) == 0 {
		recommendations = append(
			recommendations,
			strings.TrimSpace(
				fmt.Sprintf(
					"Balance is within current guardrails (target win delta <= %.2f from %.2f baseline). Continue structured playtests.",
					MaxAllowedWinRateDeltaFromFair,
					DefaultWinRateTarget,
				),
			),
		)
	}
	return recommendations
}
