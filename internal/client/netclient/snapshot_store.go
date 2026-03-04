package netclient

import (
	"sort"
	"sync"

	"prison-break/internal/shared/model"
)

type SnapshotStore struct {
	mu    sync.RWMutex
	state *model.GameState
	meta  *SnapshotMeta
}

func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{}
}

type SnapshotMeta struct {
	TickID     uint64
	PlayerAcks []model.PlayerAck
}

func (s *SnapshotStore) ApplySnapshot(snapshot model.Snapshot) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch snapshot.Kind {
	case model.SnapshotKindFull:
		if snapshot.State == nil {
			return false
		}
		next := cloneGameState(*snapshot.State)
		s.state = &next
		s.meta = &SnapshotMeta{
			TickID:     snapshot.TickID,
			PlayerAcks: clonePlayerAcks(snapshot.PlayerAcks),
		}
		return true

	case model.SnapshotKindDelta:
		if snapshot.Delta == nil || s.state == nil {
			return false
		}
		if snapshot.TickID <= s.state.TickID {
			return false
		}

		applyDeltaLocked(s.state, *snapshot.Delta)
		s.state.TickID = snapshot.TickID
		s.meta = &SnapshotMeta{
			TickID:     snapshot.TickID,
			PlayerAcks: clonePlayerAcks(snapshot.PlayerAcks),
		}
		return true

	default:
		return false
	}
}

func (s *SnapshotStore) CurrentState() (model.GameState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.state == nil {
		return model.GameState{}, false
	}

	return cloneGameState(*s.state), true
}

func (s *SnapshotStore) LatestSnapshotMeta() (SnapshotMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.meta == nil {
		return SnapshotMeta{}, false
	}

	return SnapshotMeta{
		TickID:     s.meta.TickID,
		PlayerAcks: clonePlayerAcks(s.meta.PlayerAcks),
	}, true
}

func applyDeltaLocked(state *model.GameState, delta model.GameDelta) {
	if state == nil {
		return
	}

	state.Players = upsertPlayers(state.Players, delta.ChangedPlayers)
	state.Players = removePlayers(state.Players, delta.RemovedPlayerIDs)

	state.Entities = upsertEntities(state.Entities, delta.ChangedEntities)
	state.Entities = removeEntities(state.Entities, delta.RemovedEntityIDs)

	state.Map.Doors = upsertDoors(state.Map.Doors, delta.ChangedDoors)
	state.Map.Cells = upsertCells(state.Map.Cells, delta.ChangedCells)
	state.Map.RestrictedZones = upsertZones(state.Map.RestrictedZones, delta.ChangedZones)

	if delta.Phase != nil {
		state.Phase = *delta.Phase
	}
	if delta.Status != nil {
		state.Status = *delta.Status
	}
	if delta.CycleCount != nil {
		state.CycleCount = *delta.CycleCount
	}
	if delta.PowerOn != nil {
		state.Map.PowerOn = *delta.PowerOn
	}
	if delta.Alarm != nil {
		state.Map.Alarm = *delta.Alarm
	}
	if delta.BlackMarketRoomID != nil {
		state.Map.BlackMarketRoomID = *delta.BlackMarketRoomID
	}
	if delta.GameOver != nil {
		gameOver := *delta.GameOver
		gameOver.WinnerPlayerIDs = append([]model.PlayerID(nil), delta.GameOver.WinnerPlayerIDs...)
		state.GameOver = &gameOver
	}
}

func upsertPlayers(existing []model.PlayerState, changed []model.PlayerState) []model.PlayerState {
	if len(changed) == 0 {
		return existing
	}

	byID := make(map[model.PlayerID]model.PlayerState, len(existing)+len(changed))
	for _, player := range existing {
		byID[player.ID] = clonePlayerState(player)
	}
	for _, player := range changed {
		byID[player.ID] = clonePlayerState(player)
	}

	ids := make([]model.PlayerID, 0, len(byID))
	for playerID := range byID {
		ids = append(ids, playerID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.PlayerState, 0, len(ids))
	for _, playerID := range ids {
		out = append(out, byID[playerID])
	}
	return out
}

func removePlayers(existing []model.PlayerState, removed []model.PlayerID) []model.PlayerState {
	if len(removed) == 0 || len(existing) == 0 {
		return existing
	}

	removedSet := make(map[model.PlayerID]struct{}, len(removed))
	for _, playerID := range removed {
		removedSet[playerID] = struct{}{}
	}

	filtered := existing[:0]
	for _, player := range existing {
		if _, remove := removedSet[player.ID]; remove {
			continue
		}
		filtered = append(filtered, player)
	}

	out := make([]model.PlayerState, len(filtered))
	copy(out, filtered)
	return out
}

func upsertEntities(existing []model.EntityState, changed []model.EntityState) []model.EntityState {
	if len(changed) == 0 {
		return existing
	}

	byID := make(map[model.EntityID]model.EntityState, len(existing)+len(changed))
	for _, entity := range existing {
		clone := entity
		clone.Tags = append([]string(nil), entity.Tags...)
		byID[entity.ID] = clone
	}
	for _, entity := range changed {
		clone := entity
		clone.Tags = append([]string(nil), entity.Tags...)
		byID[entity.ID] = clone
	}

	ids := make([]model.EntityID, 0, len(byID))
	for entityID := range byID {
		ids = append(ids, entityID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.EntityState, 0, len(ids))
	for _, entityID := range ids {
		out = append(out, byID[entityID])
	}
	return out
}

func removeEntities(existing []model.EntityState, removed []model.EntityID) []model.EntityState {
	if len(removed) == 0 || len(existing) == 0 {
		return existing
	}

	removedSet := make(map[model.EntityID]struct{}, len(removed))
	for _, entityID := range removed {
		removedSet[entityID] = struct{}{}
	}

	filtered := existing[:0]
	for _, entity := range existing {
		if _, remove := removedSet[entity.ID]; remove {
			continue
		}
		filtered = append(filtered, entity)
	}

	out := make([]model.EntityState, len(filtered))
	copy(out, filtered)
	return out
}

func upsertDoors(existing []model.DoorState, changed []model.DoorState) []model.DoorState {
	if len(changed) == 0 {
		return existing
	}

	byID := make(map[model.DoorID]model.DoorState, len(existing)+len(changed))
	for _, door := range existing {
		byID[door.ID] = door
	}
	for _, door := range changed {
		byID[door.ID] = door
	}

	ids := make([]model.DoorID, 0, len(byID))
	for doorID := range byID {
		ids = append(ids, doorID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.DoorState, 0, len(ids))
	for _, doorID := range ids {
		out = append(out, byID[doorID])
	}
	return out
}

func upsertCells(existing []model.CellState, changed []model.CellState) []model.CellState {
	if len(changed) == 0 {
		return existing
	}

	byID := make(map[model.CellID]model.CellState, len(existing)+len(changed))
	for _, cell := range existing {
		clone := cell
		clone.OccupantPlayerIDs = append([]model.PlayerID(nil), cell.OccupantPlayerIDs...)
		clone.Stash = append([]model.ItemStack(nil), cell.Stash...)
		byID[cell.ID] = clone
	}
	for _, cell := range changed {
		clone := cell
		clone.OccupantPlayerIDs = append([]model.PlayerID(nil), cell.OccupantPlayerIDs...)
		clone.Stash = append([]model.ItemStack(nil), cell.Stash...)
		byID[cell.ID] = clone
	}

	ids := make([]model.CellID, 0, len(byID))
	for cellID := range byID {
		ids = append(ids, cellID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.CellState, 0, len(ids))
	for _, cellID := range ids {
		out = append(out, byID[cellID])
	}
	return out
}

func upsertZones(existing []model.ZoneState, changed []model.ZoneState) []model.ZoneState {
	if len(changed) == 0 {
		return existing
	}

	byID := make(map[model.ZoneID]model.ZoneState, len(existing)+len(changed))
	for _, zone := range existing {
		byID[zone.ID] = zone
	}
	for _, zone := range changed {
		byID[zone.ID] = zone
	}

	ids := make([]model.ZoneID, 0, len(byID))
	for zoneID := range byID {
		ids = append(ids, zoneID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.ZoneState, 0, len(ids))
	for _, zoneID := range ids {
		out = append(out, byID[zoneID])
	}
	return out
}

func cloneGameState(in model.GameState) model.GameState {
	out := in

	out.Players = make([]model.PlayerState, len(in.Players))
	for index := range in.Players {
		out.Players[index] = clonePlayerState(in.Players[index])
	}

	out.Entities = make([]model.EntityState, len(in.Entities))
	for index := range in.Entities {
		entity := in.Entities[index]
		entity.Tags = append([]string(nil), in.Entities[index].Tags...)
		out.Entities[index] = entity
	}

	out.Map.Doors = append([]model.DoorState(nil), in.Map.Doors...)
	out.Map.Cells = make([]model.CellState, len(in.Map.Cells))
	for index := range in.Map.Cells {
		out.Map.Cells[index] = in.Map.Cells[index]
		out.Map.Cells[index].OccupantPlayerIDs = append([]model.PlayerID(nil), in.Map.Cells[index].OccupantPlayerIDs...)
		out.Map.Cells[index].Stash = append([]model.ItemStack(nil), in.Map.Cells[index].Stash...)
	}
	out.Map.RestrictedZones = append([]model.ZoneState(nil), in.Map.RestrictedZones...)

	if in.GameOver != nil {
		gameOver := *in.GameOver
		gameOver.WinnerPlayerIDs = append([]model.PlayerID(nil), in.GameOver.WinnerPlayerIDs...)
		out.GameOver = &gameOver
	}

	return out
}

func clonePlayerState(in model.PlayerState) model.PlayerState {
	out := in
	out.Inventory = append([]model.ItemStack(nil), in.Inventory...)
	out.Cards = append([]model.CardType(nil), in.Cards...)
	out.NightCardChoices = append([]model.CardType(nil), in.NightCardChoices...)
	out.Effects = append([]model.EffectState(nil), in.Effects...)
	return out
}

func clonePlayerAcks(in []model.PlayerAck) []model.PlayerAck {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.PlayerAck, len(in))
	copy(out, in)
	return out
}
