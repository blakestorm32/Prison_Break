package onboarding

import "strings"

type CodexPage struct {
	Title string
	Lines []string
}

func Pages() []CodexPage {
	return []CodexPage{
		{
			Title: "Controls Basics",
			Lines: []string{
				"Desktop: Move with WASD, sprint with Shift.",
				"Combat: Aim with mouse, fire with Left Mouse, reload with R.",
				"Interact: Use E for doors, pickups, and route interactions.",
				"Ability: Press V to use ability, I for role + ability info.",
				"Panels: Tab inventory, C cards, B market, X escape routes, H stash.",
				"Spectator: Use Q/E to switch follow camera targets.",
			},
		},
		{
			Title: "Roles and Hidden Info",
			Lines: []string{
				"Authority team: Warden + Deputies maintain prison control.",
				"Prisoner team: Gang leader + members coordinate escape routes.",
				"Snitch/neutral variants change private objectives per match.",
				"Do not trust role labels from other players without verification.",
				"Spectators only see role-safe data; hidden roles stay concealed.",
			},
		},
		{
			Title: "Cards and Economy",
			Lines: []string{
				"Money cards fuel black-market purchases during night phase.",
				"Cards are capped and consumed on use; timing matters.",
				"Typical tactical cards: speed, armor plate, lock snap, item steal.",
				"Reserve high-value cards for escape windows, not early skirmishes.",
				"Track item/card spend to avoid economy collapse before endgame.",
			},
		},
		{
			Title: "Abilities and Teamplay",
			Lines: []string{
				"Authority utilities: search, camera control, detain, tracker intel.",
				"Prisoner utilities: pick pocket, hacker, disguise, locksmith, chameleon.",
				"Most abilities have cooldown/cycle constraints; avoid overlap waste.",
				"Use utility to create positional advantage before direct combat.",
				"Communicate short intent: who acts, target, and fallback route.",
			},
		},
		{
			Title: "Win Conditions and Pacing",
			Lines: []string{
				"Day phase is 5 minutes; night phase is 2 minutes.",
				"Matches run for up to 6 cycles, so tempo is intentionally high.",
				"Authority wins by preventing successful escapes through cycle limit.",
				"Prisoners win by completing a valid escape route.",
				"Balance target: close games with counterplay on both sides.",
			},
		},
	}
}

func PageAt(index int) (CodexPage, bool) {
	pages := Pages()
	if index < 0 || index >= len(pages) {
		return CodexPage{}, false
	}
	page := pages[index]
	page.Title = strings.TrimSpace(page.Title)
	return page, true
}
