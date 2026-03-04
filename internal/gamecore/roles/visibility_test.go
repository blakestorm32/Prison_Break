package roles

import (
	"testing"

	"prison-break/internal/shared/model"
)

func TestProjectSnapshotForViewerKeepsSelfAndHidesOthers(t *testing.T) {
	snapshot := model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 10,
		State: &model.GameState{
			Players: []model.PlayerState{
				{
					ID:        "warden",
					Role:      model.RoleWarden,
					Faction:   model.FactionAuthority,
					Alignment: model.AlignmentGood,
					Bullets:   9,
					Inventory: []model.ItemStack{
						{Item: model.ItemPistol, Quantity: 1},
					},
					Cards: []model.CardType{
						model.CardMoney,
					},
					LastEscapeAttempt: model.EscapeAttemptFeedback{
						Route:  model.EscapeRouteCourtyardDig,
						Status: model.EscapeAttemptStatusFailed,
						Reason: "missing shovel",
						TickID: 10,
					},
				},
				{
					ID:        "deputy",
					Role:      model.RoleDeputy,
					Faction:   model.FactionAuthority,
					Alignment: model.AlignmentEvil,
					Bullets:   6,
					Inventory: []model.ItemStack{
						{Item: model.ItemShiv, Quantity: 1},
					},
					Cards: []model.CardType{
						model.CardSpeed,
					},
				},
				{
					ID:        "gang",
					Role:      model.RoleGangLeader,
					Faction:   model.FactionPrisoner,
					Alignment: model.AlignmentEvil,
					Bullets:   2,
					Inventory: []model.ItemStack{
						{Item: model.ItemShovel, Quantity: 1},
					},
					Cards: []model.CardType{
						model.CardMoney,
					},
				},
			},
		},
		Delta: &model.GameDelta{
			ChangedPlayers: []model.PlayerState{
				{
					ID:        "deputy",
					Role:      model.RoleDeputy,
					Faction:   model.FactionAuthority,
					Alignment: model.AlignmentEvil,
				},
				{
					ID:        "gang",
					Role:      model.RoleGangMember,
					Faction:   model.FactionPrisoner,
					Alignment: model.AlignmentEvil,
				},
			},
		},
	}

	projected := ProjectSnapshotForViewer(snapshot, "deputy")
	if projected.State == nil || projected.Delta == nil {
		t.Fatalf("expected projected state and delta")
	}

	var sawSelf bool
	var sawPublicWarden bool
	var sawHiddenGang bool
	for _, player := range projected.State.Players {
		switch player.ID {
		case "deputy":
			sawSelf = true
			if player.Role != model.RoleDeputy || player.Alignment != model.AlignmentEvil || player.Faction != model.FactionAuthority || player.Bullets != 6 {
				t.Fatalf("expected self details preserved, got %+v", player)
			}
			if len(player.Inventory) != 1 || len(player.Cards) != 1 {
				t.Fatalf("expected self private inventory/cards preserved, got %+v", player)
			}
		case "warden":
			sawPublicWarden = true
			if player.Role != model.RoleWarden || player.Faction != model.FactionAuthority {
				t.Fatalf("expected warden identity public, got %+v", player)
			}
			if player.Alignment != "" {
				t.Fatalf("expected warden alignment hidden from others, got %+v", player)
			}
			if len(player.Inventory) != 0 || len(player.Cards) != 0 || player.Bullets != 0 {
				t.Fatalf("expected non-self warden private loadout hidden, got %+v", player)
			}
			if player.LastEscapeAttempt.Route != "" {
				t.Fatalf("expected non-self warden escape feedback hidden, got %+v", player.LastEscapeAttempt)
			}
		case "gang":
			sawHiddenGang = true
			if player.Role != "" || player.Faction != "" || player.Alignment != "" {
				t.Fatalf("expected gang role hidden, got %+v", player)
			}
			if len(player.Inventory) != 0 || len(player.Cards) != 0 || player.Bullets != 0 {
				t.Fatalf("expected hidden gang private loadout to be removed, got %+v", player)
			}
		}
	}

	if !sawSelf || !sawPublicWarden || !sawHiddenGang {
		t.Fatalf("expected self/warden/gang visibility checks to execute")
	}

	for _, player := range projected.Delta.ChangedPlayers {
		if player.ID == "gang" && (player.Role != "" || player.Faction != "" || player.Alignment != "") {
			t.Fatalf("expected hidden role in delta changed player, got %+v", player)
		}
		if player.ID == "gang" && (len(player.Inventory) != 0 || len(player.Cards) != 0 || player.Bullets != 0) {
			t.Fatalf("expected hidden private loadout in delta changed player, got %+v", player)
		}
	}
}

func TestProjectSnapshotForViewerWithUnknownViewerStillHidesNonWarden(t *testing.T) {
	snapshot := model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 5,
		State: &model.GameState{
			Players: []model.PlayerState{
				{
					ID:        "warden",
					Role:      model.RoleWarden,
					Faction:   model.FactionAuthority,
					Alignment: model.AlignmentGood,
				},
				{
					ID:        "snitch",
					Role:      model.RoleSnitch,
					Faction:   model.FactionPrisoner,
					Alignment: model.AlignmentGood,
				},
			},
		},
	}

	projected := ProjectSnapshotForViewer(snapshot, "spectator")
	if projected.State == nil {
		t.Fatalf("expected projected state")
	}

	for _, player := range projected.State.Players {
		if player.ID == "warden" {
			if player.Role != model.RoleWarden {
				t.Fatalf("expected warden role visible")
			}
			continue
		}
		if player.Role != "" || player.Faction != "" || player.Alignment != "" {
			t.Fatalf("expected hidden role for non-warden spectator view, got %+v", player)
		}
	}
}

func TestProjectSnapshotForViewerWithEmptyViewerStillHidesNonWarden(t *testing.T) {
	snapshot := model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 7,
		State: &model.GameState{
			Players: []model.PlayerState{
				{
					ID:        "warden",
					Role:      model.RoleWarden,
					Faction:   model.FactionAuthority,
					Alignment: model.AlignmentGood,
				},
				{
					ID:        "gang",
					Role:      model.RoleGangLeader,
					Faction:   model.FactionPrisoner,
					Alignment: model.AlignmentEvil,
				},
			},
		},
	}

	projected := ProjectSnapshotForViewer(snapshot, "")
	if projected.State == nil {
		t.Fatalf("expected projected state")
	}

	for _, player := range projected.State.Players {
		if player.ID == "warden" {
			if player.Role != model.RoleWarden || player.Faction != model.FactionAuthority {
				t.Fatalf("expected warden identity public, got %+v", player)
			}
			if player.Alignment != "" {
				t.Fatalf("expected warden alignment hidden from empty viewer, got %+v", player)
			}
			continue
		}
		if player.Role != "" || player.Faction != "" || player.Alignment != "" {
			t.Fatalf("expected non-warden role hidden for empty viewer, got %+v", player)
		}
	}
}

func TestProjectSnapshotVisibilityPolicyCoversAllRoles(t *testing.T) {
	snapshot := model.Snapshot{
		Kind:   model.SnapshotKindFull,
		TickID: 22,
		State: &model.GameState{
			Players: []model.PlayerState{
				{ID: "warden", Role: model.RoleWarden, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
				{ID: "deputy", Role: model.RoleDeputy, Faction: model.FactionAuthority, Alignment: model.AlignmentGood},
				{ID: "leader", Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
				{ID: "member", Role: model.RoleGangMember, Faction: model.FactionPrisoner, Alignment: model.AlignmentEvil},
				{ID: "snitch", Role: model.RoleSnitch, Faction: model.FactionPrisoner, Alignment: model.AlignmentGood},
				{ID: "neutral", Role: model.RoleNeutralPrisoner, Faction: model.FactionNeutral, Alignment: model.AlignmentNeutral},
			},
		},
	}

	projected := ProjectSnapshotForViewer(snapshot, "viewer")
	if projected.State == nil {
		t.Fatalf("expected projected state")
	}

	for _, player := range projected.State.Players {
		switch player.ID {
		case "warden":
			if player.Role != model.RoleWarden || player.Faction != model.FactionAuthority {
				t.Fatalf("expected warden to remain public identity, got %+v", player)
			}
			if player.Alignment != "" {
				t.Fatalf("expected warden alignment hidden, got %+v", player)
			}
		default:
			if player.Role != "" || player.Faction != "" || player.Alignment != "" {
				t.Fatalf("expected role/faction/alignment hidden for %s, got %+v", player.ID, player)
			}
		}
	}
}
