package persistence

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prison-break/internal/shared/model"
)

func TestOpenInMemoryAndUpsertAccount(t *testing.T) {
	store, err := Open(DefaultConfig())
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}

	now := time.Date(2026, 3, 4, 12, 0, 0, 0, time.UTC)
	if err := store.UpsertAccount("p2", "PlayerTwo", now); err != nil {
		t.Fatalf("upsert p2: %v", err)
	}
	if err := store.UpsertAccount("p1", "PlayerOne", now.Add(5*time.Minute)); err != nil {
		t.Fatalf("upsert p1: %v", err)
	}

	account, exists := store.Account("p1")
	if !exists {
		t.Fatalf("expected account p1 to exist")
	}
	if account.DisplayName != "PlayerOne" {
		t.Fatalf("expected account display name PlayerOne, got %q", account.DisplayName)
	}
	if !account.LastSeenAt.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("expected account last seen update, got %s", account.LastSeenAt)
	}

	accounts := store.Accounts()
	if len(accounts) != 2 {
		t.Fatalf("expected two accounts, got %d", len(accounts))
	}
	if accounts[0].PlayerID != "p1" || accounts[1].PlayerID != "p2" {
		t.Fatalf("expected accounts sorted by player id, got %+v", accounts)
	}
}

func TestRecordMatchUpdatesStatsHistoryAndPlayerFilter(t *testing.T) {
	store, err := Open(DefaultConfig())
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}

	endedAt := time.Date(2026, 3, 4, 12, 10, 0, 0, time.UTC)
	if err := store.RecordMatch(sampleFinishedState(
		"match-100",
		11,
		model.WinReasonGangLeaderEscaped,
		[]model.PlayerID{"p1"},
		[]samplePlayer{
			{ID: "p1", Name: "Leader", Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alive: true, Escaped: true},
			{ID: "p2", Name: "Deputy", Role: model.RoleDeputy, Faction: model.FactionAuthority, Alive: false, Escaped: false},
		},
	), endedAt); err != nil {
		t.Fatalf("record match: %v", err)
	}

	history := store.MatchHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected one match record, got %d", len(history))
	}
	record := history[0]
	if record.MatchID != "match-100" || record.Reason != model.WinReasonGangLeaderEscaped {
		t.Fatalf("unexpected match record: %+v", record)
	}
	if len(record.Players) != 2 {
		t.Fatalf("expected two player records, got %+v", record.Players)
	}

	p1, exists := store.Account("p1")
	if !exists {
		t.Fatalf("expected p1 account")
	}
	if p1.Stats.MatchesPlayed != 1 || p1.Stats.MatchesWon != 1 || p1.Stats.MatchesEscaped != 1 || p1.Stats.MatchesSurvived != 1 || p1.Stats.Deaths != 0 {
		t.Fatalf("unexpected p1 stats: %+v", p1.Stats)
	}

	p2, exists := store.Account("p2")
	if !exists {
		t.Fatalf("expected p2 account")
	}
	if p2.Stats.MatchesPlayed != 1 || p2.Stats.MatchesWon != 0 || p2.Stats.MatchesEscaped != 0 || p2.Stats.MatchesSurvived != 0 || p2.Stats.Deaths != 1 {
		t.Fatalf("unexpected p2 stats: %+v", p2.Stats)
	}

	playerHistory := store.PlayerMatchHistory("p2", 10)
	if len(playerHistory) != 1 || playerHistory[0].MatchID != "match-100" {
		t.Fatalf("expected player-specific history for p2, got %+v", playerHistory)
	}
	if none := store.PlayerMatchHistory("unknown", 10); len(none) != 0 {
		t.Fatalf("expected empty history for unknown player, got %+v", none)
	}
}

func TestRecordMatchIsIdempotentByMatchID(t *testing.T) {
	store, err := Open(DefaultConfig())
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}

	first := sampleFinishedState(
		"match-200",
		8,
		model.WinReasonGangLeaderEscaped,
		[]model.PlayerID{"p1"},
		[]samplePlayer{
			{ID: "p1", Name: "One", Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alive: true, Escaped: true},
			{ID: "p2", Name: "Two", Role: model.RoleDeputy, Faction: model.FactionAuthority, Alive: false, Escaped: false},
		},
	)
	second := sampleFinishedState(
		"match-200",
		9,
		model.WinReasonWardenDied,
		[]model.PlayerID{"p2"},
		[]samplePlayer{
			{ID: "p1", Name: "One", Role: model.RoleGangLeader, Faction: model.FactionPrisoner, Alive: false, Escaped: false},
			{ID: "p2", Name: "Two", Role: model.RoleDeputy, Faction: model.FactionAuthority, Alive: true, Escaped: false},
		},
	)

	if err := store.RecordMatch(first, time.Date(2026, 3, 4, 13, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("record first version: %v", err)
	}
	if err := store.RecordMatch(second, time.Date(2026, 3, 4, 13, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("record second version: %v", err)
	}

	history := store.MatchHistory(10)
	if len(history) != 1 {
		t.Fatalf("expected one idempotent record, got %d", len(history))
	}
	if history[0].Reason != model.WinReasonWardenDied {
		t.Fatalf("expected latest record version to replace old one, got %+v", history[0])
	}

	p1, _ := store.Account("p1")
	p2, _ := store.Account("p2")
	if p1.Stats.MatchesPlayed != 1 || p2.Stats.MatchesPlayed != 1 {
		t.Fatalf("expected one match counted per player, got p1=%+v p2=%+v", p1.Stats, p2.Stats)
	}
	if p1.Stats.MatchesWon != 0 || p2.Stats.MatchesWon != 1 {
		t.Fatalf("expected stats to reflect latest winner set, got p1=%+v p2=%+v", p1.Stats, p2.Stats)
	}
}

func TestMatchHistoryLimitAndInputValidation(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "persist.json")
	store, err := Open(Config{
		Path:            path,
		MaxMatchHistory: 2,
	})
	if err != nil {
		t.Fatalf("open file store: %v", err)
	}

	for i := 1; i <= 3; i++ {
		matchID := model.MatchID("match-limit-" + string(rune('0'+i)))
		state := sampleFinishedState(
			matchID,
			uint64(i),
			model.WinReasonNoEscapesAtTimeLimit,
			[]model.PlayerID{"p1"},
			[]samplePlayer{
				{ID: "p1", Name: "One", Role: model.RoleWarden, Faction: model.FactionAuthority, Alive: true, Escaped: false},
			},
		)
		if err := store.RecordMatch(state, time.Date(2026, 3, 4, 14, i, 0, 0, time.UTC)); err != nil {
			t.Fatalf("record limited match %d: %v", i, err)
		}
	}

	history := store.MatchHistory(10)
	if len(history) != 2 {
		t.Fatalf("expected capped history size 2, got %d", len(history))
	}
	if history[0].MatchID != "match-limit-3" || history[1].MatchID != "match-limit-2" {
		t.Fatalf("expected newest-first capped history, got %+v", history)
	}

	if err := store.UpsertAccount("", "bad", time.Now()); !errors.Is(err, ErrInvalidPlayerID) {
		t.Fatalf("expected ErrInvalidPlayerID, got %v", err)
	}
	if err := store.RecordMatch(model.GameState{MatchID: "x"}, time.Now()); !errors.Is(err, ErrInvalidMatchRecord) {
		t.Fatalf("expected ErrInvalidMatchRecord for missing game over state, got %v", err)
	}
}

func TestFilePersistenceRoundTripAndMigration(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "legacy_store.json")

	legacy := storeFileV0{
		Accounts: []Account{
			{
				PlayerID:    "legacy-player",
				DisplayName: "Legacy",
				CreatedAt:   time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
				LastSeenAt:  time.Date(2026, 3, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy v0 file: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write legacy v0 file: %v", err)
	}

	store, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	if got := store.SchemaVersion(); got != CurrentSchemaVersion {
		t.Fatalf("expected migrated schema version %d, got %d", CurrentSchemaVersion, got)
	}

	account, exists := store.Account("legacy-player")
	if !exists || account.DisplayName != "Legacy" {
		t.Fatalf("expected migrated account, got exists=%t account=%+v", exists, account)
	}

	if err := store.UpsertAccount("legacy-player", "LegacyRenamed", time.Date(2026, 3, 4, 15, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("upsert migrated account: %v", err)
	}

	newRaw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migrated file: %v", err)
	}
	if !strings.Contains(string(newRaw), "\"schema_version\": 1") {
		t.Fatalf("expected persisted file to include schema_version=1, raw=%s", string(newRaw))
	}

	unsupportedPath := filepath.Join(tempDir, "unsupported.json")
	if err := os.WriteFile(unsupportedPath, []byte(`{"schema_version":99,"accounts":{},"matches":[]}`), 0o644); err != nil {
		t.Fatalf("write unsupported schema file: %v", err)
	}
	if _, err := Open(Config{Path: unsupportedPath}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("expected ErrUnsupportedSchema, got %v", err)
	}
}

type samplePlayer struct {
	ID        model.PlayerID
	Name      string
	Role      model.RoleType
	Faction   model.FactionType
	Alignment model.AlignmentType
	Alive     bool
	Escaped   bool
}

func sampleFinishedState(
	matchID model.MatchID,
	tickID uint64,
	reason model.WinReason,
	winners []model.PlayerID,
	players []samplePlayer,
) model.GameState {
	statePlayers := make([]model.PlayerState, 0, len(players))
	for _, player := range players {
		roomID := model.RoomID("corridor_main")
		if player.Escaped {
			roomID = model.RoomID("escaped")
		}
		statePlayers = append(statePlayers, model.PlayerState{
			ID:            player.ID,
			Name:          player.Name,
			Role:          player.Role,
			Faction:       player.Faction,
			Alignment:     player.Alignment,
			Alive:         player.Alive,
			CurrentRoomID: roomID,
		})
	}

	return model.GameState{
		MatchID: matchID,
		TickID:  tickID,
		Status:  model.MatchStatusGameOver,
		Players: statePlayers,
		GameOver: &model.GameOverState{
			Reason:          reason,
			EndedTick:       tickID,
			WinnerPlayerIDs: append([]model.PlayerID(nil), winners...),
		},
	}
}
