package render

import (
	"strings"
	"testing"

	"prison-break/internal/shared/model"
)

func TestBuildHUDLinesIncludesPhaseHealthAmmoAndCooldowns(t *testing.T) {
	state := model.GameState{
		MatchID:    "m-1",
		TickID:     50,
		Status:     model.MatchStatusRunning,
		CycleCount: 2,
		Phase: model.PhaseState{
			Current:     model.PhaseNight,
			StartedTick: 40,
			EndsTick:    70,
		},
		Map: model.MapState{
			PowerOn:           false,
			BlackMarketRoomID: "courtyard",
			Alarm: model.AlarmState{
				Active:   true,
				EndsTick: 55,
			},
		},
		Players: []model.PlayerState{
			{
				ID:               "p1",
				Name:             "Local",
				Faction:          model.FactionPrisoner,
				Role:             model.RoleGangMember,
				HeartsHalf:       5,
				TempHeartsHalf:   2,
				Bullets:          3,
				CurrentRoomID:    "roof_lookout",
				StunnedUntilTick: 54,
				Effects: []model.EffectState{
					{Effect: model.EffectSpeedBoost, EndsTick: 58, Stacks: 2},
				},
				LastActionFeedback: model.ActionFeedback{
					Kind:    model.ActionFeedbackKindCombat,
					Level:   model.ActionFeedbackLevelWarning,
					Message: "Hit by guard.",
					TickID:  49,
				},
			},
		},
	}

	lines := BuildHUDLines(state, "p1")
	if len(lines) < 6 {
		t.Fatalf("expected rich HUD output lines, got %v", lines)
	}

	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Phase night")
	assertContains(t, joined, "Health 2.5")
	assertContains(t, joined, "Ammo 3")
	assertContains(t, joined, "Objective")
	assertContains(t, joined, "Effects speed_boost(x2)")
	assertContains(t, joined, "stunned:4t")
	assertContains(t, joined, "speed_boost:8t(x2)")
	assertContains(t, joined, "Power OFF")
	assertContains(t, joined, "Controls[Desktop]")
	assertContains(t, joined, "Panels Tab/C/V/B/X")
	assertContains(t, joined, "Escape ")
	assertContains(t, joined, "ObjectiveProgress ")
	assertContains(t, joined, "ActionFeedback combat:warning @49 (Hit by guard.)")
}

func TestBuildHUDLinesHandlesMissingLocalPlayer(t *testing.T) {
	state := model.GameState{
		MatchID: "m-2",
		TickID:  1,
		Status:  model.MatchStatusRunning,
		Map: model.MapState{
			PowerOn: true,
		},
	}

	lines := BuildHUDLines(state, "missing")
	joined := strings.Join(lines, " | ")

	assertContains(t, joined, "Local player \"missing\" not in snapshot")
	assertContains(t, joined, "Objective Rejoin your match session")
	assertContains(t, joined, "Health --")
	assertContains(t, joined, "Effects --")
	assertContains(t, joined, "Cooldowns --")
}

func TestBuildHUDLinesUsesRoleSafeSpectatorView(t *testing.T) {
	state := model.GameState{
		MatchID: "m-spec",
		TickID:  14,
		Status:  model.MatchStatusRunning,
		Players: []model.PlayerState{
			{
				ID:      "warden-1",
				Name:    "Warden",
				Role:    model.RoleWarden,
				Faction: model.FactionAuthority,
			},
		},
	}

	lines := BuildHUDLinesWithOptions(state, "", HUDOptions{
		ShowDesktopActionHints:  true,
		SpectatorFollowPlayerID: "warden-1",
		SpectatorFollowSlot:     1,
		SpectatorSlotCount:      1,
	})
	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Spectator View")
	assertContains(t, joined, "Follow warden-1 (slot 1/1)")
	assertContains(t, joined, "Objective Observe the match")
	assertContains(t, joined, "SpectatorControls")
	assertNotContains(t, joined, "Role warden")
}

func TestBuildHUDLinesWithMobileHintsIncludesMobileControls(t *testing.T) {
	state := model.GameState{
		MatchID: "m-mobile",
		TickID:  3,
		Status:  model.MatchStatusRunning,
		Players: []model.PlayerState{
			{
				ID:      "p1",
				Name:    "Local",
				Faction: model.FactionAuthority,
				Role:    model.RoleDeputy,
			},
		},
	}

	lines := BuildHUDLinesWithOptions(state, "p1", HUDOptions{
		ShowDesktopActionHints: true,
		ShowMobileActionHints:  true,
	})
	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Controls[Desktop]")
	assertContains(t, joined, "Controls[Mobile]")
}

func TestBuildHUDLinesRoleAlignmentVariantDeputyEvilObjective(t *testing.T) {
	state := model.GameState{
		MatchID:    "m-role",
		TickID:     9,
		Status:     model.MatchStatusRunning,
		CycleCount: 1,
		Phase: model.PhaseState{
			Current: model.PhaseDay,
		},
		Players: []model.PlayerState{
			{
				ID:        "p1",
				Name:      "Deputy",
				Alive:     true,
				Role:      model.RoleDeputy,
				Faction:   model.FactionAuthority,
				Alignment: model.AlignmentEvil,
			},
			{
				ID:            "leader",
				Alive:         true,
				Role:          model.RoleGangLeader,
				Faction:       model.FactionPrisoner,
				Alignment:     model.AlignmentEvil,
				CurrentRoomID: "escaped",
			},
		},
	}

	lines := BuildHUDLines(state, "p1")
	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Secret objective: aid prisoner breakout while preserving cover.")
	assertContains(t, joined, "ObjectiveProgress cover_alive=yes, gang_leader_escaped=yes")
}

func TestBuildCompactHUDLinesReturnsMinimalOverlay(t *testing.T) {
	state := model.GameState{
		MatchID:    "m-compact",
		TickID:     5,
		Status:     model.MatchStatusRunning,
		CycleCount: 2,
		Phase: model.PhaseState{
			Current: model.PhaseNight,
		},
		Players: []model.PlayerState{
			{
				ID:      "p1",
				Faction: model.FactionPrisoner,
			},
		},
	}

	lines := BuildHUDLinesWithOptions(state, "p1", HUDOptions{
		ShowVerboseDetails: false,
		PingMS:             48,
	})
	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Faction prisoner")
	assertContains(t, joined, "Phase night")
	assertContains(t, joined, "Ping 48ms")
	assertNotContains(t, joined, "Controls[Desktop]")
	assertNotContains(t, joined, "Objective")
}

func TestFormatHearts(t *testing.T) {
	if got := formatHearts(6); got != "3" {
		t.Fatalf("expected 6 half-hearts => 3, got %s", got)
	}
	if got := formatHearts(7); got != "3.5" {
		t.Fatalf("expected 7 half-hearts => 3.5, got %s", got)
	}
	if got := formatHearts(1); got != "0.5" {
		t.Fatalf("expected 1 half-heart => 0.5, got %s", got)
	}
}

func assertContains(t *testing.T, text string, part string) {
	t.Helper()
	if !strings.Contains(text, part) {
		t.Fatalf("expected substring %q in %q", part, text)
	}
}

func assertNotContains(t *testing.T, text string, part string) {
	t.Helper()
	if strings.Contains(text, part) {
		t.Fatalf("did not expect substring %q in %q", part, text)
	}
}
