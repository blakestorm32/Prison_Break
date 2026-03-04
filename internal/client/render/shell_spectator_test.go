package render

import (
	"testing"

	"prison-break/internal/client/input"
	"prison-break/internal/client/netclient"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestShellSpectatorFollowInitializesFromRequestedPlayerID(t *testing.T) {
	shell := newShellForSpectatorFollowTest(t, spectatorFollowFixture{
		initialFollowPlayerID: "p3",
		initialFollowSlot:     1,
		players: []model.PlayerState{
			{ID: "p1", Alive: true, Position: model.Vector2{X: 2, Y: 2}},
			{ID: "p2", Alive: true, Position: model.Vector2{X: 4, Y: 4}},
			{ID: "p3", Alive: true, Position: model.Vector2{X: 6, Y: 6}},
		},
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if shell.spectatorFollowPlayerID != "p3" {
		t.Fatalf("expected spectator follow player p3, got %s", shell.spectatorFollowPlayerID)
	}
	if shell.spectatorFollowSlot != 3 || shell.spectatorSlotCount != 3 {
		t.Fatalf("expected spectator follow slot/count 3/3, got %d/%d", shell.spectatorFollowSlot, shell.spectatorSlotCount)
	}

	focus, ok := shell.resolveCameraFocusPlayer(mustCurrentStateForSpectatorShellTest(t, shell))
	if !ok || focus.ID != "p3" {
		t.Fatalf("expected camera focus target p3, got %+v (ok=%v)", focus, ok)
	}
}

func TestShellSpectatorFollowInitializesFromSlotAndCyclesWrap(t *testing.T) {
	frame := 0
	shell := newShellForSpectatorFollowTest(t, spectatorFollowFixture{
		initialFollowSlot: 2,
		players: []model.PlayerState{
			{ID: "p1", Alive: true, Position: model.Vector2{X: 2, Y: 2}},
			{ID: "p2", Alive: true, Position: model.Vector2{X: 4, Y: 4}},
			{ID: "p3", Alive: true, Position: model.Vector2{X: 6, Y: 6}},
		},
		inputSnapshotProvider: func() input.InputSnapshot {
			frame++
			switch frame {
			case 1:
				return input.InputSnapshot{}
			case 2:
				return input.InputSnapshot{SpectatorNextPressed: true}
			case 3:
				return input.InputSnapshot{}
			case 4:
				return input.InputSnapshot{SpectatorNextPressed: true}
			case 5:
				return input.InputSnapshot{}
			case 6:
				return input.InputSnapshot{SpectatorPrevPressed: true}
			default:
				return input.InputSnapshot{}
			}
		},
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("update frame 1 failed: %v", err)
	}
	if shell.spectatorFollowPlayerID != "p2" {
		t.Fatalf("expected initial slot-based follow player p2, got %s", shell.spectatorFollowPlayerID)
	}

	if err := shell.Update(); err != nil {
		t.Fatalf("update frame 2 failed: %v", err)
	}
	if shell.spectatorFollowPlayerID != "p3" {
		t.Fatalf("expected next follow player p3 after next edge, got %s", shell.spectatorFollowPlayerID)
	}

	_ = shell.Update() // release
	if err := shell.Update(); err != nil {
		t.Fatalf("update frame 4 failed: %v", err)
	}
	if shell.spectatorFollowPlayerID != "p1" {
		t.Fatalf("expected next edge wrap to p1, got %s", shell.spectatorFollowPlayerID)
	}

	_ = shell.Update() // release
	if err := shell.Update(); err != nil {
		t.Fatalf("update frame 6 failed: %v", err)
	}
	if shell.spectatorFollowPlayerID != "p3" {
		t.Fatalf("expected previous edge wrap to p3, got %s", shell.spectatorFollowPlayerID)
	}
}

type spectatorFollowFixture struct {
	initialFollowPlayerID model.PlayerID
	initialFollowSlot     uint8
	players               []model.PlayerState
	inputSnapshotProvider func() input.InputSnapshot
}

func newShellForSpectatorFollowTest(t *testing.T, fixture spectatorFollowFixture) *Shell {
	t.Helper()

	store := netclient.NewSnapshotStore()
	state := model.GameState{
		MatchID: "spectator-follow",
		TickID:  9,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: append([]model.PlayerState(nil), fixture.players...),
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: state.TickID,
		State:  &state,
	}) {
		t.Fatalf("expected baseline snapshot apply")
	}

	provider := fixture.inputSnapshotProvider
	if provider == nil {
		provider = func() input.InputSnapshot { return input.InputSnapshot{} }
	}

	return NewShell(ShellConfig{
		ScreenWidth:             1280,
		ScreenHeight:            720,
		LocalPlayerID:           "",
		SpectatorFollowPlayerID: fixture.initialFollowPlayerID,
		SpectatorFollowSlot:     fixture.initialFollowSlot,
		Store:                   store,
		Layout:                  gamemap.DefaultPrisonLayout(),
		InputSnapshotProvider:   provider,
	})
}

func mustCurrentStateForSpectatorShellTest(t *testing.T, shell *Shell) model.GameState {
	t.Helper()
	state, ok := shell.store.CurrentState()
	if !ok {
		t.Fatalf("expected shell current state")
	}
	return state
}
