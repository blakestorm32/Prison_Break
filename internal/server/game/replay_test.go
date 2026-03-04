package game

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"prison-break/internal/shared/model"
)

func TestReplayLogCapturesAcceptedInputsWithTickAndIngressMetadata(t *testing.T) {
	manager, _, factory := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "replay",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("submit reload failed: %v", err)
	}
	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:   "p1",
		ClientSeq:  2,
		TargetTick: 2,
		Type:       model.CmdMoveIntent,
		Payload:    mustRawJSON(t, model.MovementInputPayload{MoveX: 1, MoveY: 0}),
	}); err != nil {
		t.Fatalf("submit move failed: %v", err)
	}

	ticker := factory.Last()
	if ticker == nil {
		t.Fatalf("expected ticker after start")
	}
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 1, 0, time.UTC))
	ticker.Tick(time.Date(2026, 2, 22, 12, 0, 2, 0, time.UTC))
	waitForTick(t, manager, match.MatchID, 2)

	replay, err := manager.ReplayLog(match.MatchID)
	if err != nil {
		t.Fatalf("replay log failed: %v", err)
	}

	if replay.MatchID != match.MatchID {
		t.Fatalf("unexpected replay match id: got=%s want=%s", replay.MatchID, match.MatchID)
	}
	if replay.TickRateHz != 30 {
		t.Fatalf("unexpected replay tick rate: got=%d want=30", replay.TickRateHz)
	}
	if len(replay.Entries) != 2 {
		t.Fatalf("expected 2 replay entries, got %d", len(replay.Entries))
	}

	first := replay.Entries[0]
	if first.AcceptedTick != 1 || first.IngressSeq != 1 {
		t.Fatalf("unexpected first replay metadata: %+v", first)
	}
	if first.Command.PlayerID != "p1" || first.Command.ClientSeq != 1 || first.Command.Type != model.CmdReload {
		t.Fatalf("unexpected first replay command: %+v", first.Command)
	}
	if first.AcceptedAt.IsZero() {
		t.Fatalf("expected first accepted_at to be populated")
	}

	second := replay.Entries[1]
	if second.AcceptedTick != 2 || second.IngressSeq != 2 {
		t.Fatalf("unexpected second replay metadata: %+v", second)
	}
	if second.Command.PlayerID != "p1" || second.Command.ClientSeq != 2 || second.Command.Type != model.CmdMoveIntent {
		t.Fatalf("unexpected second replay command: %+v", second.Command)
	}
	var payload model.MovementInputPayload
	if err := json.Unmarshal(second.Command.Payload, &payload); err != nil {
		t.Fatalf("decode second replay payload: %v", err)
	}
	if payload.MoveX != 1 || payload.MoveY != 0 {
		t.Fatalf("unexpected second replay movement payload: %+v", payload)
	}
}

func TestReplayLogDuplicateInputIsRejectedAndNotRecorded(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "replay",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); err != nil {
		t.Fatalf("submit reload failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdReload,
	}); !errors.Is(err, ErrDuplicateInput) {
		t.Fatalf("expected duplicate input error, got %v", err)
	}

	replay, err := manager.ReplayLog(match.MatchID)
	if err != nil {
		t.Fatalf("replay log failed: %v", err)
	}
	if len(replay.Entries) != 1 {
		t.Fatalf("expected one replay entry after duplicate rejection, got %d", len(replay.Entries))
	}
}

func TestReplayLogReturnsDeepCopyOfInputPayloads(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "replay",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	match := manager.CreateMatch()
	if _, err := manager.JoinMatch(match.MatchID, "p1", "P1"); err != nil {
		t.Fatalf("join failed: %v", err)
	}
	if _, err := manager.StartMatch(match.MatchID); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if _, err := manager.SubmitInput(match.MatchID, model.InputCommand{
		PlayerID:  "p1",
		ClientSeq: 1,
		Type:      model.CmdMoveIntent,
		Payload:   mustRawJSON(t, model.MovementInputPayload{MoveX: 0.5, MoveY: -0.25}),
	}); err != nil {
		t.Fatalf("submit move failed: %v", err)
	}

	firstReplay, err := manager.ReplayLog(match.MatchID)
	if err != nil {
		t.Fatalf("first replay log failed: %v", err)
	}
	if len(firstReplay.Entries) != 1 {
		t.Fatalf("expected one replay entry, got %d", len(firstReplay.Entries))
	}

	firstReplay.Entries[0].Command.Type = model.CmdReload
	if len(firstReplay.Entries[0].Command.Payload) == 0 {
		t.Fatalf("expected payload in replay command")
	}
	firstReplay.Entries[0].Command.Payload[0] = byte('x')

	secondReplay, err := manager.ReplayLog(match.MatchID)
	if err != nil {
		t.Fatalf("second replay log failed: %v", err)
	}
	if secondReplay.Entries[0].Command.Type != model.CmdMoveIntent {
		t.Fatalf("expected replay command type immutability, got %s", secondReplay.Entries[0].Command.Type)
	}

	var payload model.MovementInputPayload
	if err := json.Unmarshal(secondReplay.Entries[0].Command.Payload, &payload); err != nil {
		t.Fatalf("decode replay payload after mutation attempt: %v", err)
	}
	if payload.MoveX != 0.5 || payload.MoveY != -0.25 {
		t.Fatalf("unexpected replay payload after mutation attempt: %+v", payload)
	}
}

func TestReplayLogMatchNotFound(t *testing.T) {
	manager, _, _ := newTestManager(
		Config{
			MinPlayers:    1,
			MaxPlayers:    4,
			TickRateHz:    30,
			MatchIDPrefix: "replay",
		},
		time.Date(2026, 2, 22, 12, 0, 0, 0, time.UTC),
	)
	t.Cleanup(manager.Close)

	if _, err := manager.ReplayLog("missing"); !errors.Is(err, ErrMatchNotFound) {
		t.Fatalf("expected ErrMatchNotFound, got %v", err)
	}
}
