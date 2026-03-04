package determinism

import (
	"encoding/json"
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestRunSameSeedAndInputsProducesSameHash(t *testing.T) {
	initial := fixtureState()
	inputs := fixtureInputs(t)

	config := SimulationConfig{
		MatchSeed: 77,
		StartTick: 1,
		EndTick:   5,
	}

	finalA := Run(config, initial, inputs, testStep)
	finalB := Run(config, initial, inputs, testStep)

	hashA, err := HashGameState(finalA)
	if err != nil {
		t.Fatalf("hash first final state: %v", err)
	}

	hashB, err := HashGameState(finalB)
	if err != nil {
		t.Fatalf("hash second final state: %v", err)
	}

	if hashA != hashB {
		t.Fatalf("expected identical hashes, got %q and %q", hashA, hashB)
	}
}

func TestRunDifferentArrivalBatchingSameIngressOrderProducesSameState(t *testing.T) {
	initial := fixtureState()

	commandA := moveCommand(t, "p1", 1, 10, 1, 1, 0)
	commandB := moveCommand(t, "p2", 1, 11, 1, -1, 0)
	commandC := moveCommand(t, "p3", 1, 12, 2, 0, 1)
	commandD := fireCommand(t, "p1", 2, 13, 3, 5, 2)
	commandE := moveCommand(t, "p2", 2, 14, 4, 0.5, 0.25)

	batchingOne := []model.InputCommand{
		commandC,
		commandA,
		commandD,
		commandB,
		commandE,
	}
	batchingTwo := []model.InputCommand{
		commandA,
		commandB,
		commandC,
		commandD,
		commandE,
	}

	config := SimulationConfig{
		MatchSeed: 100,
		StartTick: 1,
		EndTick:   5,
	}

	stateOne := Run(config, initial, batchingOne, testStep)
	stateTwo := Run(config, initial, batchingTwo, testStep)

	hashOne, err := HashGameState(stateOne)
	if err != nil {
		t.Fatalf("hash first batched state: %v", err)
	}
	hashTwo, err := HashGameState(stateTwo)
	if err != nil {
		t.Fatalf("hash second batched state: %v", err)
	}

	if hashOne != hashTwo {
		t.Fatalf("expected same state hash for equivalent ingress ordering, got %q and %q", hashOne, hashTwo)
	}
}

func TestRunDuplicateClientSeqIsIdempotent(t *testing.T) {
	initial := fixtureState()
	baseInputs := fixtureInputs(t)

	duplicate := moveCommand(t, "p1", 1, 999, 1, 99, 99)
	withDuplicate := append([]model.InputCommand(nil), baseInputs...)
	withDuplicate = append(withDuplicate, duplicate)

	config := SimulationConfig{
		MatchSeed: 7,
		StartTick: 1,
		EndTick:   5,
	}

	baseState := Run(config, initial, baseInputs, testStep)
	dedupedState := Run(config, initial, withDuplicate, testStep)

	baseHash, err := HashGameState(baseState)
	if err != nil {
		t.Fatalf("hash base state: %v", err)
	}
	dedupedHash, err := HashGameState(dedupedState)
	if err != nil {
		t.Fatalf("hash duplicate state: %v", err)
	}

	if baseHash != dedupedHash {
		t.Fatalf("duplicate command changed outcome: %q vs %q", baseHash, dedupedHash)
	}
}

func TestRNGStreamsAreReproducibleAcrossRuns(t *testing.T) {
	streamsA := NewRNGStreams(12345)
	streamsB := NewRNGStreams(12345)

	rolesA := make([]uint64, 0, 10)
	rolesB := make([]uint64, 0, 10)
	for i := 0; i < 10; i++ {
		rolesA = append(rolesA, streamsA.Stream("roles").NextUint64())
		rolesB = append(rolesB, streamsB.Stream("roles").NextUint64())
	}

	eventsA := make([]uint64, 0, 10)
	eventsB := make([]uint64, 0, 10)
	for i := 0; i < 10; i++ {
		eventsA = append(eventsA, streamsA.Stream("events").NextUint64())
		eventsB = append(eventsB, streamsB.Stream("events").NextUint64())
	}

	if !reflect.DeepEqual(rolesA, rolesB) {
		t.Fatalf("roles stream mismatch between runs")
	}
	if !reflect.DeepEqual(eventsA, eventsB) {
		t.Fatalf("events stream mismatch between runs")
	}
}

func TestDropAckedInputsRemovesAckedOnly(t *testing.T) {
	pending := []model.InputCommand{
		{ClientSeq: 4},
		{ClientSeq: 2},
		{ClientSeq: 6},
		{ClientSeq: 5},
		{ClientSeq: 7},
	}

	filtered := DropAckedInputs(pending, 4)

	got := make([]uint64, 0, len(filtered))
	for _, cmd := range filtered {
		got = append(got, cmd.ClientSeq)
	}

	want := []uint64{6, 5, 7}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered seq list: got=%v want=%v", got, want)
	}
}

func fixtureState() model.GameState {
	return model.GameState{
		MatchID:    "match-01",
		TickID:     0,
		Status:     model.MatchStatusRunning,
		CycleCount: 0,
		Phase: model.PhaseState{
			Current:     model.PhaseDay,
			StartedTick: 1,
			EndsTick:    300,
		},
		Map: model.MapState{
			PowerOn: true,
			Alarm: model.AlarmState{
				Active: false,
			},
		},
		Players: []model.PlayerState{
			{
				ID:         "p2",
				Name:       "P2",
				Connected:  true,
				Alive:      true,
				Role:       model.RoleGangMember,
				Faction:    model.FactionPrisoner,
				Alignment:  model.AlignmentEvil,
				HeartsHalf: 6,
				Bullets:    3,
				Position: model.Vector2{
					X: 5,
					Y: 5,
				},
			},
			{
				ID:         "p1",
				Name:       "P1",
				Connected:  true,
				Alive:      true,
				Role:       model.RoleWarden,
				Faction:    model.FactionAuthority,
				Alignment:  model.AlignmentGood,
				HeartsHalf: 10,
				Bullets:    3,
				Position: model.Vector2{
					X: 2,
					Y: 2,
				},
			},
			{
				ID:         "p3",
				Name:       "P3",
				Connected:  true,
				Alive:      true,
				Role:       model.RoleSnitch,
				Faction:    model.FactionPrisoner,
				Alignment:  model.AlignmentGood,
				HeartsHalf: 6,
				Bullets:    1,
				Position: model.Vector2{
					X: 3,
					Y: 4,
				},
			},
		},
	}
}

func fixtureInputs(t *testing.T) []model.InputCommand {
	t.Helper()

	return []model.InputCommand{
		moveCommand(t, "p1", 1, 1, 1, 1, 0),
		moveCommand(t, "p2", 1, 2, 1, -1, 0),
		moveCommand(t, "p3", 1, 3, 2, 0, 1),
		fireCommand(t, "p1", 2, 4, 3, 6, 6),
		moveCommand(t, "p2", 2, 5, 4, 0.25, 0.5),
	}
}

func testStep(state *model.GameState, tickID uint64, commands []model.InputCommand, rng *RNGStreams) {
	for _, cmd := range commands {
		playerIndex := findPlayer(state.Players, cmd.PlayerID)
		if playerIndex < 0 {
			continue
		}

		switch cmd.Type {
		case model.CmdMoveIntent:
			var payload model.MovementInputPayload
			if err := json.Unmarshal(cmd.Payload, &payload); err != nil {
				continue
			}
			state.Players[playerIndex].Position.X += payload.MoveX
			state.Players[playerIndex].Position.Y += payload.MoveY
			state.Players[playerIndex].Velocity = model.Vector2{
				X: payload.MoveX,
				Y: payload.MoveY,
			}
		case model.CmdFireWeapon:
			if state.Players[playerIndex].Bullets > 0 {
				state.Players[playerIndex].Bullets--
			}
			if len(state.Players) > 0 {
				targetIndex := rng.Stream("events").NextIntn(len(state.Players))
				if state.Players[targetIndex].HeartsHalf > 0 {
					state.Players[targetIndex].HeartsHalf--
				}
			}
		default:
			// Keep non-movement commands deterministic without needing full game logic.
			state.Players[playerIndex].TempHeartsHalf++
		}
	}

	if len(state.Players) == 0 {
		return
	}

	bonusPlayer := rng.Stream("loot").NextIntn(len(state.Players))
	state.Players[bonusPlayer].Bullets += uint8(rng.Stream("loot").NextUint64() % 2)
	state.TickID = tickID
}

func findPlayer(players []model.PlayerState, playerID model.PlayerID) int {
	for i := range players {
		if players[i].ID == playerID {
			return i
		}
	}
	return -1
}

func moveCommand(
	t *testing.T,
	playerID model.PlayerID,
	clientSeq uint64,
	ingressSeq uint64,
	targetTick uint64,
	moveX float32,
	moveY float32,
) model.InputCommand {
	t.Helper()

	payload := mustRawJSON(t, model.MovementInputPayload{
		MoveX: moveX,
		MoveY: moveY,
	})

	return model.InputCommand{
		PlayerID:   playerID,
		ClientSeq:  clientSeq,
		IngressSeq: ingressSeq,
		TargetTick: targetTick,
		Type:       model.CmdMoveIntent,
		Payload:    payload,
	}
}

func fireCommand(
	t *testing.T,
	playerID model.PlayerID,
	clientSeq uint64,
	ingressSeq uint64,
	targetTick uint64,
	targetX float32,
	targetY float32,
) model.InputCommand {
	t.Helper()

	payload := mustRawJSON(t, model.FireWeaponPayload{
		Weapon:  model.ItemPistol,
		TargetX: targetX,
		TargetY: targetY,
	})

	return model.InputCommand{
		PlayerID:   playerID,
		ClientSeq:  clientSeq,
		IngressSeq: ingressSeq,
		TargetTick: targetTick,
		Type:       model.CmdFireWeapon,
		Payload:    payload,
	}
}

func mustRawJSON(t *testing.T, payload any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	return raw
}
