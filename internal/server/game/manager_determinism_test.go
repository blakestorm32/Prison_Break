package game

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestDeterministicReplayProducesIdenticalSnapshotsAndFinalState(t *testing.T) {
	config := Config{
		MinPlayers:    3,
		MaxPlayers:    6,
		TickRateHz:    30,
		DaySeconds:    300,
		NightSeconds:  120,
		MaxCycles:     6,
		MatchIDPrefix: "det",
	}
	start := time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC)

	first := runDeterministicReplayScenario(t, config, start)
	second := runDeterministicReplayScenario(t, config, start)

	if !reflect.DeepEqual(first.snapshots, second.snapshots) {
		firstJSON, _ := json.MarshalIndent(first.snapshots, "", "  ")
		secondJSON, _ := json.MarshalIndent(second.snapshots, "", "  ")
		t.Fatalf("expected identical snapshot streams\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}

	if !reflect.DeepEqual(first.finalState, second.finalState) {
		firstJSON, _ := json.MarshalIndent(first.finalState, "", "  ")
		secondJSON, _ := json.MarshalIndent(second.finalState, "", "  ")
		t.Fatalf("expected identical final state\nfirst=%s\nsecond=%s", firstJSON, secondJSON)
	}
}

type replayResult struct {
	snapshots  []model.Snapshot
	finalState model.GameState
}

func runDeterministicReplayScenario(t *testing.T, config Config, start time.Time) replayResult {
	t.Helper()

	manager, _, factory := newTestManager(config, start)
	defer manager.Close()

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join p1 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p2", "P2"); err != nil {
		t.Fatalf("join p2 failed: %v", err)
	}
	if _, err := manager.JoinMatch(match.MatchID, "p3", "P3"); err != nil {
		t.Fatalf("join p3 failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 1, 1, 0, false)
	mustSubmitMoveIntent(t, manager, match.MatchID, "p2", 1, 0, 1, false)
	ticker.Tick(start.Add(1 * time.Second))
	waitForTick(t, manager, match.MatchID, 1)

	mustSubmitMoveIntent(t, manager, match.MatchID, "p1", 2, 1, 0, true)
	mustSubmitMoveIntent(t, manager, match.MatchID, "p3", 1, -1, 0, false)
	ticker.Tick(start.Add(2 * time.Second))
	waitForTick(t, manager, match.MatchID, 2)

	mustSubmitMoveIntent(t, manager, match.MatchID, "p2", 2, 0, -1, false)
	mustSubmitInteract(t, manager, match.MatchID, "p3", 2, model.InteractPayload{
		TargetRoomID: gamemap.RoomCorridorMain,
	})
	ticker.Tick(start.Add(3 * time.Second))
	waitForTick(t, manager, match.MatchID, 3)

	snapshots, err := manager.SnapshotsSince(match.MatchID, 0)
	if err != nil {
		t.Fatalf("snapshots since failed: %v", err)
	}
	full, err := manager.FullSnapshot(match.MatchID)
	if err != nil {
		t.Fatalf("full snapshot failed: %v", err)
	}
	if full.State == nil {
		t.Fatalf("expected full snapshot state")
	}

	return replayResult{
		snapshots:  snapshots,
		finalState: *full.State,
	}
}
