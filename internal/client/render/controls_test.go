package render

import (
	"strings"
	"testing"
)

func TestPauseMenuLinesListCanonicalControls(t *testing.T) {
	lines := pauseMenuLines()
	if len(lines) < 12 {
		t.Fatalf("expected rich pause menu control list, got %v", lines)
	}

	joined := strings.Join(lines, " | ")
	assertContains(t, joined, "Move: W / A / S / D")
	assertContains(t, joined, "Fire: Left Mouse")
	assertContains(t, joined, "Interact / Use: E")
	assertContains(t, joined, "Use ability: V")
	assertContains(t, joined, "Role + ability info: I")
	assertContains(t, joined, "Reload: R")
	assertContains(t, joined, "Inventory panel: Tab")
	assertContains(t, joined, "Cards panel: C")
	assertContains(t, joined, "Black market panel: B")
	assertContains(t, joined, "Escape panel: X")
	assertContains(t, joined, "Cell stash panel: H")
	assertContains(t, joined, "Modal navigation: Up / Down arrows")
	assertContains(t, joined, "Modal confirm: Enter")
	assertContains(t, joined, "Esc: Resume")
	assertContains(t, joined, "Q: Exit match to menu")
}

func TestControlHintsAvoidDuplicateKeyBindingLanguage(t *testing.T) {
	joinedPause := strings.Join(pauseMenuLines(), " | ")
	assertNotContains(t, joinedPause, "WASD/Arrows")
	assertNotContains(t, joinedPause, "Space/LMB")
	assertNotContains(t, joinedPause, "E/F")
	assertNotContains(t, joinedPause, "V/G")
	assertNotContains(t, joinedPause, "Esc or P")

	hints := actionHintLines(HUDOptions{ShowDesktopActionHints: true})
	if len(hints) != 1 {
		t.Fatalf("expected one desktop hint line, got %v", hints)
	}
	desktop := hints[0]
	assertContains(t, desktop, "Move WASD")
	assertContains(t, desktop, "Fire LMB")
	assertContains(t, desktop, "Interact E")
	assertContains(t, desktop, "Panels Tab/C/B/X/H")
	assertContains(t, desktop, "Modal Arrows+Enter")
	assertNotContains(t, desktop, "WASD/Arrows")
	assertNotContains(t, desktop, "Space/LMB")
	assertNotContains(t, desktop, "E/F")
}
