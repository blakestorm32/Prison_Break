package networking

import (
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestQuickPlayRegionPreferenceCreatesAndReusesRegionalLobbies(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    4,
		MaxPlayers:    6,
		TickRateHz:    30,
		MatchIDPrefix: "region",
	})
	defer h.Close(t)

	eastA := h.Dial(t)
	defer eastA.Close()
	westA := h.Dial(t)
	defer westA.Close()
	eastB := h.Dial(t)
	defer eastB.Close()

	matchEast := joinClientWithRegionHints(t, eastA, "east-1", "", "us-east", nil)
	matchWest := joinClientWithRegionHints(t, westA, "west-1", "", "us-west", nil)
	matchEastSecond := joinClientWithRegionHints(t, eastB, "east-2", "", "us-east", nil)

	if matchEast == matchWest {
		t.Fatalf("expected distinct lobbies for distinct region preferences, got %s", matchEast)
	}
	if matchEastSecond != matchEast {
		t.Fatalf("expected us-east player to reuse us-east lobby %s, got %s", matchEast, matchEastSecond)
	}

	summaries := h.server.LobbySummaries(false)
	regionByMatch := make(map[model.MatchID]string, len(summaries))
	for _, summary := range summaries {
		regionByMatch[summary.MatchID] = summary.Region
	}
	if got := regionByMatch[matchEast]; got != "us-east" {
		t.Fatalf("expected match %s region us-east, got %q", matchEast, got)
	}
	if got := regionByMatch[matchWest]; got != "us-west" {
		t.Fatalf("expected match %s region us-west, got %q", matchWest, got)
	}
}

func TestQuickPlayWithoutPreferredRegionUsesLatencyHint(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    5,
		MaxPlayers:    8,
		TickRateHz:    30,
		MatchIDPrefix: "region-lat",
	})
	defer h.Close(t)

	eastA := h.Dial(t)
	defer eastA.Close()
	eastB := h.Dial(t)
	defer eastB.Close()
	westA := h.Dial(t)
	defer westA.Close()
	auto := h.Dial(t)
	defer auto.Close()

	matchEast := joinClientWithRegionHints(t, eastA, "east-1", "", "us-east", nil)
	_ = joinClientWithRegionHints(t, eastB, "east-2", "", "us-east", nil)
	matchWest := joinClientWithRegionHints(t, westA, "west-1", "", "us-west", nil)

	autoMatch := joinClientWithRegionHints(t, auto, "auto-1", "", "", map[string]uint16{
		"us-east": 120,
		"us-west": 45,
	})
	if autoMatch != matchWest {
		t.Fatalf("expected latency-optimized quick play to choose %s, got %s (east lobby %s)", matchWest, autoMatch, matchEast)
	}
}

func TestListLobbiesRespectsRegionPreferenceAndLatencyHints(t *testing.T) {
	h := newHarness(t, game.Config{
		MinPlayers:    4,
		MaxPlayers:    8,
		TickRateHz:    30,
		MatchIDPrefix: "region-list",
	})
	defer h.Close(t)

	eastA := h.Dial(t)
	defer eastA.Close()
	eastB := h.Dial(t)
	defer eastB.Close()
	westA := h.Dial(t)
	defer westA.Close()
	queryClient := h.Dial(t)
	defer queryClient.Close()

	_ = joinClientWithRegionHints(t, eastA, "east-1", "", "us-east", nil)
	_ = joinClientWithRegionHints(t, eastB, "east-2", "", "us-east", nil)
	_ = joinClientWithRegionHints(t, westA, "west-1", "", "us-west", nil)

	requestEnvelope, err := protocol.NewEnvelope(protocol.MsgListLobbies, protocol.ListLobbiesRequest{
		PreferredRegion: "us-west",
		RegionLatencyMS: map[string]uint16{
			"us-east": 140,
			"us-west": 40,
		},
	})
	if err != nil {
		t.Fatalf("build list_lobbies envelope: %v", err)
	}
	writeEnvelope(t, queryClient, requestEnvelope)

	response := readUntilType(t, queryClient, protocol.MsgLobbyList, 2*time.Second)
	payload, err := protocol.DecodePayload[protocol.LobbyListMessage](response)
	if err != nil {
		t.Fatalf("decode lobby_list payload: %v", err)
	}
	if len(payload.Lobbies) < 2 {
		t.Fatalf("expected at least two lobbies, got %d", len(payload.Lobbies))
	}
	if payload.Lobbies[0].Region != "us-west" {
		t.Fatalf("expected preferred region lobby first, got region=%q", payload.Lobbies[0].Region)
	}
}

func joinClientWithRegionHints(
	t *testing.T,
	client *websocket.Conn,
	playerID model.PlayerID,
	preferredMatchID model.MatchID,
	preferredRegion string,
	regionLatencyMS map[string]uint16,
) model.MatchID {
	t.Helper()

	joinEnvelope, err := protocol.NewEnvelope(protocol.MsgJoinGame, protocol.JoinGameRequest{
		PlayerName:       string(playerID),
		PreferredMatchID: preferredMatchID,
		PreferredRegion:  preferredRegion,
		RegionLatencyMS:  regionLatencyMS,
	})
	if err != nil {
		t.Fatalf("build join envelope: %v", err)
	}
	joinEnvelope.PlayerID = playerID

	writeEnvelope(t, client, joinEnvelope)
	response := readUntilType(t, client, protocol.MsgJoinAccepted, 2*time.Second)
	accepted, err := protocol.DecodePayload[protocol.JoinGameAccepted](response)
	if err != nil {
		t.Fatalf("decode join_accepted payload: %v", err)
	}
	return accepted.MatchID
}
