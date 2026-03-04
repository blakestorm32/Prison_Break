package render

import (
	"strings"
	"testing"
)

func TestClampManualUITestClientCount(t *testing.T) {
	cases := []struct {
		name     string
		input    int
		expected int
	}{
		{name: "default_on_zero", input: 0, expected: 5},
		{name: "default_on_negative", input: -3, expected: 5},
		{name: "keeps_valid_value", input: 6, expected: 6},
		{name: "clamps_high_value", input: 42, expected: 9},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := clampManualUITestClientCount(tc.input); got != tc.expected {
				t.Fatalf("expected clamp(%d)=%d, got %d", tc.input, tc.expected, got)
			}
		})
	}
}

func TestBuildPreMatchLinesManualModeConnectingHint(t *testing.T) {
	app := NewClientApp(ClientAppConfig{
		ScreenWidth:         1280,
		ScreenHeight:        720,
		ManualUITestMode:    true,
		ManualUITestClients: 5,
	})

	_, shouldConnect := app.flow.ActivateMenuSelection()
	if !shouldConnect {
		t.Fatalf("expected quick-play selection to enter connecting stage")
	}

	lines := app.buildPreMatchLines()
	joined := strings.Join(lines, " | ")
	if !strings.Contains(joined, "Manual UI Test Mode") {
		t.Fatalf("expected manual mode connecting heading, got %q", joined)
	}
	if !strings.Contains(joined, "Spawning 5 local clients into one lobby") {
		t.Fatalf("expected manual mode spawn hint, got %q", joined)
	}
}
