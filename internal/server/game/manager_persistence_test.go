package game

import (
	"sync"
	"testing"
	"time"

	"prison-break/internal/shared/model"
)

func TestManagerJoinResumeAndMatchEndPersistToStore(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    2,
			TickRateHz:    10,
			MatchIDPrefix: "persist-hook",
		},
		time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	recorder := &persistenceRecorderStub{}
	manager.BindPersistence(recorder)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "Alice"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if recorder.UpsertCount() != 1 {
		t.Fatalf("expected one persistence upsert after join, got %d", recorder.UpsertCount())
	}

	if err := manager.SetPlayerConnected(match.MatchID, "p1", false); err != nil {
		t.Fatalf("set disconnected: %v", err)
	}
	if _, err := manager.ResumePlayer(match.MatchID, "p1", "Alice-Rejoin"); err != nil {
		t.Fatalf("resume failed: %v", err)
	}
	if recorder.UpsertCount() != 2 {
		t.Fatalf("expected second persistence upsert after resume, got %d", recorder.UpsertCount())
	}

	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start match failed: %v", err)
	}
	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after match start")
	}

	// Force immediate game-over path and verify persistence recording hook.
	setPlayerHeartsForTest(manager, match.MatchID, "p1", 0)
	ticker.Tick(time.Date(2026, 3, 4, 12, 0, 1, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 1)

	if recorder.RecordCount() != 1 {
		t.Fatalf("expected one persistence record call on match end, got %d", recorder.RecordCount())
	}
	recorded := recorder.LastRecordedState()
	if recorded.GameOver == nil {
		t.Fatalf("expected recorded game state to include game over result")
	}
	if recorded.MatchID != match.MatchID {
		t.Fatalf("expected recorded match id %s, got %s", match.MatchID, recorded.MatchID)
	}
}

type persistenceRecorderStub struct {
	mu sync.Mutex

	upserts []persistenceUpsertCall
	records []persistenceRecordCall
}

type persistenceUpsertCall struct {
	PlayerID    model.PlayerID
	DisplayName string
	ObservedAt  time.Time
}

type persistenceRecordCall struct {
	State   model.GameState
	EndedAt time.Time
}

func (stub *persistenceRecorderStub) UpsertAccount(playerID model.PlayerID, displayName string, observedAt time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	stub.upserts = append(stub.upserts, persistenceUpsertCall{
		PlayerID:    playerID,
		DisplayName: displayName,
		ObservedAt:  observedAt,
	})
	return nil
}

func (stub *persistenceRecorderStub) RecordMatch(state model.GameState, endedAt time.Time) error {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	stub.records = append(stub.records, persistenceRecordCall{
		State:   state,
		EndedAt: endedAt,
	})
	return nil
}

func (stub *persistenceRecorderStub) UpsertCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return len(stub.upserts)
}

func (stub *persistenceRecorderStub) RecordCount() int {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return len(stub.records)
}

func (stub *persistenceRecorderStub) LastRecordedState() model.GameState {
	stub.mu.Lock()
	defer stub.mu.Unlock()

	if len(stub.records) == 0 {
		return model.GameState{}
	}
	return cloneGameState(stub.records[len(stub.records)-1].State)
}
