package render

import (
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestResolveLocalVisionScopeUsesCurrentRoomWhenLocalPlayerPresent(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{
				ID:            "p1",
				CurrentRoomID: gamemap.RoomCellBlockA,
			},
		},
	}

	local, hasLocal, roomID, limited := resolveLocalVisionScope(state, "p1")
	if !hasLocal {
		t.Fatalf("expected local player to be resolved")
	}
	if local.ID != "p1" {
		t.Fatalf("expected local player id p1, got %s", local.ID)
	}
	if roomID != gamemap.RoomCellBlockA {
		t.Fatalf("expected local room %s, got %s", gamemap.RoomCellBlockA, roomID)
	}
	if !limited {
		t.Fatalf("expected render scope to be limited to local room")
	}
}

func TestResolveLocalVisionScopeDisablesRoomLimitingWhenLocalRoomUnknown(t *testing.T) {
	state := model.GameState{
		Players: []model.PlayerState{
			{
				ID: "p1",
			},
		},
	}

	_, hasLocal, roomID, limited := resolveLocalVisionScope(state, "p1")
	if !hasLocal {
		t.Fatalf("expected local player to be resolved")
	}
	if roomID != "" {
		t.Fatalf("expected unknown local room id, got %s", roomID)
	}
	if limited {
		t.Fatalf("expected no room-limited rendering when local room is unknown")
	}
}

func TestProjectDoorOpenForViewerDeniesRestrictedTargetRoom(t *testing.T) {
	door := model.DoorState{
		ID:    1,
		RoomA: gamemap.RoomWardenHQ,
		RoomB: gamemap.RoomCorridorMain,
		Open:  true,
	}
	viewer := model.PlayerState{
		ID:            "prisoner",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		CurrentRoomID: gamemap.RoomCorridorMain,
	}
	mapState := gamemap.DefaultPrisonLayout().ToMapState()

	if projectDoorOpenForViewer(true, door, viewer, mapState) {
		t.Fatalf("expected warden-hq door to project as closed for non-warden viewer")
	}
}

func TestProjectDoorOpenForViewerAllowsExitWhenTargetRoomAccessible(t *testing.T) {
	door := model.DoorState{
		ID:    2,
		RoomA: gamemap.RoomCameraRoom,
		RoomB: gamemap.RoomCorridorMain,
		Open:  true,
	}
	viewer := model.PlayerState{
		ID:            "deputy",
		Role:          model.RoleDeputy,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCameraRoom,
	}
	mapState := gamemap.DefaultPrisonLayout().ToMapState()
	mapState.PowerOn = false

	if !projectDoorOpenForViewer(true, door, viewer, mapState) {
		t.Fatalf("expected corridor exit to remain visible/open while power off")
	}
}

func TestProjectDoorOpenForViewerDeniesCellDoorForNonOwnerPrisoner(t *testing.T) {
	door := model.DoorState{
		ID:    201,
		RoomA: gamemap.RoomCellBlockA,
		RoomB: model.RoomID("cell_a_001"),
		Open:  true,
	}
	viewer := model.PlayerState{
		ID:            "other-prisoner",
		Role:          model.RoleGangMember,
		Faction:       model.FactionPrisoner,
		CurrentRoomID: gamemap.RoomCellBlockA,
	}
	mapState := gamemap.DefaultPrisonLayout().ToMapState()
	for idx := range mapState.Cells {
		if mapState.Cells[idx].ID == 1 {
			mapState.Cells[idx].OwnerPlayerID = "owner"
		}
	}

	if projectDoorOpenForViewer(true, door, viewer, mapState) {
		t.Fatalf("expected non-owner prisoner to see cell door as closed")
	}
}

func TestHasNearbyInteractableSkipsInaccessibleDoors(t *testing.T) {
	shell := &Shell{}
	local := model.PlayerState{
		ID:            "deputy",
		Role:          model.RoleDeputy,
		Faction:       model.FactionAuthority,
		CurrentRoomID: gamemap.RoomCorridorMain,
	}
	state := model.GameState{
		Map: model.MapState{
			PowerOn: true,
			Doors: []model.DoorState{
				{
					ID:    1,
					RoomA: gamemap.RoomWardenHQ,
					RoomB: gamemap.RoomCorridorMain,
					Open:  true,
				},
			},
		},
	}

	if shell.hasNearbyInteractable(local, state) {
		t.Fatalf("expected no nearby interactable when adjacent door is access-denied")
	}
}

func TestDoorLeadLabelForViewerUsesAdjacentTargetRoom(t *testing.T) {
	door := model.DoorState{
		ID:    8,
		RoomA: gamemap.RoomCorridorMain,
		RoomB: gamemap.RoomCourtyard,
		Open:  true,
	}

	label := doorLeadLabelForViewer(door, true, gamemap.RoomCorridorMain)
	if label != "Courtyard" {
		t.Fatalf("expected local-view door destination label Courtyard, got %q", label)
	}
}

func TestDoorLeadLabelForViewerShowsBothRoomsWhenNoLocalContext(t *testing.T) {
	door := model.DoorState{
		ID:    8,
		RoomA: gamemap.RoomCorridorMain,
		RoomB: gamemap.RoomCourtyard,
		Open:  true,
	}

	label := doorLeadLabelForViewer(door, false, "")
	if label != "Main Corridor/Courtyard" {
		t.Fatalf("expected fallback door label with both rooms, got %q", label)
	}
}
