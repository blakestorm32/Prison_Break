package onboarding

import (
	"strings"
	"testing"
)

func TestCodexPagesCoversCoreGameplayTopics(t *testing.T) {
	pages := Pages()
	if len(pages) < 5 {
		t.Fatalf("expected at least 5 codex pages, got %d", len(pages))
	}

	var (
		sawControls  bool
		sawRoles     bool
		sawCards     bool
		sawAbilities bool
		sawWinConds  bool
	)
	for _, page := range pages {
		title := strings.ToLower(strings.TrimSpace(page.Title))
		if title == "" {
			t.Fatalf("expected codex page title to be non-empty")
		}
		if len(page.Lines) < 3 {
			t.Fatalf("expected codex page %q to include several guidance lines", page.Title)
		}
		for _, line := range page.Lines {
			if strings.TrimSpace(line) == "" {
				t.Fatalf("expected codex line to be non-empty on page %q", page.Title)
			}
		}

		switch {
		case strings.Contains(title, "control"):
			sawControls = true
		case strings.Contains(title, "role"):
			sawRoles = true
		case strings.Contains(title, "card"):
			sawCards = true
		case strings.Contains(title, "abilit"):
			sawAbilities = true
		case strings.Contains(title, "win"):
			sawWinConds = true
		}
	}

	if !sawControls || !sawRoles || !sawCards || !sawAbilities || !sawWinConds {
		t.Fatalf(
			"expected codex topics controls/roles/cards/abilities/win-conditions, got controls=%v roles=%v cards=%v abilities=%v wins=%v",
			sawControls,
			sawRoles,
			sawCards,
			sawAbilities,
			sawWinConds,
		)
	}
}

func TestPageAtBounds(t *testing.T) {
	if _, ok := PageAt(-1); ok {
		t.Fatalf("expected negative page index to be rejected")
	}
	if _, ok := PageAt(999); ok {
		t.Fatalf("expected out-of-range page index to be rejected")
	}

	page, ok := PageAt(0)
	if !ok {
		t.Fatalf("expected first page to resolve")
	}
	if strings.TrimSpace(page.Title) == "" {
		t.Fatalf("expected first page title to be non-empty")
	}
}
