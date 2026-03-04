package gamemap

import (
	"errors"
	"reflect"
	"testing"

	"prison-break/internal/shared/model"
)

func TestDefaultPrisonLayoutContainsExpectedRoomsAndRestrictedZones(t *testing.T) {
	layout := DefaultPrisonLayout()
	if layout.Width() != 76 || layout.Height() != 44 {
		t.Fatalf("expected default layout dimensions 76x44, got %dx%d", layout.Width(), layout.Height())
	}

	expectedRooms := []model.RoomID{
		RoomCorridorMain,
		RoomWardenHQ,
		RoomCameraRoom,
		RoomPowerRoom,
		RoomAmmoRoom,
		RoomMailRoom,
		RoomCellBlockA,
		RoomCafeteria,
		RoomCourtyard,
		RoomBlackMarket,
		RoomRoofLookout,
	}

	for _, roomID := range expectedRooms {
		if !layout.HasRoom(roomID) {
			t.Fatalf("expected room %s in default layout", roomID)
		}
	}

	if !layout.IsCorridorRoom(RoomCorridorMain) {
		t.Fatalf("expected %s to be marked as corridor room", RoomCorridorMain)
	}
	if !layout.IsRoomRestricted(RoomWardenHQ) {
		t.Fatalf("expected %s to be restricted", RoomWardenHQ)
	}
	if !layout.IsRoomRestricted(RoomCameraRoom) {
		t.Fatalf("expected %s to be restricted", RoomCameraRoom)
	}
	if !layout.IsRoomRestricted(RoomAmmoRoom) {
		t.Fatalf("expected %s to be restricted", RoomAmmoRoom)
	}
	if layout.IsRoomRestricted(RoomCafeteria) {
		t.Fatalf("expected %s to not be restricted", RoomCafeteria)
	}

	if layout.BlackMarketRoomID() != RoomBlackMarket {
		t.Fatalf(
			"expected black market room id %s, got %s",
			RoomBlackMarket,
			layout.BlackMarketRoomID(),
		)
	}

	mapState := layout.ToMapState()
	if len(mapState.Doors) != 22 {
		t.Fatalf("expected 22 doors (10 room + 12 cell), got %d", len(mapState.Doors))
	}
	if len(mapState.Cells) != 12 {
		t.Fatalf("expected 12 cells, got %d", len(mapState.Cells))
	}
	if len(mapState.RestrictedZones) != 4 {
		t.Fatalf("expected 4 restricted zones, got %d", len(mapState.RestrictedZones))
	}
	if mapState.BlackMarketRoomID != RoomBlackMarket {
		t.Fatalf(
			"expected map state black market room %s, got %s",
			RoomBlackMarket,
			mapState.BlackMarketRoomID,
		)
	}
}

func TestCheckRoomAccessReachabilityAndRestrictionFlags(t *testing.T) {
	layout := DefaultPrisonLayout()

	access, err := layout.CheckRoomAccess(RoomCafeteria, RoomWardenHQ)
	if err != nil {
		t.Fatalf("check room access failed: %v", err)
	}

	if !access.Reachable {
		t.Fatalf("expected cafeteria -> warden_hq to be reachable")
	}
	if !access.TargetRestricted {
		t.Fatalf("expected warden_hq target to be marked restricted")
	}
	if len(access.RoomPath) < 3 {
		t.Fatalf("expected at least 3 rooms in path, got %v", access.RoomPath)
	}
	if access.RoomPath[0] != RoomCafeteria {
		t.Fatalf("expected path to begin at cafeteria, got %s", access.RoomPath[0])
	}
	if access.RoomPath[len(access.RoomPath)-1] != RoomWardenHQ {
		t.Fatalf(
			"expected path to end at warden_hq, got %s",
			access.RoomPath[len(access.RoomPath)-1],
		)
	}

	_, missingErr := layout.CheckRoomAccess("missing-room", RoomWardenHQ)
	if !errors.Is(missingErr, ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound for missing room, got %v", missingErr)
	}
}

func TestFindRoomPathIsDeterministicAndConnected(t *testing.T) {
	layout := DefaultPrisonLayout()

	pathA, ok := layout.FindRoomPath(RoomMailRoom, RoomCellBlockA)
	if !ok {
		t.Fatalf("expected room path to exist")
	}
	pathB, ok := layout.FindRoomPath(RoomMailRoom, RoomCellBlockA)
	if !ok {
		t.Fatalf("expected room path to exist on second call")
	}
	if !reflect.DeepEqual(pathA, pathB) {
		t.Fatalf("expected deterministic room path, got %v vs %v", pathA, pathB)
	}

	expected := []model.RoomID{RoomMailRoom, RoomCorridorMain, RoomCellBlockA}
	if !reflect.DeepEqual(pathA, expected) {
		t.Fatalf("unexpected room path: got=%v want=%v", pathA, expected)
	}
	if !layout.AreRoomsConnected(RoomMailRoom, RoomCellBlockA) {
		t.Fatalf("expected rooms to be connected")
	}
}

func TestFindPathAcrossRoomsReturnsContiguousWalkablePoints(t *testing.T) {
	layout := DefaultPrisonLayout()

	start := Point{X: 3, Y: 3}
	end := Point{X: 24, Y: 16}

	path, err := layout.FindPath(start, end)
	if err != nil {
		t.Fatalf("find path failed: %v", err)
	}
	if len(path) < 2 {
		t.Fatalf("expected path length >= 2, got %d", len(path))
	}
	if path[0] != start {
		t.Fatalf("expected first path point %+v, got %+v", start, path[0])
	}
	if path[len(path)-1] != end {
		t.Fatalf("expected final path point %+v, got %+v", end, path[len(path)-1])
	}

	sawCorridorTile := false
	for index := 0; index < len(path); index++ {
		point := path[index]
		if !layout.InBounds(point) {
			t.Fatalf("path contains out-of-bounds point %+v", point)
		}
		if !layout.IsWalkable(point) {
			t.Fatalf("path contains non-walkable point %+v", point)
		}

		roomID, exists := layout.RoomAt(point)
		if exists && roomID == RoomCorridorMain {
			sawCorridorTile = true
		}

		if index == 0 {
			continue
		}
		prev := path[index-1]
		dx := point.X - prev.X
		if dx < 0 {
			dx = -dx
		}
		dy := point.Y - prev.Y
		if dy < 0 {
			dy = -dy
		}
		if dx+dy != 1 {
			t.Fatalf("path contains non-adjacent steps: prev=%+v current=%+v", prev, point)
		}
	}

	if !sawCorridorTile {
		t.Fatalf("expected path to traverse corridor room tiles")
	}
}

func TestFindPathEdgeCases(t *testing.T) {
	layout := DefaultPrisonLayout()

	_, err := layout.FindPath(Point{X: -1, Y: 0}, Point{X: 3, Y: 3})
	if !errors.Is(err, ErrNoTilePathFound) {
		t.Fatalf("expected ErrNoTilePathFound for out-of-bounds start, got %v", err)
	}

	_, err = layout.FindPath(Point{X: 3, Y: 3}, Point{X: 0, Y: 0})
	if !errors.Is(err, ErrNoTilePathFound) {
		t.Fatalf("expected ErrNoTilePathFound for wall target, got %v", err)
	}

	same := Point{X: 11, Y: 15}
	path, err := layout.FindPath(same, same)
	if err != nil {
		t.Fatalf("expected zero-distance walkable path to succeed, got %v", err)
	}
	if len(path) != 1 || path[0] != same {
		t.Fatalf("expected single-point path for same start/end, got %v", path)
	}
}

func TestToMapStateCollectionsAreSortedDeterministically(t *testing.T) {
	layout := DefaultPrisonLayout()
	mapState := layout.ToMapState()

	for i := 1; i < len(mapState.Doors); i++ {
		if mapState.Doors[i-1].ID > mapState.Doors[i].ID {
			t.Fatalf("doors not sorted at index %d", i)
		}
	}
	for i := 1; i < len(mapState.Cells); i++ {
		if mapState.Cells[i-1].ID > mapState.Cells[i].ID {
			t.Fatalf("cells not sorted at index %d", i)
		}
	}
	for i := 1; i < len(mapState.RestrictedZones); i++ {
		if mapState.RestrictedZones[i-1].ID > mapState.RestrictedZones[i].ID {
			t.Fatalf("restricted zones not sorted at index %d", i)
		}
	}
}
