package persistence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"prison-break/internal/gamecore/winconditions"
	"prison-break/internal/shared/model"
)

const CurrentSchemaVersion = 1

var (
	ErrInvalidPlayerID        = errors.New("persistence: invalid player id")
	ErrInvalidMatchRecord     = errors.New("persistence: invalid match record input")
	ErrUnsupportedSchema      = errors.New("persistence: unsupported schema version")
	ErrUnknownSchemaMigration = errors.New("persistence: missing schema migration step")
)

type Config struct {
	Path string

	// MaxMatchHistory controls retained match records.
	// 0 means unlimited history.
	MaxMatchHistory int
}

func DefaultConfig() Config {
	return Config{
		Path:            "",
		MaxMatchHistory: 0,
	}
}

func (config Config) normalized() Config {
	defaults := DefaultConfig()

	config.Path = strings.TrimSpace(config.Path)
	if config.MaxMatchHistory < 0 {
		config.MaxMatchHistory = defaults.MaxMatchHistory
	}
	return config
}

type AccountStats struct {
	MatchesPlayed   uint32 `json:"matches_played"`
	MatchesWon      uint32 `json:"matches_won"`
	MatchesEscaped  uint32 `json:"matches_escaped"`
	MatchesSurvived uint32 `json:"matches_survived"`
	Deaths          uint32 `json:"deaths"`
}

type Account struct {
	PlayerID    model.PlayerID `json:"player_id"`
	DisplayName string         `json:"display_name"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	LastSeenAt  time.Time      `json:"last_seen_at"`
	Stats       AccountStats   `json:"stats"`
}

type MatchPlayerRecord struct {
	PlayerID  model.PlayerID      `json:"player_id"`
	Name      string              `json:"name"`
	Role      model.RoleType      `json:"role"`
	Faction   model.FactionType   `json:"faction"`
	Alignment model.AlignmentType `json:"alignment"`
	Alive     bool                `json:"alive"`
	Escaped   bool                `json:"escaped"`
	Winner    bool                `json:"winner"`
}

type MatchRecord struct {
	MatchID         model.MatchID       `json:"match_id"`
	EndedAt         time.Time           `json:"ended_at"`
	EndedTick       uint64              `json:"ended_tick"`
	Reason          model.WinReason     `json:"reason"`
	WinnerPlayerIDs []model.PlayerID    `json:"winner_player_ids,omitempty"`
	Players         []MatchPlayerRecord `json:"players"`
}

type storeFileV0 struct {
	Accounts []Account     `json:"accounts,omitempty"`
	Matches  []MatchRecord `json:"matches,omitempty"`
}

type storeFileV1 struct {
	SchemaVersion int                        `json:"schema_version"`
	Accounts      map[model.PlayerID]Account `json:"accounts"`
	Matches       []MatchRecord              `json:"matches,omitempty"`
}

type Store struct {
	mu sync.RWMutex

	path            string
	maxMatchHistory int
	data            storeFileV1
}

func Open(config Config) (*Store, error) {
	normalized := config.normalized()

	store := &Store{
		path:            normalized.Path,
		maxMatchHistory: normalized.MaxMatchHistory,
		data: storeFileV1{
			SchemaVersion: CurrentSchemaVersion,
			Accounts:      make(map[model.PlayerID]Account),
			Matches:       make([]MatchRecord, 0, 64),
		},
	}

	if normalized.Path == "" {
		return store, nil
	}

	loaded, err := loadStoreFile(normalized.Path)
	if err != nil {
		return nil, err
	}
	store.data = loaded
	store.enforceHistoryCapLocked()
	store.recomputeStatsLocked()

	if err := store.persistLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (store *Store) SchemaVersion() int {
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.data.SchemaVersion
}

func (store *Store) UpsertAccount(playerID model.PlayerID, displayName string, observedAt time.Time) error {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return ErrInvalidPlayerID
	}
	playerID = model.PlayerID(trimmedID)
	displayName = strings.TrimSpace(displayName)

	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	account := store.ensureAccountLocked(playerID, displayName, observedAt)
	account.UpdatedAt = observedAt
	account.LastSeenAt = observedAt
	if displayName != "" {
		account.DisplayName = displayName
	}
	if account.DisplayName == "" {
		account.DisplayName = fallbackDisplayName(playerID)
	}
	store.data.Accounts[playerID] = account

	return store.persistLocked()
}

func (store *Store) RecordMatch(state model.GameState, endedAt time.Time) error {
	if state.MatchID == "" || state.GameOver == nil {
		return ErrInvalidMatchRecord
	}

	if endedAt.IsZero() {
		endedAt = time.Now().UTC()
	} else {
		endedAt = endedAt.UTC()
	}

	record := buildMatchRecord(state, endedAt)

	store.mu.Lock()
	defer store.mu.Unlock()

	// Upsert by match ID so retries/replays do not inflate stats.
	replaced := false
	for index := range store.data.Matches {
		if store.data.Matches[index].MatchID == record.MatchID {
			store.data.Matches[index] = record
			replaced = true
			break
		}
	}
	if !replaced {
		store.data.Matches = append(store.data.Matches, record)
	}

	store.sortMatchesLocked()
	store.enforceHistoryCapLocked()
	store.recomputeStatsLocked()
	return store.persistLocked()
}

func (store *Store) Account(playerID model.PlayerID) (Account, bool) {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return Account{}, false
	}

	store.mu.RLock()
	defer store.mu.RUnlock()

	account, exists := store.data.Accounts[model.PlayerID(trimmedID)]
	if !exists {
		return Account{}, false
	}
	return cloneAccount(account), true
}

func (store *Store) Accounts() []Account {
	store.mu.RLock()
	defer store.mu.RUnlock()

	out := make([]Account, 0, len(store.data.Accounts))
	for _, account := range store.data.Accounts {
		out = append(out, cloneAccount(account))
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i].PlayerID < out[j].PlayerID
	})
	return out
}

func (store *Store) MatchHistory(limit int) []MatchRecord {
	store.mu.RLock()
	defer store.mu.RUnlock()

	return cloneHistoryWithLimit(store.data.Matches, limit)
}

func (store *Store) PlayerMatchHistory(playerID model.PlayerID, limit int) []MatchRecord {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return nil
	}
	playerID = model.PlayerID(trimmedID)

	store.mu.RLock()
	defer store.mu.RUnlock()

	filtered := make([]MatchRecord, 0, len(store.data.Matches))
	for _, record := range store.data.Matches {
		if matchHasPlayer(record, playerID) {
			filtered = append(filtered, record)
		}
	}
	return cloneHistoryWithLimit(filtered, limit)
}

func cloneHistoryWithLimit(in []MatchRecord, limit int) []MatchRecord {
	if len(in) == 0 {
		return nil
	}

	if limit > 0 && len(in) > limit {
		in = in[:limit]
	}

	out := make([]MatchRecord, len(in))
	for index := range in {
		out[index] = cloneMatchRecord(in[index])
	}
	return out
}

func matchHasPlayer(record MatchRecord, playerID model.PlayerID) bool {
	for _, player := range record.Players {
		if player.PlayerID == playerID {
			return true
		}
	}
	return false
}

func buildMatchRecord(state model.GameState, endedAt time.Time) MatchRecord {
	winnerSet := make(map[model.PlayerID]struct{}, len(state.GameOver.WinnerPlayerIDs))
	for _, winner := range state.GameOver.WinnerPlayerIDs {
		winnerSet[winner] = struct{}{}
	}

	players := make([]MatchPlayerRecord, 0, len(state.Players))
	for _, player := range state.Players {
		_, winner := winnerSet[player.ID]
		players = append(players, MatchPlayerRecord{
			PlayerID:  player.ID,
			Name:      player.Name,
			Role:      player.Role,
			Faction:   player.Faction,
			Alignment: player.Alignment,
			Alive:     player.Alive,
			Escaped:   player.CurrentRoomID == winconditions.EscapedRoomID,
			Winner:    winner,
		})
	}
	sort.Slice(players, func(i int, j int) bool {
		return players[i].PlayerID < players[j].PlayerID
	})

	winners := append([]model.PlayerID(nil), state.GameOver.WinnerPlayerIDs...)
	sort.Slice(winners, func(i int, j int) bool {
		return winners[i] < winners[j]
	})

	return MatchRecord{
		MatchID:         state.MatchID,
		EndedAt:         endedAt,
		EndedTick:       state.TickID,
		Reason:          state.GameOver.Reason,
		WinnerPlayerIDs: winners,
		Players:         players,
	}
}

func (store *Store) ensureAccountLocked(playerID model.PlayerID, displayName string, observedAt time.Time) Account {
	account, exists := store.data.Accounts[playerID]
	if !exists {
		account = Account{
			PlayerID:    playerID,
			DisplayName: strings.TrimSpace(displayName),
			CreatedAt:   observedAt,
			UpdatedAt:   observedAt,
			LastSeenAt:  observedAt,
		}
	}

	if account.CreatedAt.IsZero() {
		account.CreatedAt = observedAt
	}
	if displayName != "" {
		account.DisplayName = displayName
	}
	if account.DisplayName == "" {
		account.DisplayName = fallbackDisplayName(playerID)
	}
	if observedAt.After(account.LastSeenAt) {
		account.LastSeenAt = observedAt
	}
	if observedAt.After(account.UpdatedAt) {
		account.UpdatedAt = observedAt
	}
	return account
}

func (store *Store) recomputeStatsLocked() {
	for playerID, account := range store.data.Accounts {
		account.Stats = AccountStats{}
		store.data.Accounts[playerID] = account
	}

	for _, record := range store.data.Matches {
		for _, player := range record.Players {
			if player.PlayerID == "" {
				continue
			}

			account := store.ensureAccountLocked(player.PlayerID, player.Name, record.EndedAt)
			account.Stats.MatchesPlayed++
			if player.Winner {
				account.Stats.MatchesWon++
			}
			if player.Escaped {
				account.Stats.MatchesEscaped++
			}
			if player.Alive {
				account.Stats.MatchesSurvived++
			} else {
				account.Stats.Deaths++
			}
			store.data.Accounts[player.PlayerID] = account
		}
	}
}

func (store *Store) sortMatchesLocked() {
	sort.Slice(store.data.Matches, func(i int, j int) bool {
		left := store.data.Matches[i]
		right := store.data.Matches[j]
		if left.EndedAt.Equal(right.EndedAt) {
			return left.MatchID > right.MatchID
		}
		return left.EndedAt.After(right.EndedAt)
	})
}

func (store *Store) enforceHistoryCapLocked() {
	if store.maxMatchHistory <= 0 || len(store.data.Matches) <= store.maxMatchHistory {
		return
	}

	trimmed := make([]MatchRecord, store.maxMatchHistory)
	copy(trimmed, store.data.Matches[:store.maxMatchHistory])
	store.data.Matches = trimmed
}

func fallbackDisplayName(playerID model.PlayerID) string {
	if playerID == "" {
		return "Player"
	}
	return string(playerID)
}

func (store *Store) persistLocked() error {
	if store.path == "" {
		return nil
	}

	store.data.SchemaVersion = CurrentSchemaVersion

	raw, err := json.MarshalIndent(store.data, "", "  ")
	if err != nil {
		return err
	}

	directory := filepath.Dir(store.path)
	if directory != "" && directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}

	tempPath := store.path + ".tmp"
	if err := os.WriteFile(tempPath, raw, 0o644); err != nil {
		return err
	}

	if err := os.Remove(store.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, store.path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

type migrationFunc func(raw []byte) ([]byte, error)

var schemaMigrations = map[int]migrationFunc{
	0: migrateV0ToV1,
}

func loadStoreFile(path string) (storeFileV1, error) {
	out := storeFileV1{
		SchemaVersion: CurrentSchemaVersion,
		Accounts:      make(map[model.PlayerID]Account),
		Matches:       make([]MatchRecord, 0, 64),
	}

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return out, nil
	}

	version, err := detectSchemaVersion(raw)
	if err != nil {
		return out, err
	}

	currentRaw := raw
	for version < CurrentSchemaVersion {
		migrate, exists := schemaMigrations[version]
		if !exists {
			return out, fmt.Errorf("%w: from %d to %d", ErrUnknownSchemaMigration, version, CurrentSchemaVersion)
		}
		currentRaw, err = migrate(currentRaw)
		if err != nil {
			return out, err
		}
		version++
	}
	if version > CurrentSchemaVersion {
		return out, fmt.Errorf("%w: %d", ErrUnsupportedSchema, version)
	}

	if err := json.Unmarshal(currentRaw, &out); err != nil {
		return out, err
	}
	if out.Accounts == nil {
		out.Accounts = make(map[model.PlayerID]Account)
	}
	if out.Matches == nil {
		out.Matches = make([]MatchRecord, 0, 64)
	}
	out.SchemaVersion = CurrentSchemaVersion
	sort.Slice(out.Matches, func(i int, j int) bool {
		if out.Matches[i].EndedAt.Equal(out.Matches[j].EndedAt) {
			return out.Matches[i].MatchID > out.Matches[j].MatchID
		}
		return out.Matches[i].EndedAt.After(out.Matches[j].EndedAt)
	})

	return out, nil
}

func detectSchemaVersion(raw []byte) (int, error) {
	var probe struct {
		SchemaVersion *int `json:"schema_version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 0, err
	}
	if probe.SchemaVersion == nil {
		return 0, nil
	}
	return *probe.SchemaVersion, nil
}

func migrateV0ToV1(raw []byte) ([]byte, error) {
	var legacy storeFileV0
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return nil, err
	}

	accounts := make(map[model.PlayerID]Account, len(legacy.Accounts))
	for _, account := range legacy.Accounts {
		if account.PlayerID == "" {
			continue
		}

		existing, exists := accounts[account.PlayerID]
		if !exists || account.UpdatedAt.After(existing.UpdatedAt) {
			if account.DisplayName == "" {
				account.DisplayName = fallbackDisplayName(account.PlayerID)
			}
			accounts[account.PlayerID] = account
		}
	}

	next := storeFileV1{
		SchemaVersion: CurrentSchemaVersion,
		Accounts:      accounts,
		Matches:       append([]MatchRecord(nil), legacy.Matches...),
	}
	if next.Matches == nil {
		next.Matches = make([]MatchRecord, 0, 64)
	}

	return json.Marshal(next)
}

func cloneAccount(in Account) Account {
	return in
}

func cloneMatchRecord(in MatchRecord) MatchRecord {
	out := in
	out.WinnerPlayerIDs = append([]model.PlayerID(nil), in.WinnerPlayerIDs...)
	out.Players = append([]MatchPlayerRecord(nil), in.Players...)
	return out
}
