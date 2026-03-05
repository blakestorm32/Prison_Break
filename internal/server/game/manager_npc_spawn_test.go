package game

import (
	"fmt"
	"reflect"
	"testing"

	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

func TestNPCPrisonerSpawnRoomCandidatesUseCorridorOrPrisonerAccessibleRooms(t *testing.T) {
	mapState := defaultMatchLayout.ToMapState()
	candidates := npcPrisonerSpawnRoomCandidates(mapState)
	if len(candidates) == 0 {
		t.Fatalf("expected at least one npc prisoner spawn room candidate")
	}

	probe := model.PlayerState{
		ID:      "probe",
		Role:    model.RoleGangMember,
		Faction: model.FactionPrisoner,
	}
	for _, roomID := range candidates {
		room, exists := defaultMatchLayout.Room(roomID)
		if !exists {
			t.Fatalf("expected candidate room to exist in layout, room=%s", roomID)
		}
		if room.IsCorridor {
			continue
		}
		if !gamemap.CanEnterRoom(probe, roomID, mapState) {
			t.Fatalf("expected non-corridor candidate room to be prisoner-accessible, room=%s", roomID)
		}
	}
}

func TestDeterministicNPCPrisonerSpawnRoomsStableUniqueAndBounded(t *testing.T) {
	mapState := defaultMatchLayout.ToMapState()

	first := deterministicNPCPrisonerSpawnRooms("spawn-stability", mapState, npcPrisonerSpawnCount)
	second := deterministicNPCPrisonerSpawnRooms("spawn-stability", mapState, npcPrisonerSpawnCount)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("expected deterministic npc spawn rooms to be stable for same inputs, first=%v second=%v", first, second)
	}
	if len(first) == 0 {
		t.Fatalf("expected deterministic spawn to return at least one room")
	}

	seen := make(map[model.RoomID]struct{}, len(first))
	for _, roomID := range first {
		if _, exists := seen[roomID]; exists {
			t.Fatalf("expected deterministic spawn rooms to be unique, got duplicate room=%s in %v", roomID, first)
		}
		seen[roomID] = struct{}{}
	}

	candidates := npcPrisonerSpawnRoomCandidates(mapState)
	all := deterministicNPCPrisonerSpawnRooms("spawn-cap", mapState, len(candidates)+16)
	if len(all) != len(candidates) {
		t.Fatalf("expected deterministic spawn to cap at candidate count, candidates=%d got=%d", len(candidates), len(all))
	}

	varied := make(map[model.RoomID]struct{}, len(candidates))
	for index := 0; index < 12; index++ {
		rooms := deterministicNPCPrisonerSpawnRooms(model.MatchID(fmt.Sprintf("spawn-%d", index)), mapState, 1)
		if len(rooms) != 1 {
			t.Fatalf("expected single-room deterministic spawn result, got %v", rooms)
		}
		varied[rooms[0]] = struct{}{}
	}
	if len(varied) < 2 {
		t.Fatalf("expected deterministic spawn output to vary across different match ids, got %v", varied)
	}
}
