package netclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

func TestFetchLobbiesReturnsSummaries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lobbies" {
			t.Fatalf("expected /lobbies path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocol.LobbyListMessage{
			Lobbies: []protocol.LobbySummary{
				{
					MatchID:      "match-1",
					Status:       model.MatchStatusLobby,
					PlayerCount:  1,
					MinPlayers:   2,
					MaxPlayers:   6,
					OpenSlots:    5,
					Joinable:     true,
					ReadyToStart: false,
				},
			},
		})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	lobbies, err := FetchLobbies(ctx, wsURL, false)
	if err != nil {
		t.Fatalf("fetch lobbies returned error: %v", err)
	}
	if len(lobbies) != 1 {
		t.Fatalf("expected one lobby summary, got %d", len(lobbies))
	}
	if lobbies[0].MatchID != "match-1" || lobbies[0].MinPlayers != 2 || lobbies[0].MaxPlayers != 6 {
		t.Fatalf("unexpected lobby summary payload: %+v", lobbies[0])
	}
}

func TestFetchLobbiesIncludeRunningAddsQueryFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("include_running"); got != "true" {
			t.Fatalf("expected include_running=true query flag, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocol.LobbyListMessage{})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := FetchLobbies(ctx, wsURL, true)
	if err != nil {
		t.Fatalf("fetch lobbies returned error: %v", err)
	}
}

func TestFetchLobbiesWithAuthSendsBearerHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer lobby-token" {
			t.Fatalf("expected Authorization bearer header, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocol.LobbyListMessage{})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := FetchLobbiesWithAuth(ctx, wsURL, false, " lobby-token ")
	if err != nil {
		t.Fatalf("fetch lobbies with auth returned error: %v", err)
	}
}

func TestFetchLobbiesWithPreferencesAddsRegionQueryParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("preferred_region"); got != "us-east" {
			t.Fatalf("expected preferred_region=us-east, got %q", got)
		}
		if got := r.URL.Query().Get("region_latency_ms"); got != "eu:120,us-east:45,us-west:90" {
			t.Fatalf("expected region_latency_ms query serialization, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(protocol.LobbyListMessage{})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := FetchLobbiesWithPreferencesAndAuth(
		ctx,
		wsURL,
		false,
		"",
		"US-East",
		map[string]uint16{
			"US-East": 45,
			"us-west": 90,
			"eu":      120,
		},
	)
	if err != nil {
		t.Fatalf("fetch lobbies with preferences returned error: %v", err)
	}
}

func TestFetchLobbiesRejectsUnsupportedScheme(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := FetchLobbies(ctx, "tcp://127.0.0.1:8080/ws", false)
	if err == nil {
		t.Fatalf("expected unsupported scheme error")
	}
}

func TestFetchLobbiesReturnsStatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := FetchLobbies(ctx, wsURL, false)
	if err == nil {
		t.Fatalf("expected non-200 status error")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Fatalf("expected status code details in error, got %v", err)
	}
}

func TestWSURLToLobbiesHTTPURLConvertsSchemesAndPath(t *testing.T) {
	got, err := wsURLToLobbiesHTTPURL("wss://example.com:8443/ws", true)
	if err != nil {
		t.Fatalf("convert ws URL failed: %v", err)
	}
	if got != "https://example.com:8443/lobbies?include_running=true" {
		t.Fatalf("unexpected converted URL: %s", got)
	}
}
