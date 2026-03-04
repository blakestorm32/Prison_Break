package main

import (
	"os"
	"testing"
)

func TestParseRegionLatencyEnvParsesAndNormalizesEntries(t *testing.T) {
	got := parseRegionLatencyEnv("US-East=45, us-west=90,bad,noeq=,eu=0")
	if len(got) != 2 {
		t.Fatalf("expected 2 valid region latency entries, got %d", len(got))
	}
	if got["us-east"] != 45 {
		t.Fatalf("expected us-east latency 45, got %d", got["us-east"])
	}
	if got["us-west"] != 90 {
		t.Fatalf("expected us-west latency 90, got %d", got["us-west"])
	}
}

func TestLoadRunConfigFromEnvIncludesRegionPreferences(t *testing.T) {
	t.Setenv("PRISON_SERVER_WS_URL", "ws://127.0.0.1:8080/ws")
	t.Setenv("PRISON_PLAYER_NAME", "Tester")
	t.Setenv("PRISON_PLAYER_ID", "player-a")
	t.Setenv("PRISON_PREFERRED_MATCH_ID", "")
	t.Setenv("PRISON_PREFERRED_REGION", "us-east")
	t.Setenv("PRISON_REGION_LATENCY_MS", "us-east=40,us-west=85")
	t.Setenv("PRISON_SPECTATOR_FOLLOW_PLAYER", "p2")
	t.Setenv("PRISON_SPECTATOR_FOLLOW_SLOT", "3")
	t.Setenv("PRISON_SESSION_TOKEN", "")
	t.Setenv("PRISON_SPECTATOR", "false")

	cfg := loadRunConfigFromEnv()
	if cfg.session.PreferredRegion != "us-east" {
		t.Fatalf("expected preferred region us-east, got %q", cfg.session.PreferredRegion)
	}
	if len(cfg.session.RegionLatencyMS) != 2 {
		t.Fatalf("expected two region latency entries, got %d", len(cfg.session.RegionLatencyMS))
	}
	if cfg.session.RegionLatencyMS["us-east"] != 40 {
		t.Fatalf("expected us-east latency 40, got %d", cfg.session.RegionLatencyMS["us-east"])
	}
	if cfg.session.RegionLatencyMS["us-west"] != 85 {
		t.Fatalf("expected us-west latency 85, got %d", cfg.session.RegionLatencyMS["us-west"])
	}
	if cfg.session.SpectatorFollowPlayerID != "p2" {
		t.Fatalf("expected spectator follow player id p2, got %s", cfg.session.SpectatorFollowPlayerID)
	}
	if cfg.session.SpectatorFollowSlot != 3 {
		t.Fatalf("expected spectator follow slot 3, got %d", cfg.session.SpectatorFollowSlot)
	}

	// Ensure test does not leak env for other packages that might call os.Getenv directly.
	_ = os.Unsetenv("PRISON_PREFERRED_REGION")
	_ = os.Unsetenv("PRISON_REGION_LATENCY_MS")
}

func TestParseUint8EnvParsesValidAndRejectsInvalid(t *testing.T) {
	t.Setenv("TEST_UINT8_ENV", "12")
	if got := parseUint8Env("TEST_UINT8_ENV"); got != 12 {
		t.Fatalf("expected parseUint8Env valid parse 12, got %d", got)
	}

	t.Setenv("TEST_UINT8_ENV", "bad")
	if got := parseUint8Env("TEST_UINT8_ENV"); got != 0 {
		t.Fatalf("expected parseUint8Env invalid parse fallback to 0, got %d", got)
	}
}
