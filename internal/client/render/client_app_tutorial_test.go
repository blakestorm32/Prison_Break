package render

import (
	"strings"
	"testing"
)

func TestClientAppTutorialLinesExposeCodexContentAndNavigation(t *testing.T) {
	app := NewClientApp(ClientAppConfig{
		ScreenWidth:  1280,
		ScreenHeight: 720,
	})
	if len(app.codexPages) == 0 {
		t.Fatalf("expected tutorial codex pages to be loaded")
	}

	app.flow.MoveMenuSelection(3) // Tutorial / Rules Codex
	_, shouldConnect := app.flow.ActivateMenuSelection()
	if shouldConnect {
		t.Fatalf("expected tutorial menu activation to avoid connection flow")
	}

	lines := app.buildPreMatchLines()
	joined := strings.Join(lines, " | ")
	if !strings.Contains(joined, "Tutorial / Rules Codex") {
		t.Fatalf("expected tutorial heading in pre-match lines, got %q", joined)
	}
	if !strings.Contains(joined, "Controls: Left/Right move page, Esc back to menu") {
		t.Fatalf("expected tutorial navigation hint in pre-match lines, got %q", joined)
	}

	firstPageLine := lines[1]
	app.flow.MoveTutorialPage(1, len(app.codexPages))
	lines = app.buildPreMatchLines()
	if lines[1] == firstPageLine {
		t.Fatalf("expected tutorial page line to change after page navigation")
	}
}
