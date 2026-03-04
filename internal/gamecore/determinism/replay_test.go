package determinism

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestSortInputCommandsUsesAllTieBreakers(t *testing.T) {
	cmdA := model.InputCommand{
		PlayerID:   "b",
		ClientSeq:  1,
		IngressSeq: 2,
		Type:       model.CmdMoveIntent,
		Payload:    json.RawMessage(`{"k":1}`),
	}
	cmdB := model.InputCommand{
		PlayerID:   "c",
		ClientSeq:  2,
		IngressSeq: 1,
		Type:       model.CmdMoveIntent,
		Payload:    json.RawMessage(`{"k":1}`),
	}
	cmdC := model.InputCommand{
		PlayerID:   "a",
		ClientSeq:  2,
		IngressSeq: 1,
		Type:       model.CmdMoveIntent,
		Payload:    json.RawMessage(`{"k":1}`),
	}
	cmdD := model.InputCommand{
		PlayerID:   "a",
		ClientSeq:  1,
		IngressSeq: 1,
		Type:       model.CmdMoveIntent,
		Payload:    json.RawMessage(`{"k":2}`),
	}
	cmdE := model.InputCommand{
		PlayerID:   "a",
		ClientSeq:  1,
		IngressSeq: 1,
		Type:       model.CmdMoveIntent,
		Payload:    json.RawMessage(`{"k":1}`),
	}

	commands := []model.InputCommand{cmdA, cmdB, cmdC, cmdD, cmdE}
	sortInputCommands(commands)

	got := make([]string, 0, len(commands))
	for _, cmd := range commands {
		got = append(got, fmt.Sprintf("%s:%d:%s", cmd.PlayerID, cmd.ClientSeq, string(cmd.Payload)))
	}

	want := []string{
		`a:1:{"k":1}`,
		`a:1:{"k":2}`,
		`a:2:{"k":1}`,
		`c:2:{"k":1}`,
		`b:1:{"k":1}`,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sort order: got=%v want=%v", got, want)
	}
}

func TestNormalizeInputLogSortsAndDedupesByPlayerAndClientSeq(t *testing.T) {
	commands := []model.InputCommand{
		{
			PlayerID:   "p1",
			ClientSeq:  1,
			IngressSeq: 9,
			Type:       model.CmdMoveIntent,
			Payload:    json.RawMessage(`{"v":9}`),
		},
		{
			PlayerID:   "p2",
			ClientSeq:  1,
			IngressSeq: 2,
			Type:       model.CmdMoveIntent,
			Payload:    json.RawMessage(`{"v":2}`),
		},
		{
			PlayerID:   "p1",
			ClientSeq:  1,
			IngressSeq: 1,
			Type:       model.CmdMoveIntent,
			Payload:    json.RawMessage(`{"v":1}`),
		},
	}

	normalized := normalizeInputLog(commands)
	if len(normalized) != 2 {
		t.Fatalf("expected 2 commands after dedupe, got %d", len(normalized))
	}

	if normalized[0].PlayerID != "p1" || normalized[0].ClientSeq != 1 || normalized[0].IngressSeq != 1 {
		t.Fatalf("expected earliest p1 command by sort order to survive dedupe, got %+v", normalized[0])
	}
	if normalized[1].PlayerID != "p2" || normalized[1].ClientSeq != 1 {
		t.Fatalf("expected p2 command in normalized set, got %+v", normalized[1])
	}
}

func TestNormalizeInputLogNilInput(t *testing.T) {
	normalized := normalizeInputLog(nil)
	if normalized != nil {
		t.Fatalf("expected nil normalized log for nil input, got %#v", normalized)
	}
}

func TestScheduleByTickClampsAndDropsOutOfRange(t *testing.T) {
	commands := []model.InputCommand{
		{PlayerID: "p1", ClientSeq: 1, TargetTick: 0},
		{PlayerID: "p1", ClientSeq: 2, TargetTick: 1},
		{PlayerID: "p1", ClientSeq: 3, TargetTick: 2},
		{PlayerID: "p1", ClientSeq: 4, TargetTick: 5},
		{PlayerID: "p1", ClientSeq: 5, TargetTick: 99},
	}

	scheduled := scheduleByTick(commands, 3, 5)

	if len(scheduled[3]) != 3 {
		t.Fatalf("expected 3 commands clamped/defaulted to tick 3, got %d", len(scheduled[3]))
	}
	if len(scheduled[5]) != 1 {
		t.Fatalf("expected 1 command scheduled to tick 5, got %d", len(scheduled[5]))
	}
	if _, exists := scheduled[99]; exists {
		t.Fatalf("did not expect commands beyond end tick to be scheduled")
	}
}

func TestRunDefaultsStartTickAndSupportsNilStep(t *testing.T) {
	initial := fixtureState()
	final := Run(
		SimulationConfig{
			MatchSeed: 1,
			StartTick: 0,
			EndTick:   3,
		},
		initial,
		nil,
		nil,
	)

	if final.TickID != 3 {
		t.Fatalf("expected TickID=3 with nil step and default start tick, got %d", final.TickID)
	}
}

func TestRunEndTickBeforeStartReturnsDeepClone(t *testing.T) {
	initial := fixtureState()
	initial.Map.Cells = []model.CellState{
		{
			ID:                1,
			OccupantPlayerIDs: []model.PlayerID{"p1", "p2"},
		},
	}

	final := Run(
		SimulationConfig{
			MatchSeed: 1,
			StartTick: 10,
			EndTick:   5,
		},
		initial,
		nil,
		nil,
	)

	final.Players[0].Name = "changed"
	final.Map.Cells[0].OccupantPlayerIDs[0] = "changed"

	if initial.Players[0].Name == "changed" {
		t.Fatalf("expected player slice to be deep-cloned")
	}
	if initial.Map.Cells[0].OccupantPlayerIDs[0] == "changed" {
		t.Fatalf("expected nested map cell occupants to be deep-cloned")
	}
}

func TestRunProcessesOnlyCommandsWithinTickWindow(t *testing.T) {
	initial := fixtureState()
	commandAtTwo := moveCommand(t, "p1", 1, 1, 2, 1, 0)
	commandAtThree := moveCommand(t, "p1", 2, 2, 3, 1, 0)
	commandAtFour := moveCommand(t, "p1", 3, 3, 4, 1, 0)

	processed := make([]uint64, 0, 4)

	final := Run(
		SimulationConfig{
			MatchSeed: 2,
			StartTick: 2,
			EndTick:   3,
		},
		initial,
		[]model.InputCommand{commandAtTwo, commandAtThree, commandAtFour},
		func(state *model.GameState, tickID uint64, commands []model.InputCommand, _ *RNGStreams) {
			processed = append(processed, tickID)
			for _, cmd := range commands {
				idx := findPlayer(state.Players, cmd.PlayerID)
				if idx >= 0 {
					state.Players[idx].TempHeartsHalf++
				}
			}
		},
	)

	if final.TickID != 3 {
		t.Fatalf("expected simulation to end at tick 3, got %d", final.TickID)
	}

	player := final.Players[findPlayer(final.Players, "p1")]
	if player.TempHeartsHalf != 2 {
		t.Fatalf("expected only 2 in-window commands to apply, got %d", player.TempHeartsHalf)
	}

	wantTicks := []uint64{2, 3}
	if !reflect.DeepEqual(processed, wantTicks) {
		t.Fatalf("unexpected processed ticks: got=%v want=%v", processed, wantTicks)
	}
}

func TestRunIgnoresUnknownPlayersInStep(t *testing.T) {
	initial := fixtureState()

	unknown := moveCommand(t, "missing", 1, 1, 1, 1, 0)
	processedKnownCommands := 0

	final := Run(
		SimulationConfig{
			MatchSeed: 10,
			StartTick: 1,
			EndTick:   1,
		},
		initial,
		[]model.InputCommand{unknown},
		func(state *model.GameState, _ uint64, commands []model.InputCommand, _ *RNGStreams) {
			for _, cmd := range commands {
				if findPlayer(state.Players, cmd.PlayerID) >= 0 {
					processedKnownCommands++
				}
			}
		},
	)

	if processedKnownCommands != 0 {
		t.Fatalf("expected no known-player commands to be processed, got %d", processedKnownCommands)
	}
	if final.TickID != 1 {
		t.Fatalf("expected simulation to advance to tick 1, got %d", final.TickID)
	}
}
