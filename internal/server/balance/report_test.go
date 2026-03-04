package balance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
)

func TestBuildReportAggregatesWinRatesPacingAndCommandUsage(t *testing.T) {
	start := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	endTwo := start.Add(4 * time.Minute)

	matchSnapshots := []game.MatchSnapshot{
		{
			MatchID:   "m-1",
			Status:    model.MatchStatusGameOver,
			TickID:    9000,
			CreatedAt: start.Add(-time.Minute),
			StartedAt: &start,
			EndedAt:   &end,
		},
		{
			MatchID:   "m-2",
			Status:    model.MatchStatusGameOver,
			TickID:    7200,
			CreatedAt: start.Add(-2 * time.Minute),
			StartedAt: &start,
			EndedAt:   &endTwo,
		},
	}

	fullStateByMatch := map[model.MatchID]model.GameState{
		"m-1": {
			MatchID:    "m-1",
			TickID:     9000,
			CycleCount: 6,
			Players: []model.PlayerState{
				{ID: "a1", Faction: model.FactionAuthority},
				{ID: "p1", Faction: model.FactionPrisoner},
			},
			GameOver: &model.GameOverState{
				Reason:          model.WinReasonMaxCyclesReached,
				EndedTick:       9000,
				WinnerPlayerIDs: []model.PlayerID{"a1"},
			},
		},
		"m-2": {
			MatchID:    "m-2",
			TickID:     7200,
			CycleCount: 3,
			Players: []model.PlayerState{
				{ID: "a2", Faction: model.FactionAuthority},
				{ID: "p2", Faction: model.FactionPrisoner},
			},
			GameOver: &model.GameOverState{
				Reason:          model.WinReasonGangLeaderEscaped,
				EndedTick:       7200,
				WinnerPlayerIDs: []model.PlayerID{"p2"},
			},
		},
	}

	replayByMatch := map[model.MatchID]game.MatchReplay{
		"m-1": {
			MatchID:    "m-1",
			Status:     model.MatchStatusGameOver,
			TickRateHz: 30,
			Entries: []game.ReplayEntry{
				{
					Command: model.InputCommand{
						Type:    model.CmdFireWeapon,
						Payload: mustRawJSON(t, model.FireWeaponPayload{Weapon: model.ItemPistol}),
					},
				},
				{
					Command: model.InputCommand{
						Type:    model.CmdBlackMarketBuy,
						Payload: mustRawJSON(t, model.BlackMarketPurchasePayload{Item: model.ItemShiv}),
					},
				},
			},
		},
		"m-2": {
			MatchID:    "m-2",
			Status:     model.MatchStatusGameOver,
			TickRateHz: 30,
			Entries: []game.ReplayEntry{
				{
					Command: model.InputCommand{
						Type:    model.CmdFireWeapon,
						Payload: mustRawJSON(t, model.FireWeaponPayload{Weapon: model.ItemType("baton")}),
					},
				},
				{
					Command: model.InputCommand{
						Type:    model.CmdFireWeapon,
						Payload: mustRawJSON(t, model.FireWeaponPayload{Weapon: model.ItemPistol}),
					},
				},
			},
		},
	}

	report := BuildReport(time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC), matchSnapshots, fullStateByMatch, replayByMatch)

	if report.MatchCount != 2 || report.CompletedMatchCount != 2 {
		t.Fatalf("expected match and completed counts 2/2, got %d/%d", report.MatchCount, report.CompletedMatchCount)
	}
	if report.WinnerFactionCounts[model.FactionAuthority] != 1 || report.WinnerFactionCounts[model.FactionPrisoner] != 1 {
		t.Fatalf("expected winner faction counts authority/prisoner = 1/1, got %+v", report.WinnerFactionCounts)
	}
	if report.WinnerFactionWinRates[model.FactionAuthority] != 0.5 || report.WinnerFactionWinRates[model.FactionPrisoner] != 0.5 {
		t.Fatalf("expected winner faction win rates authority/prisoner = 0.5/0.5, got %+v", report.WinnerFactionWinRates)
	}
	if report.CommandUsage[model.CmdFireWeapon] != 3 {
		t.Fatalf("expected three fire commands aggregated, got %d", report.CommandUsage[model.CmdFireWeapon])
	}
	if report.WeaponUsage[model.ItemPistol] != 2 || report.WeaponUsage[model.ItemType("baton")] != 1 {
		t.Fatalf("expected weapon usage pistol=2 baton=1, got %+v", report.WeaponUsage)
	}
	if report.MarketPurchaseCounts[model.ItemShiv] != 1 {
		t.Fatalf("expected shiv market purchase count 1, got %+v", report.MarketPurchaseCounts)
	}
	if report.AverageCycles != 4.5 {
		t.Fatalf("expected average cycles 4.5, got %f", report.AverageCycles)
	}
	if report.AverageMatchDurationSeconds < 269 || report.AverageMatchDurationSeconds > 271 {
		t.Fatalf("expected average duration near 270 seconds, got %f", report.AverageMatchDurationSeconds)
	}
	if report.AverageMarketPurchasesPerMatch != 0.5 {
		t.Fatalf("expected average market purchases per match 0.5, got %f", report.AverageMarketPurchasesPerMatch)
	}
	if len(report.Recommendations) == 0 {
		t.Fatalf("expected non-empty recommendations")
	}
}

func TestBuildReportWithEmptyInputsReturnsActionableRecommendation(t *testing.T) {
	report := BuildReport(time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC), nil, nil, nil)
	if report.MatchCount != 0 || report.CompletedMatchCount != 0 {
		t.Fatalf("expected zero counts for empty report input, got %d/%d", report.MatchCount, report.CompletedMatchCount)
	}
	if len(report.Recommendations) == 0 {
		t.Fatalf("expected recommendation for empty report")
	}
	joined := strings.Join(report.Recommendations, " | ")
	if !strings.Contains(joined, "Collect at least 5 completed playtest matches") {
		t.Fatalf("expected recommendation requesting more playtest matches, got %q", joined)
	}
}

func mustRawJSON(t *testing.T, payload any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}
