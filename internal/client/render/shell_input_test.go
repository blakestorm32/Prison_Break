package render

import (
	"testing"

	"prison-break/internal/client/input"
	"prison-break/internal/client/netclient"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestShellUpdateGeneratesAndQueuesInputCommands(t *testing.T) {
	store := netclient.NewSnapshotStore()
	state := model.GameState{
		MatchID: "m-1",
		TickID:  9,
		Status:  model.MatchStatusRunning,
		Phase: model.PhaseState{
			Current: model.PhaseDay,
		},
		Map: gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{
				ID:       "p1",
				Position: model.Vector2{X: 10, Y: 10},
				Alive:    true,
			},
		},
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: state.TickID,
		State:  &state,
	}) {
		t.Fatalf("expected full snapshot apply")
	}

	snapshotProvider := func() input.InputSnapshot {
		return input.InputSnapshot{
			MoveRight:   true,
			FirePressed: true,
			HasAim:      true,
			AimWorldX:   12,
			AimWorldY:   10,
		}
	}

	shell := NewShell(ShellConfig{
		ScreenWidth:           1280,
		ScreenHeight:          720,
		LocalPlayerID:         "p1",
		Store:                 store,
		Layout:                gamemap.DefaultPrisonLayout(),
		InputController:       input.NewController(input.ControllerConfig{PlayerID: "p1", ScreenWidth: 1280, ScreenHeight: 720}),
		InputSnapshotProvider: snapshotProvider,
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}

	commands := shell.DrainOutgoingCommands()
	if len(commands) != 3 {
		t.Fatalf("expected move+aim+fire commands queued, got %+v", commands)
	}
	if commands[0].Type != model.CmdMoveIntent || commands[1].Type != model.CmdAimIntent || commands[2].Type != model.CmdFireWeapon {
		t.Fatalf("unexpected command types/order: %+v", commands)
	}
	for _, command := range commands {
		if command.TargetTick != 10 {
			t.Fatalf("expected target tick to be next authoritative tick (10), got %d", command.TargetTick)
		}
	}

	if again := shell.DrainOutgoingCommands(); len(again) != 0 {
		t.Fatalf("expected drain to clear queue, got %+v", again)
	}
}

func TestShellUpdateRespectsEdgeTriggeredFireAcrossFrames(t *testing.T) {
	store := netclient.NewSnapshotStore()
	state := model.GameState{
		MatchID: "m-2",
		TickID:  40,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{
				ID:       "p1",
				Position: model.Vector2{X: 8, Y: 8},
				Alive:    true,
			},
		},
	}
	_ = store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: state.TickID,
		State:  &state,
	})

	firePressed := true
	shell := NewShell(ShellConfig{
		ScreenWidth:   800,
		ScreenHeight:  600,
		LocalPlayerID: "p1",
		Store:         store,
		Layout:        gamemap.DefaultPrisonLayout(),
		InputController: input.NewController(input.ControllerConfig{
			PlayerID: "p1",
		}),
		InputSnapshotProvider: func() input.InputSnapshot {
			return input.InputSnapshot{
				FirePressed: firePressed,
				HasAim:      true,
				AimWorldX:   9,
				AimWorldY:   8,
			}
		},
	})

	if err := shell.Update(); err != nil {
		t.Fatalf("first update failed: %v", err)
	}
	first := shell.DrainOutgoingCommands()
	if len(first) != 2 {
		t.Fatalf("expected first frame aim+fire commands, got %+v", first)
	}
	if first[1].Type != model.CmdFireWeapon {
		t.Fatalf("expected second first-frame command to be fire, got %s", first[1].Type)
	}

	if err := shell.Update(); err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	second := shell.DrainOutgoingCommands()
	if len(second) != 0 {
		t.Fatalf("expected held fire button and unchanged target tick to suppress repeated commands, got %+v", second)
	}

	firePressed = false
	_ = shell.Update()
	_ = shell.DrainOutgoingCommands()

	firePressed = true
	if err := shell.Update(); err != nil {
		t.Fatalf("fourth update failed: %v", err)
	}
	fourth := shell.DrainOutgoingCommands()
	if len(fourth) != 1 || fourth[0].Type != model.CmdFireWeapon {
		t.Fatalf("expected fire to retrigger after release, got %+v", fourth)
	}
}

func TestShellShowsMobileActionSurfacesOnNarrowScreen(t *testing.T) {
	store := netclient.NewSnapshotStore()
	state := model.GameState{
		MatchID: "m-mobile-hud",
		TickID:  1,
		Status:  model.MatchStatusRunning,
		Map:     gamemap.DefaultPrisonLayout().ToMapState(),
		Players: []model.PlayerState{
			{
				ID:       "p1",
				Position: model.Vector2{X: 6, Y: 6},
				Alive:    true,
			},
		},
	}
	if !store.ApplySnapshot(model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 1,
		State:  &state,
	}) {
		t.Fatalf("expected baseline state apply")
	}

	shell := NewShell(ShellConfig{
		ScreenWidth:   720,
		ScreenHeight:  1280,
		LocalPlayerID: "p1",
		Store:         store,
		Layout:        gamemap.DefaultPrisonLayout(),
		InputController: input.NewController(input.ControllerConfig{
			PlayerID:     "p1",
			ScreenWidth:  720,
			ScreenHeight: 1280,
		}),
	})

	if !shell.shouldShowMobileActionSurfaces() {
		t.Fatalf("expected narrow-screen shell to show mobile action surfaces")
	}
}
