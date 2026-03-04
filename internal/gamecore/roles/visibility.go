package roles

import "prison-break/internal/shared/model"

func ProjectSnapshotForViewer(snapshot model.Snapshot, viewerID model.PlayerID) model.Snapshot {
	out := snapshot

	if snapshot.State != nil {
		stateCopy := *snapshot.State
		stateCopy.Players = make([]model.PlayerState, len(snapshot.State.Players))
		for idx := range snapshot.State.Players {
			stateCopy.Players[idx] = projectPlayerForViewer(snapshot.State.Players[idx], viewerID)
		}
		out.State = &stateCopy
	}

	if snapshot.Delta != nil {
		deltaCopy := *snapshot.Delta
		deltaCopy.ChangedPlayers = make([]model.PlayerState, len(snapshot.Delta.ChangedPlayers))
		for idx := range snapshot.Delta.ChangedPlayers {
			deltaCopy.ChangedPlayers[idx] = projectPlayerForViewer(snapshot.Delta.ChangedPlayers[idx], viewerID)
		}
		out.Delta = &deltaCopy
	}

	out.PlayerAcks = append([]model.PlayerAck(nil), snapshot.PlayerAcks...)
	return out
}

func projectPlayerForViewer(player model.PlayerState, viewerID model.PlayerID) model.PlayerState {
	out := clonePlayerState(player)
	if viewerID != "" && out.ID == viewerID {
		return out
	}

	hidePrivatePlayerState(&out)

	// Warden identity remains public, while alignments stay hidden.
	if out.Role == model.RoleWarden {
		out.Alignment = ""
		return out
	}

	out.Role = ""
	out.Faction = ""
	out.Alignment = ""
	return out
}

func hidePrivatePlayerState(player *model.PlayerState) {
	if player == nil {
		return
	}

	player.Inventory = nil
	player.Cards = nil
	player.Effects = nil
	player.LastEscapeAttempt = model.EscapeAttemptFeedback{}

	// Ammo and temporary-heart economy are private loadout state.
	player.Bullets = 0
	player.TempHeartsHalf = 0

	// Detailed control-state timers remain private to avoid role/info leakage.
	player.StunnedUntilTick = 0
	player.SolitaryUntilTick = 0
}

func clonePlayerState(in model.PlayerState) model.PlayerState {
	out := in
	out.Inventory = append([]model.ItemStack(nil), in.Inventory...)
	out.Cards = append([]model.CardType(nil), in.Cards...)
	out.Effects = append([]model.EffectState(nil), in.Effects...)
	return out
}
