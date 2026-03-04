package render

import (
	"fmt"
	"sort"
	"strings"

	"prison-break/internal/gamecore/escape"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/gamecore/winconditions"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type HUDOptions struct {
	ShowDesktopActionHints  bool
	ShowMobileActionHints   bool
	SpectatorFollowPlayerID model.PlayerID
	SpectatorFollowSlot     int
	SpectatorSlotCount      int
	ShowVerboseDetails      bool
	PingMS                  int
}

func BuildHUDLines(state model.GameState, localPlayerID model.PlayerID) []string {
	return BuildHUDLinesWithOptions(state, localPlayerID, HUDOptions{
		ShowDesktopActionHints: true,
		ShowMobileActionHints:  false,
		ShowVerboseDetails:     true,
		PingMS:                 -1,
	})
}

func BuildHUDLinesWithOptions(state model.GameState, localPlayerID model.PlayerID, options HUDOptions) []string {
	showVerbose := options.ShowVerboseDetails || options.ShowDesktopActionHints || options.ShowMobileActionHints
	if !showVerbose {
		return BuildCompactHUDLines(state, localPlayerID, options)
	}

	lines := make([]string, 0, 16)
	lines = append(lines, fmt.Sprintf("Match %s  Tick %d  Status %s", state.MatchID, state.TickID, state.Status))
	lines = append(
		lines,
		fmt.Sprintf(
			"Phase %s  Cycle %d/%d  Ends@%d",
			state.Phase.Current,
			state.CycleCount,
			constants.MaxDayNightCycles,
			state.Phase.EndsTick,
		),
	)

	alarmText := "OFF"
	if state.Map.Alarm.Active {
		alarmText = fmt.Sprintf("ON (ends@%d)", state.Map.Alarm.EndsTick)
	}
	lines = append(lines, fmt.Sprintf("Power %s  Alarm %s  Market %s", onOff(state.Map.PowerOn), alarmText, roomOrUnknown(state.Map.BlackMarketRoomID)))

	if localPlayerID == "" {
		lines = append(lines, fmt.Sprintf("Spectator View  Players %d", len(state.Players)))
		if options.SpectatorFollowPlayerID != "" {
			lines = append(
				lines,
				fmt.Sprintf(
					"Follow %s (slot %d/%d)",
					options.SpectatorFollowPlayerID,
					options.SpectatorFollowSlot,
					options.SpectatorSlotCount,
				),
			)
		} else {
			lines = append(lines, "Follow auto")
		}
		lines = append(lines, "Objective Observe the match and coordinate using public information.")
		lines = append(lines, "SpectatorControls Q/E or Left/Right switch follow target.")
		lines = append(lines, actionHintLines(options)...)
		return lines
	}

	local, found := playerByID(state.Players, localPlayerID)
	if !found {
		lines = append(lines, fmt.Sprintf("Local player %q not in snapshot", localPlayerID))
		lines = append(lines, "Objective Rejoin your match session to restore local HUD.")
		lines = append(lines, "Health --  Ammo --  Room --")
		lines = append(lines, "Effects --")
		lines = append(lines, "Cooldowns --")
		lines = append(lines, actionHintLines(options)...)
		return lines
	}

	lines = append(lines, fmt.Sprintf("You %s (%s)  Faction %s  Role %s", local.Name, local.ID, local.Faction, local.Role))
	lines = append(lines, "Objective "+objectiveSummary(local))
	lines = append(lines, "ObjectiveProgress "+objectiveProgressSummary(state, local))
	lines = append(lines, fmt.Sprintf("Health %s (+%s temp)  Ammo %d  Room %s", formatHearts(local.HeartsHalf), formatHearts(local.TempHeartsHalf), local.Bullets, roomOrUnknown(local.CurrentRoomID)))
	lines = append(lines, "Effects "+formatEffects(local))
	lines = append(lines, "Cooldowns "+formatCooldowns(state.TickID, local))
	if local.LastActionFeedback.Kind != "" && local.LastActionFeedback.TickID > 0 {
		lines = append(lines, "ActionFeedback "+formatActionFeedback(local.LastActionFeedback))
	}
	if gamemap.IsPrisonerPlayer(local) {
		lines = append(lines, "Escape "+escapeProgressSummary(local, state.Map))
		if local.LastEscapeAttempt.Route != "" && local.LastEscapeAttempt.TickID > 0 {
			lines = append(lines, "EscapeFeedback "+formatEscapeFeedback(local.LastEscapeAttempt))
		}
	}

	if local.HeartsHalf <= 2 {
		lines = append(lines, "Warning Low health - avoid combat and regroup.")
	}
	if local.Bullets == 0 {
		lines = append(lines, "Warning Out of ammo - reload or find ammo pickup.")
	}

	lines = append(lines, actionHintLines(options)...)
	return lines
}

func BuildCompactHUDLines(state model.GameState, localPlayerID model.PlayerID, options HUDOptions) []string {
	lines := make([]string, 0, 4)
	phaseLabel := fmt.Sprintf("Phase %s  Cycle %d/%d", state.Phase.Current, state.CycleCount, constants.MaxDayNightCycles)

	pingLabel := "Ping --"
	if options.PingMS >= 0 {
		pingLabel = fmt.Sprintf("Ping %dms", options.PingMS)
	}

	if localPlayerID == "" {
		lines = append(lines, "Spectator")
		lines = append(lines, phaseLabel)
		lines = append(lines, pingLabel)
		return lines
	}

	local, found := playerByID(state.Players, localPlayerID)
	if !found {
		lines = append(lines, "Faction -- | Role --")
		lines = append(lines, phaseLabel)
		lines = append(lines, pingLabel)
		return lines
	}

	lines = append(lines, fmt.Sprintf("Faction %s | Role %s", local.Faction, local.Role))
	lines = append(lines, phaseLabel)
	lines = append(lines, pingLabel)
	return lines
}

func formatCooldowns(currentTick uint64, player model.PlayerState) string {
	parts := make([]string, 0, len(player.Effects)+2)

	if player.StunnedUntilTick > currentTick {
		parts = append(parts, fmt.Sprintf("stunned:%dt", player.StunnedUntilTick-currentTick))
	}
	if player.SolitaryUntilTick > currentTick {
		parts = append(parts, fmt.Sprintf("solitary:%dt", player.SolitaryUntilTick-currentTick))
	}

	for _, effect := range player.Effects {
		if effect.EndsTick > currentTick {
			if effect.Stacks > 1 {
				parts = append(parts, fmt.Sprintf("%s:%dt(x%d)", effect.Effect, effect.EndsTick-currentTick, effect.Stacks))
			} else {
				parts = append(parts, fmt.Sprintf("%s:%dt", effect.Effect, effect.EndsTick-currentTick))
			}
		}
	}

	sort.Strings(parts)
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func formatEffects(player model.PlayerState) string {
	if len(player.Effects) == 0 {
		return "none"
	}

	parts := make([]string, 0, len(player.Effects))
	for _, effect := range player.Effects {
		label := string(effect.Effect)
		if effect.Stacks > 1 {
			label = fmt.Sprintf("%s(x%d)", effect.Effect, effect.Stacks)
		}
		parts = append(parts, label)
	}

	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func objectiveSummary(local model.PlayerState) string {
	switch local.Role {
	case model.RoleWarden:
		return "Hold control until cycle limit and prevent escapes."
	case model.RoleDeputy:
		if local.Alignment == model.AlignmentEvil {
			return "Secret objective: aid prisoner breakout while preserving cover."
		}
		return "Protect authority control and keep the Warden alive."
	case model.RoleGangLeader:
		return "Coordinate gang objectives and secure escape execution."
	case model.RoleGangMember:
		return "Support gang routes, gather tools, and escape when ready."
	case model.RoleSnitch:
		if local.Alignment == model.AlignmentEvil {
			return "Misdirect authority and support prisoner momentum while surviving."
		}
		return "Assist authority outcomes by gathering information and surviving."
	case model.RoleNeutralPrisoner:
		return "Outlast both sides and exploit the safest escape opportunity."
	}

	switch local.Faction {
	case model.FactionAuthority:
		return "Maintain prison control and prevent successful escapes."
	case model.FactionPrisoner:
		return "Acquire tools and complete an escape route."
	default:
		return "Stay alive and adapt to evolving objectives."
	}
}

type objectiveStats struct {
	cyclesRemaining   int
	aliveAuthority    int
	alivePrisoners    int
	aliveGang         int
	gangLeaderAlive   bool
	gangLeaderEscaped bool
	wardenAlive       bool
	localEscaped      bool
	routesReady       int
	routeCount        int
}

func objectiveProgressSummary(state model.GameState, local model.PlayerState) string {
	stats := computeObjectiveStats(state, local)

	switch local.Role {
	case model.RoleWarden:
		return fmt.Sprintf(
			"cycles_left=%d, you_alive=%s, gang_leader_escaped=%s, alive_gang=%d",
			stats.cyclesRemaining,
			yesNo(local.Alive),
			yesNo(stats.gangLeaderEscaped),
			stats.aliveGang,
		)
	case model.RoleDeputy:
		if local.Alignment == model.AlignmentEvil {
			return fmt.Sprintf(
				"cover_alive=%s, gang_leader_escaped=%s, alive_gang=%d",
				yesNo(local.Alive),
				yesNo(stats.gangLeaderEscaped),
				stats.aliveGang,
			)
		}
		return fmt.Sprintf(
			"warden_alive=%s, authority_alive=%d, alive_gang=%d",
			yesNo(stats.wardenAlive),
			stats.aliveAuthority,
			stats.aliveGang,
		)
	case model.RoleGangLeader:
		return fmt.Sprintf(
			"escaped=%s, gang_alive=%d, routes_ready=%d/%d",
			yesNo(stats.localEscaped),
			stats.aliveGang,
			stats.routesReady,
			stats.routeCount,
		)
	case model.RoleGangMember:
		return fmt.Sprintf(
			"leader_alive=%s, leader_escaped=%s, routes_ready=%d/%d",
			yesNo(stats.gangLeaderAlive),
			yesNo(stats.gangLeaderEscaped),
			stats.routesReady,
			stats.routeCount,
		)
	case model.RoleSnitch:
		if local.Alignment == model.AlignmentEvil {
			return fmt.Sprintf(
				"you_alive=%s, gang_leader_escaped=%s, cycles_left=%d",
				yesNo(local.Alive),
				yesNo(stats.gangLeaderEscaped),
				stats.cyclesRemaining,
			)
		}
		return fmt.Sprintf(
			"you_alive=%s, cycles_left=%d, alive_gang=%d",
			yesNo(local.Alive),
			stats.cyclesRemaining,
			stats.aliveGang,
		)
	case model.RoleNeutralPrisoner:
		return fmt.Sprintf(
			"you_alive=%s, escaped=%s, routes_ready=%d/%d",
			yesNo(local.Alive),
			yesNo(stats.localEscaped),
			stats.routesReady,
			stats.routeCount,
		)
	}

	switch local.Faction {
	case model.FactionAuthority:
		return fmt.Sprintf(
			"cycles_left=%d, authority_alive=%d, alive_gang=%d",
			stats.cyclesRemaining,
			stats.aliveAuthority,
			stats.aliveGang,
		)
	case model.FactionPrisoner:
		return fmt.Sprintf(
			"you_alive=%s, routes_ready=%d/%d, alive_authority=%d",
			yesNo(local.Alive),
			stats.routesReady,
			stats.routeCount,
			stats.aliveAuthority,
		)
	default:
		return fmt.Sprintf(
			"you_alive=%s, escaped=%s, cycles_left=%d",
			yesNo(local.Alive),
			yesNo(stats.localEscaped),
			stats.cyclesRemaining,
		)
	}
}

func actionHintLines(options HUDOptions) []string {
	lines := make([]string, 0, 2)
	if options.ShowDesktopActionHints {
		lines = append(lines, "Controls[Desktop] Move WASD/Arrows | Sprint Shift | Fire Space/LMB | Interact E/F | Ability V | Info I | Reload R | Panels Tab/C/B/X + [ ] + Enter | Menu Esc")
	}
	if options.ShowMobileActionHints {
		lines = append(lines, "Controls[Mobile] Left joystick move | Fire/Use/Ability/Reload buttons + panel tabs/use buttons")
	}
	return lines
}

func escapeProgressSummary(local model.PlayerState, mapState model.MapState) string {
	evaluations := escape.EvaluateAllRoutes(local, mapState)
	if len(evaluations) == 0 {
		return "--"
	}

	parts := make([]string, 0, len(evaluations))
	for _, evaluation := range evaluations {
		state := "blocked"
		if evaluation.CanAttempt {
			state = "ready"
		}
		parts = append(parts, fmt.Sprintf("%s:%s", evaluation.Route, state))
	}
	return strings.Join(parts, ", ")
}

func formatEscapeFeedback(feedback model.EscapeAttemptFeedback) string {
	if feedback.Route == "" || feedback.Status == "" {
		return "none"
	}
	return fmt.Sprintf("%s:%s (%s)", feedback.Route, feedback.Status, feedback.Reason)
}

func formatActionFeedback(feedback model.ActionFeedback) string {
	if feedback.Kind == "" || feedback.Level == "" || feedback.Message == "" || feedback.TickID == 0 {
		return "none"
	}
	return fmt.Sprintf("%s:%s @%d (%s)", feedback.Kind, feedback.Level, feedback.TickID, feedback.Message)
}

func computeObjectiveStats(state model.GameState, local model.PlayerState) objectiveStats {
	stats := objectiveStats{
		cyclesRemaining: int(constants.MaxDayNightCycles) - int(state.CycleCount),
		localEscaped:    local.CurrentRoomID == winconditions.EscapedRoomID,
	}
	if stats.cyclesRemaining < 0 {
		stats.cyclesRemaining = 0
	}

	for _, player := range state.Players {
		if !player.Alive {
			continue
		}
		switch player.Faction {
		case model.FactionAuthority:
			stats.aliveAuthority++
		case model.FactionPrisoner:
			stats.alivePrisoners++
		}
		if player.Role == model.RoleWarden {
			stats.wardenAlive = true
		}
		if player.Role == model.RoleGangLeader {
			stats.gangLeaderAlive = true
			if player.CurrentRoomID == winconditions.EscapedRoomID {
				stats.gangLeaderEscaped = true
			}
		}
		if player.Role == model.RoleGangLeader || player.Role == model.RoleGangMember {
			stats.aliveGang++
		}
	}

	evaluations := escape.EvaluateAllRoutes(local, state.Map)
	stats.routeCount = len(evaluations)
	for _, evaluation := range evaluations {
		if evaluation.CanAttempt {
			stats.routesReady++
		}
	}

	return stats
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func formatHearts(halfHearts uint8) string {
	whole := halfHearts / 2
	if halfHearts%2 == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.5", whole)
}

func onOff(value bool) string {
	if value {
		return "ON"
	}
	return "OFF"
}

func roomOrUnknown(roomID model.RoomID) string {
	if roomID == "" {
		return "unknown"
	}
	return string(roomID)
}

func playerByID(players []model.PlayerState, playerID model.PlayerID) (model.PlayerState, bool) {
	for _, player := range players {
		if player.ID == playerID {
			return player, true
		}
	}
	return model.PlayerState{}, false
}
