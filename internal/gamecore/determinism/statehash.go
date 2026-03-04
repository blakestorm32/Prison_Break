package determinism

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"prison-break/internal/shared/model"
)

func HashGameState(state model.GameState) (string, error) {
	normalized := cloneGameState(state)
	normalizeState(&normalized)

	raw, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeState(state *model.GameState) {
	sort.Slice(state.Players, func(i int, j int) bool {
		return state.Players[i].ID < state.Players[j].ID
	})

	for i := range state.Players {
		player := &state.Players[i]

		sort.Slice(player.Inventory, func(a int, b int) bool {
			if player.Inventory[a].Item != player.Inventory[b].Item {
				return player.Inventory[a].Item < player.Inventory[b].Item
			}
			return player.Inventory[a].Quantity < player.Inventory[b].Quantity
		})

		sort.Slice(player.Cards, func(a int, b int) bool {
			return player.Cards[a] < player.Cards[b]
		})

		sort.Slice(player.Effects, func(a int, b int) bool {
			left := player.Effects[a]
			right := player.Effects[b]
			if left.Effect != right.Effect {
				return left.Effect < right.Effect
			}
			if left.EndsTick != right.EndsTick {
				return left.EndsTick < right.EndsTick
			}
			if left.SourcePID != right.SourcePID {
				return left.SourcePID < right.SourcePID
			}
			if left.SourceID != right.SourceID {
				return left.SourceID < right.SourceID
			}
			return left.Stacks < right.Stacks
		})
	}

	sort.Slice(state.Map.Doors, func(i int, j int) bool {
		return state.Map.Doors[i].ID < state.Map.Doors[j].ID
	})

	sort.Slice(state.Map.Cells, func(i int, j int) bool {
		return state.Map.Cells[i].ID < state.Map.Cells[j].ID
	})
	for i := range state.Map.Cells {
		cell := &state.Map.Cells[i]
		sort.Slice(cell.OccupantPlayerIDs, func(a int, b int) bool {
			return cell.OccupantPlayerIDs[a] < cell.OccupantPlayerIDs[b]
		})
	}

	sort.Slice(state.Map.RestrictedZones, func(i int, j int) bool {
		return state.Map.RestrictedZones[i].ID < state.Map.RestrictedZones[j].ID
	})

	sort.Slice(state.Entities, func(i int, j int) bool {
		return state.Entities[i].ID < state.Entities[j].ID
	})
	for i := range state.Entities {
		sort.Strings(state.Entities[i].Tags)
	}

	if state.GameOver != nil {
		sort.Slice(state.GameOver.WinnerPlayerIDs, func(i int, j int) bool {
			return state.GameOver.WinnerPlayerIDs[i] < state.GameOver.WinnerPlayerIDs[j]
		})
	}
}
