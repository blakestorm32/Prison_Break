package game

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"prison-break/internal/engine/physics"
	"prison-break/internal/gamecore/abilities"
	"prison-break/internal/gamecore/cards"
	"prison-break/internal/gamecore/combat"
	"prison-break/internal/gamecore/escape"
	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/gamecore/phase"
	"prison-break/internal/gamecore/prison"
	"prison-break/internal/gamecore/roles"
	"prison-break/internal/gamecore/winconditions"
	"prison-break/internal/shared/model"
)

type LifecycleEventType string

const (
	LifecycleEventMatchCreated LifecycleEventType = "match_created"
	LifecycleEventPlayerJoined LifecycleEventType = "player_joined"
	LifecycleEventMatchStarted LifecycleEventType = "match_started"
	LifecycleEventTick         LifecycleEventType = "tick"
	LifecycleEventMatchEnded   LifecycleEventType = "match_ended"
	LifecycleEventShutdown     LifecycleEventType = "match_shutdown"
)

type LifecycleEvent struct {
	Type     LifecycleEventType `json:"type"`
	MatchID  model.MatchID      `json:"match_id"`
	PlayerID model.PlayerID     `json:"player_id,omitempty"`
	TickID   uint64             `json:"tick_id,omitempty"`
	At       time.Time          `json:"at"`
	Note     string             `json:"note,omitempty"`
}

type PlayerSession struct {
	PlayerID model.PlayerID `json:"player_id"`
	Name     string         `json:"name"`
	JoinedAt time.Time      `json:"joined_at"`
}

type MatchSnapshot struct {
	MatchID model.MatchID     `json:"match_id"`
	Status  model.MatchStatus `json:"status"`

	CreatedAt time.Time  `json:"created_at"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`

	EndedReason string          `json:"ended_reason,omitempty"`
	TickID      uint64          `json:"tick_id"`
	Players     []PlayerSession `json:"players"`
}

type lifecycleTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type tickerFactory func(interval time.Duration) lifecycleTicker

type realTicker struct {
	ticker *time.Ticker
}

func newRealTicker(interval time.Duration) lifecycleTicker {
	return &realTicker{ticker: time.NewTicker(interval)}
}

func (t *realTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t *realTicker) Stop() {
	t.ticker.Stop()
}

type matchSession struct {
	matchID    model.MatchID
	status     model.MatchStatus
	tickRateHz uint32

	createdAt   time.Time
	startedAt   *time.Time
	endedAt     *time.Time
	endedReason string

	tickID uint64

	players map[model.PlayerID]PlayerSession

	nextIngressSeq              uint64
	nextEntityID                model.EntityID
	blackMarketGoldenBulletSold bool
	alarmUsedDayStartTick       uint64
	scheduledInputs             map[uint64][]model.InputCommand
	seenClientSeq               map[model.PlayerID]map[uint64]struct{}
	acceptedByTick              map[uint64]map[model.PlayerID]uint8
	lastProcessedClientSeq      map[model.PlayerID]uint64
	guardEntityByRoom           map[model.RoomID]model.EntityID
	guardLastShotTick           map[model.PlayerID]uint64
	abilityCooldownUntil        map[model.PlayerID]map[model.AbilityType]uint64
	abilityUsedDayStart         map[model.PlayerID]map[model.AbilityType]uint64
	abilityDailyUsage           map[model.PlayerID]map[model.AbilityType]dailyAbilityUsage
	prisonerUnlockedRooms       map[model.RoomID]struct{}
	lockSnapDoorRestores        map[model.DoorID]lockSnapDoorRestoreState
	npcPrisonerBribeState       map[model.EntityID]npcPrisonerBribeState
	npcTaskByPlayer             map[model.PlayerID]npcTaskState
	chameleonPendingUntil       map[model.PlayerID]uint64
	lastMoveIntentTick          map[model.PlayerID]uint64
	ammoRestockUntil            map[model.PlayerID]uint64
	projectileExpireTick        map[model.EntityID]uint64
	replayEntries               []ReplayEntry

	gameState       model.GameState
	snapshotHistory map[uint64]model.Snapshot

	loopCancel context.CancelFunc
	loopDone   chan struct{}
}

type tickMutations struct {
	players          []model.PlayerState
	doors            []model.DoorState
	cells            []model.CellState
	entities         []model.EntityState
	removedEntityIDs []model.EntityID
}

type lockSnapDoorRestoreState struct {
	RestoreTick uint64
	Locked      bool
	Open        bool
}

type npcPrisonerBribeState struct {
	OfferItem model.ItemType
	OfferCost uint8
	Stock     uint8
	PayerID   model.PlayerID
	PaidCards uint8
}

type dailyAbilityUsage struct {
	DayStartTick uint64
	UseCount     uint8
}

type npcTaskType string

const (
	npcTaskVisitRoom npcTaskType = "visit_room"
	npcTaskHoldItem  npcTaskType = "hold_item"

	playerInventorySlotCount uint8 = 3
	authorityAmmoMax         uint8 = 3
	authorityRestockSeconds        = 10
)

type npcTaskState struct {
	DayStartTick uint64
	Type         npcTaskType
	TargetRoomID model.RoomID
	TargetItem   model.ItemType
	RewardCards  uint8
	AssignedBy   model.EntityID
}

type PersistenceStore interface {
	UpsertAccount(playerID model.PlayerID, displayName string, observedAt time.Time) error
	RecordMatch(state model.GameState, endedAt time.Time) error
}

type Manager struct {
	mu sync.RWMutex

	config Config

	now       func() time.Time
	newTicker tickerFactory

	nextMatchSequence uint64
	matches           map[model.MatchID]*matchSession
	playerToMatch     map[model.PlayerID]model.MatchID
	events            []LifecycleEvent
	persistence       PersistenceStore
}

var defaultMatchLayout = gamemap.DefaultPrisonLayout()

var npcPrisonerRooms = []model.RoomID{
	gamemap.RoomCafeteria,
	gamemap.RoomCourtyard,
	gamemap.RoomCellBlockA,
}

type managerPhaseHooks struct{}

func (managerPhaseHooks) OnDayStart(state *model.GameState, _ phase.Transition) {
	if state == nil {
		return
	}

	state.Map.Alarm = model.AlarmState{
		Active: false,
	}
}

func (managerPhaseHooks) OnNightStart(state *model.GameState, transition phase.Transition) {
	if state == nil {
		return
	}

	state.Map.BlackMarketRoomID = gamemap.DeterministicNightlyBlackMarketRoom(
		state.MatchID,
		transition.Cycle,
		transition.StartTick,
	)
}

func NewManager(config Config) *Manager {
	return newManagerWithDeps(config, time.Now, newRealTicker)
}

func newManagerWithDeps(config Config, now func() time.Time, newTicker tickerFactory) *Manager {
	normalized := config.normalized()

	if now == nil {
		now = time.Now
	}
	if newTicker == nil {
		newTicker = newRealTicker
	}

	return &Manager{
		config:        normalized,
		now:           now,
		newTicker:     newTicker,
		matches:       make(map[model.MatchID]*matchSession),
		playerToMatch: make(map[model.PlayerID]model.MatchID),
		events:        make([]LifecycleEvent, 0, 128),
	}
}

func (m *Manager) CreateMatch() MatchSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	matchID := m.nextMatchIDLocked()
	createdAt := m.now().UTC()
	session := &matchSession{
		matchID:                matchID,
		status:                 model.MatchStatusLobby,
		tickRateHz:             m.config.TickRateHz,
		createdAt:              createdAt,
		players:                make(map[model.PlayerID]PlayerSession),
		scheduledInputs:        make(map[uint64][]model.InputCommand),
		seenClientSeq:          make(map[model.PlayerID]map[uint64]struct{}),
		acceptedByTick:         make(map[uint64]map[model.PlayerID]uint8),
		lastProcessedClientSeq: make(map[model.PlayerID]uint64),
		guardEntityByRoom:      make(map[model.RoomID]model.EntityID),
		guardLastShotTick:      make(map[model.PlayerID]uint64),
		abilityCooldownUntil:   make(map[model.PlayerID]map[model.AbilityType]uint64),
		abilityUsedDayStart:    make(map[model.PlayerID]map[model.AbilityType]uint64),
		abilityDailyUsage:      make(map[model.PlayerID]map[model.AbilityType]dailyAbilityUsage),
		prisonerUnlockedRooms:  make(map[model.RoomID]struct{}),
		lockSnapDoorRestores:   make(map[model.DoorID]lockSnapDoorRestoreState),
		npcPrisonerBribeState:  make(map[model.EntityID]npcPrisonerBribeState),
		npcTaskByPlayer:        make(map[model.PlayerID]npcTaskState),
		chameleonPendingUntil:  make(map[model.PlayerID]uint64),
		lastMoveIntentTick:     make(map[model.PlayerID]uint64),
		replayEntries:          make([]ReplayEntry, 0, 256),
		snapshotHistory:        make(map[uint64]model.Snapshot),
	}
	session.gameState = newInitialGameState(matchID, session.status, m.config)
	m.matches[matchID] = session

	m.appendEventLocked(LifecycleEvent{
		Type:    LifecycleEventMatchCreated,
		MatchID: matchID,
		At:      createdAt,
	})

	return snapshotFromSession(session)
}

func (m *Manager) JoinMatch(matchID model.MatchID, playerID model.PlayerID, playerName string) (MatchSnapshot, error) {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return MatchSnapshot{}, ErrInvalidPlayerID
	}

	trimmedName := strings.TrimSpace(playerName)
	if trimmedName == "" {
		return MatchSnapshot{}, ErrInvalidPlayerName
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, alreadyJoined := m.playerToMatch[playerID]; alreadyJoined {
		return MatchSnapshot{}, ErrPlayerAlreadyInMatch
	}

	session, exists := m.matches[matchID]
	if !exists {
		return MatchSnapshot{}, ErrMatchNotFound
	}

	switch session.status {
	case model.MatchStatusLobby:
		// Join is allowed.
	case model.MatchStatusRunning, model.MatchStatusGameOver:
		return MatchSnapshot{}, ErrMatchNotJoinable
	default:
		return MatchSnapshot{}, ErrMatchNotJoinable
	}

	if len(session.players) >= int(m.config.MaxPlayers) {
		return MatchSnapshot{}, ErrMatchFull
	}

	joinedAt := m.now().UTC()
	session.players[playerID] = PlayerSession{
		PlayerID: playerID,
		Name:     trimmedName,
		JoinedAt: joinedAt,
	}
	m.playerToMatch[playerID] = matchID
	if _, exists := session.lastProcessedClientSeq[playerID]; !exists {
		session.lastProcessedClientSeq[playerID] = 0
	}
	syncGameStatePlayersLocked(session)
	if m.persistence != nil {
		_ = m.persistence.UpsertAccount(playerID, trimmedName, joinedAt)
	}

	m.appendEventLocked(LifecycleEvent{
		Type:     LifecycleEventPlayerJoined,
		MatchID:  matchID,
		PlayerID: playerID,
		At:       joinedAt,
	})

	return snapshotFromSession(session), nil
}

func (m *Manager) MatchIDForPlayer(playerID model.PlayerID) (model.MatchID, bool) {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return "", false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	matchID, exists := m.playerToMatch[model.PlayerID(trimmedID)]
	return matchID, exists
}

func (m *Manager) ResumePlayer(matchID model.MatchID, playerID model.PlayerID, playerName string) (MatchSnapshot, error) {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return MatchSnapshot{}, ErrInvalidPlayerID
	}
	playerID = model.PlayerID(trimmedID)

	trimmedName := strings.TrimSpace(playerName)

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.matches[matchID]
	if !exists {
		return MatchSnapshot{}, ErrMatchNotFound
	}
	if session.status == model.MatchStatusGameOver {
		return MatchSnapshot{}, ErrMatchNotJoinable
	}

	if mappedMatchID, exists := m.playerToMatch[playerID]; exists && mappedMatchID != matchID {
		return MatchSnapshot{}, ErrPlayerAlreadyInMatch
	}

	playerSession, exists := session.players[playerID]
	if !exists {
		return MatchSnapshot{}, ErrPlayerNotFound
	}

	if trimmedName != "" {
		playerSession.Name = trimmedName
		session.players[playerID] = playerSession
	}
	m.playerToMatch[playerID] = matchID
	if _, exists := session.lastProcessedClientSeq[playerID]; !exists {
		session.lastProcessedClientSeq[playerID] = 0
	}
	syncGameStatePlayersLocked(session)
	_ = setPlayerConnectedLocked(session, playerID, true)
	if m.persistence != nil {
		nameForPersistence := playerSession.Name
		if trimmedName != "" {
			nameForPersistence = trimmedName
		}
		_ = m.persistence.UpsertAccount(playerID, nameForPersistence, m.now().UTC())
	}

	return snapshotFromSession(session), nil
}

func (m *Manager) SetPlayerConnected(matchID model.MatchID, playerID model.PlayerID, connected bool) error {
	trimmedID := strings.TrimSpace(string(playerID))
	if trimmedID == "" {
		return ErrInvalidPlayerID
	}
	playerID = model.PlayerID(trimmedID)

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.matches[matchID]
	if !exists {
		return ErrMatchNotFound
	}
	if _, exists := session.players[playerID]; !exists {
		return ErrPlayerNotFound
	}
	if _, exists := session.lastProcessedClientSeq[playerID]; !exists {
		session.lastProcessedClientSeq[playerID] = 0
	}

	if setPlayerConnectedLocked(session, playerID, connected) {
		return nil
	}

	syncGameStatePlayersLocked(session)
	if !setPlayerConnectedLocked(session, playerID, connected) {
		return ErrPlayerNotFound
	}
	return nil
}

func (m *Manager) StartMatch(matchID model.MatchID) (MatchSnapshot, error) {
	m.mu.Lock()

	session, exists := m.matches[matchID]
	if !exists {
		m.mu.Unlock()
		return MatchSnapshot{}, ErrMatchNotFound
	}

	switch session.status {
	case model.MatchStatusRunning:
		m.mu.Unlock()
		return MatchSnapshot{}, ErrMatchAlreadyRunning
	case model.MatchStatusGameOver:
		m.mu.Unlock()
		return MatchSnapshot{}, ErrMatchAlreadyEnded
	}

	if len(session.players) < int(m.config.MinPlayers) {
		m.mu.Unlock()
		return MatchSnapshot{}, ErrNotEnoughPlayers
	}

	startedAt := m.now().UTC()
	session.status = model.MatchStatusRunning
	session.startedAt = &startedAt
	session.tickID = 0
	session.gameState.Status = model.MatchStatusRunning
	session.gameState.TickID = 0
	session.gameState.CycleCount = 0
	session.gameState.Phase = phase.InitialPhaseState(m.config.phaseConfig(), 1)
	session.gameState.Map.PowerOn = true
	session.gameState.Map.Alarm = model.AlarmState{Active: false}
	session.gameState.Entities = nil
	session.nextEntityID = 0
	session.blackMarketGoldenBulletSold = false
	session.alarmUsedDayStartTick = 0
	session.guardEntityByRoom = make(map[model.RoomID]model.EntityID)
	session.guardLastShotTick = make(map[model.PlayerID]uint64)
	session.abilityCooldownUntil = make(map[model.PlayerID]map[model.AbilityType]uint64)
	session.abilityUsedDayStart = make(map[model.PlayerID]map[model.AbilityType]uint64)
	session.prisonerUnlockedRooms = make(map[model.RoomID]struct{})
	session.lockSnapDoorRestores = make(map[model.DoorID]lockSnapDoorRestoreState)
	session.npcPrisonerBribeState = make(map[model.EntityID]npcPrisonerBribeState)
	session.ammoRestockUntil = make(map[model.PlayerID]uint64)
	session.projectileExpireTick = make(map[model.EntityID]uint64)
	session.replayEntries = make([]ReplayEntry, 0, 256)
	syncGameStatePlayersLocked(session)
	_ = roles.ApplyAssignments(&session.gameState, session.matchID)
	assignRandomAbilitiesLocked(session)
	assignCellsLocked(session)
	combat.ApplyRoleLoadouts(&session.gameState)
	applyInventoryLoadoutsLocked(&session.gameState)
	spawnNPCPrisonersLocked(session)

	session.scheduledInputs = make(map[uint64][]model.InputCommand)
	session.acceptedByTick = make(map[uint64]map[model.PlayerID]uint8)
	session.seenClientSeq = make(map[model.PlayerID]map[uint64]struct{})
	session.lastProcessedClientSeq = make(map[model.PlayerID]uint64, len(session.players))
	for playerID := range session.players {
		session.lastProcessedClientSeq[playerID] = 0
	}
	session.snapshotHistory = make(map[uint64]model.Snapshot)
	initialSnapshot := makeFullSnapshotLocked(session)
	storeSnapshotLocked(session, initialSnapshot)

	ctx, cancel := context.WithCancel(context.Background())
	session.loopCancel = cancel
	session.loopDone = make(chan struct{})
	ticker := m.newTicker(m.config.TickInterval())

	m.appendEventLocked(LifecycleEvent{
		Type:    LifecycleEventMatchStarted,
		MatchID: matchID,
		At:      startedAt,
	})

	snapshot := snapshotFromSession(session)
	m.mu.Unlock()

	go m.runMatchLoop(matchID, ctx, ticker, session.loopDone)
	return snapshot, nil
}

func (m *Manager) EndMatch(matchID model.MatchID, reason string) (MatchSnapshot, error) {
	trimmedReason := strings.TrimSpace(reason)
	if trimmedReason == "" {
		trimmedReason = "manual_end"
	}

	var done chan struct{}

	m.mu.Lock()
	session, exists := m.matches[matchID]
	if !exists {
		m.mu.Unlock()
		return MatchSnapshot{}, ErrMatchNotFound
	}
	if session.status == model.MatchStatusGameOver {
		m.mu.Unlock()
		return MatchSnapshot{}, ErrMatchAlreadyEnded
	}

	endedAt := m.now().UTC()
	session.status = model.MatchStatusGameOver
	session.endedAt = &endedAt
	session.endedReason = trimmedReason
	session.gameState.Status = model.MatchStatusGameOver
	session.gameState.TickID = session.tickID
	session.scheduledInputs = make(map[uint64][]model.InputCommand)
	session.acceptedByTick = make(map[uint64]map[model.PlayerID]uint8)
	session.seenClientSeq = make(map[model.PlayerID]map[uint64]struct{})
	for playerID := range session.players {
		delete(m.playerToMatch, playerID)
	}

	if session.loopCancel != nil {
		session.loopCancel()
		session.loopCancel = nil
		done = session.loopDone
		session.loopDone = nil
	}

	m.appendEventLocked(LifecycleEvent{
		Type:    LifecycleEventMatchEnded,
		MatchID: matchID,
		At:      endedAt,
		Note:    trimmedReason,
	})

	snapshot := snapshotFromSession(session)
	m.mu.Unlock()

	if done != nil {
		<-done
	}

	return snapshot, nil
}

func (m *Manager) ApplyKnockback(
	matchID model.MatchID,
	playerID model.PlayerID,
	impulse model.Vector2,
	stunDurationTicks uint64,
) (model.PlayerState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.matches[matchID]
	if !exists {
		return model.PlayerState{}, ErrMatchNotFound
	}
	if session.status != model.MatchStatusRunning {
		return model.PlayerState{}, ErrMatchNotRunning
	}

	playerIndex := -1
	for idx := range session.gameState.Players {
		if session.gameState.Players[idx].ID == playerID {
			playerIndex = idx
			break
		}
	}
	if playerIndex < 0 {
		return model.PlayerState{}, ErrPlayerNotFound
	}

	occupied := physics.BuildOccupiedTiles(session.gameState.Players)
	next, _ := physics.ApplyKnockback(
		session.gameState.Players[playerIndex],
		impulse,
		defaultMatchLayout,
		session.gameState.Map,
		occupied,
		session.tickID,
		stunDurationTicks,
	)

	if roomID, exists := defaultMatchLayout.RoomAt(physics.TileFromPosition(next.Position)); exists {
		next.CurrentRoomID = roomID
	}
	session.gameState.Players[playerIndex] = next

	return clonePlayerState(next), nil
}

func (m *Manager) MatchSnapshot(matchID model.MatchID) (MatchSnapshot, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.matches[matchID]
	if !exists {
		return MatchSnapshot{}, false
	}

	return snapshotFromSession(session), true
}

func (m *Manager) MatchConstraints() (uint8, uint8) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.config.MinPlayers, m.config.MaxPlayers
}

func (m *Manager) TickRateHz() uint32 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.config.TickRateHz
}

func (m *Manager) BindPersistence(store PersistenceStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.persistence = store
}

func (m *Manager) ListMatchSnapshots() []MatchSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshots := make([]MatchSnapshot, 0, len(m.matches))
	for _, session := range m.matches {
		snapshots = append(snapshots, snapshotFromSession(session))
	}

	sort.Slice(snapshots, func(i int, j int) bool {
		return snapshots[i].MatchID < snapshots[j].MatchID
	})

	return snapshots
}

func (m *Manager) LifecycleEvents(matchID model.MatchID) []LifecycleEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filtered := make([]LifecycleEvent, 0, len(m.events))
	for _, event := range m.events {
		if matchID != "" && event.MatchID != matchID {
			continue
		}
		filtered = append(filtered, event)
	}

	return filtered
}

func (m *Manager) FullSnapshot(matchID model.MatchID) (model.Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.matches[matchID]
	if !exists {
		return model.Snapshot{}, ErrMatchNotFound
	}

	full := makeFullSnapshotLocked(session)
	return cloneSnapshot(full), nil
}

func (m *Manager) SnapshotsSince(matchID model.MatchID, afterTick uint64) ([]model.Snapshot, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.matches[matchID]
	if !exists {
		return nil, ErrMatchNotFound
	}

	ticks := make([]uint64, 0, len(session.snapshotHistory))
	for tickID := range session.snapshotHistory {
		if tickID > afterTick {
			ticks = append(ticks, tickID)
		}
	}
	sort.Slice(ticks, func(i int, j int) bool {
		return ticks[i] < ticks[j]
	})

	out := make([]model.Snapshot, 0, len(ticks))
	for _, tickID := range ticks {
		out = append(out, cloneSnapshot(session.snapshotHistory[tickID]))
	}

	return out, nil
}

func (m *Manager) Close() {
	type stopTarget struct {
		matchID model.MatchID
		done    chan struct{}
	}

	stopTargets := make([]stopTarget, 0, len(m.matches))

	m.mu.Lock()
	for matchID, session := range m.matches {
		for playerID := range session.players {
			delete(m.playerToMatch, playerID)
		}

		if session.status == model.MatchStatusRunning {
			endedAt := m.now().UTC()
			session.status = model.MatchStatusGameOver
			session.endedAt = &endedAt
			session.gameState.Status = model.MatchStatusGameOver
			session.gameState.TickID = session.tickID
			if strings.TrimSpace(session.endedReason) == "" {
				session.endedReason = "server_shutdown"
			}
			m.appendEventLocked(LifecycleEvent{
				Type:    LifecycleEventShutdown,
				MatchID: matchID,
				At:      endedAt,
				Note:    session.endedReason,
			})
		}

		session.scheduledInputs = make(map[uint64][]model.InputCommand)
		session.acceptedByTick = make(map[uint64]map[model.PlayerID]uint8)
		session.seenClientSeq = make(map[model.PlayerID]map[uint64]struct{})

		if session.loopCancel != nil {
			session.loopCancel()
			session.loopCancel = nil
		}
		if session.loopDone != nil {
			stopTargets = append(stopTargets, stopTarget{
				matchID: matchID,
				done:    session.loopDone,
			})
			session.loopDone = nil
		}
	}
	m.mu.Unlock()

	for _, target := range stopTargets {
		<-target.done
	}
}

func (m *Manager) nextMatchIDLocked() model.MatchID {
	m.nextMatchSequence++
	return model.MatchID(fmt.Sprintf("%s-%06d", m.config.MatchIDPrefix, m.nextMatchSequence))
}

func (m *Manager) appendEventLocked(event LifecycleEvent) {
	m.events = append(m.events, event)
}

func (m *Manager) runMatchLoop(matchID model.MatchID, loopCtx context.Context, ticker lifecycleTicker, done chan struct{}) {
	defer close(done)
	defer ticker.Stop()

	for {
		select {
		case <-loopCtx.Done():
			return
		case tickAt, ok := <-ticker.Chan():
			if !ok {
				return
			}

			m.mu.Lock()
			session, exists := m.matches[matchID]
			if !exists {
				m.mu.Unlock()
				return
			}
			if session.status != model.MatchStatusRunning {
				m.mu.Unlock()
				return
			}

			session.tickID++
			session.gameState.TickID = session.tickID

			timestamp := tickAt.UTC()
			if timestamp.IsZero() {
				timestamp = m.now().UTC()
			}

			commands := session.scheduledInputs[session.tickID]
			delete(session.scheduledInputs, session.tickID)
			sortInputCommands(commands)
			previousPower := session.gameState.Map.PowerOn
			previousCycle := session.gameState.CycleCount
			previousAlarm := session.gameState.Map.Alarm
			previousBlackMarket := session.gameState.Map.BlackMarketRoomID
			mutations := applyInputsToGameStateLocked(session, commands)
			previousStatus := session.gameState.Status
			hadGameOver := session.gameState.GameOver != nil
			phaseTransitions := phase.Advance(
				&session.gameState,
				session.tickID,
				m.config.phaseConfig(),
				managerPhaseHooks{},
			)
			nightlyEconomyEntityChanges := make([]model.EntityState, 0, 8)
			phasePlayerChanges := make(map[model.PlayerID]struct{})
			for _, transition := range phaseTransitions {
				if transition.To != model.PhaseNight {
					if transition.To == model.PhaseDay {
						for _, playerID := range clearNightCardChoicesLocked(session) {
							phasePlayerChanges[playerID] = struct{}{}
						}
						for _, playerID := range restoreAuthoritySidearmsForNewDayLocked(session) {
							phasePlayerChanges[playerID] = struct{}{}
						}
					}
					continue
				}
				nightlyEconomyEntityChanges = mergeChangedEntityStates(
					nightlyEconomyEntityChanges,
					refreshNPCPrisonerOffersForNightLocked(session, transition.Cycle),
				)
				for _, playerID := range assignNightCardOffersForCycleLocked(session, transition.Cycle) {
					phasePlayerChanges[playerID] = struct{}{}
				}
			}
			outcome := winconditions.Evaluate(session.gameState, winconditions.Config{
				MaxCycles: m.config.MaxCycles,
			})
			if outcome != nil {
				session.status = model.MatchStatusGameOver
				session.endedAt = &timestamp
				session.endedReason = string(outcome.Reason)
				session.gameState.Status = model.MatchStatusGameOver
				session.gameState.GameOver = outcome
				session.scheduledInputs = make(map[uint64][]model.InputCommand)
				session.acceptedByTick = make(map[uint64]map[model.PlayerID]uint8)
				session.seenClientSeq = make(map[model.PlayerID]map[uint64]struct{})
				for playerID := range session.players {
					delete(m.playerToMatch, playerID)
				}
				if m.persistence != nil {
					_ = m.persistence.RecordMatch(session.gameState, timestamp)
				}
				m.appendEventLocked(LifecycleEvent{
					Type:    LifecycleEventMatchEnded,
					MatchID: matchID,
					At:      timestamp,
					Note:    session.endedReason,
				})
			}
			playerAcks := buildPlayerAcksLocked(session)

			delta := &model.GameDelta{}
			changedPlayers := mergeChangedPlayersByIDLocked(session, mutations.players, phasePlayerChanges)
			if len(changedPlayers) > 0 {
				delta.ChangedPlayers = changedPlayers
			}
			changedEntities := mergeChangedEntityStates(mutations.entities, nightlyEconomyEntityChanges)
			if len(changedEntities) > 0 {
				delta.ChangedEntities = changedEntities
			}
			if len(mutations.removedEntityIDs) > 0 {
				delta.RemovedEntityIDs = mutations.removedEntityIDs
			}
			if len(mutations.doors) > 0 {
				delta.ChangedDoors = mutations.doors
			}
			if len(mutations.cells) > 0 {
				delta.ChangedCells = mutations.cells
			}
			if len(phaseTransitions) > 0 {
				phaseCopy := session.gameState.Phase
				delta.Phase = &phaseCopy
			}
			if session.gameState.CycleCount != previousCycle {
				cycleCopy := session.gameState.CycleCount
				delta.CycleCount = &cycleCopy
			}
			if session.gameState.Map.PowerOn != previousPower {
				powerCopy := session.gameState.Map.PowerOn
				delta.PowerOn = &powerCopy
			}
			if session.gameState.Map.Alarm != previousAlarm {
				alarmCopy := session.gameState.Map.Alarm
				delta.Alarm = &alarmCopy
			}
			if session.gameState.Map.BlackMarketRoomID != previousBlackMarket {
				roomCopy := session.gameState.Map.BlackMarketRoomID
				delta.BlackMarketRoomID = &roomCopy
			}
			if session.gameState.Status != previousStatus {
				statusCopy := session.gameState.Status
				delta.Status = &statusCopy
			}
			if !hadGameOver && session.gameState.GameOver != nil {
				gameOverCopy := *session.gameState.GameOver
				gameOverCopy.WinnerPlayerIDs = append([]model.PlayerID(nil), session.gameState.GameOver.WinnerPlayerIDs...)
				delta.GameOver = &gameOverCopy
			}
			snapshot := model.Snapshot{
				Kind:       model.SnapshotKindDelta,
				TickID:     session.tickID,
				BaseTickID: session.tickID - 1,
				Delta:      delta,
				PlayerAcks: playerAcks,
			}
			storeSnapshotLocked(session, snapshot)

			m.appendEventLocked(LifecycleEvent{
				Type:    LifecycleEventTick,
				MatchID: matchID,
				TickID:  session.tickID,
				At:      timestamp,
			})
			m.mu.Unlock()
		}
	}
}

func newInitialGameState(matchID model.MatchID, status model.MatchStatus, config Config) model.GameState {
	mapState := gamemap.DefaultPrisonLayout().ToMapState()
	mapState.PowerOn = true
	mapState.Alarm = model.AlarmState{
		Active: false,
	}
	initialPhase := phase.InitialPhaseState(config.phaseConfig(), 1)

	return model.GameState{
		MatchID: matchID,
		TickID:  0,
		Status:  status,
		Phase:   initialPhase,
		Map:     mapState,
		Players: make([]model.PlayerState, 0, 12),
	}
}

func syncGameStatePlayersLocked(session *matchSession) {
	existing := make(map[model.PlayerID]model.PlayerState, len(session.gameState.Players))
	for _, player := range session.gameState.Players {
		existing[player.ID] = player
	}

	playerIDs := make([]model.PlayerID, 0, len(session.players))
	for playerID := range session.players {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i int, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})

	nextPlayers := make([]model.PlayerState, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		playerSession := session.players[playerID]
		state, exists := existing[playerID]
		if !exists {
			state = model.PlayerState{
				ID:             playerID,
				Name:           playerSession.Name,
				Connected:      true,
				Alive:          true,
				HeartsHalf:     6,
				LivesRemaining: combat.DefaultPlayerLives,
				Position:       model.Vector2{},
				Velocity:       model.Vector2{},
				Facing: model.Vector2{
					X: 1,
					Y: 0,
				},
			}
		}

		state.ID = playerID
		state.Name = playerSession.Name
		if !exists {
			state.Connected = true
		}
		if !exists {
			state.Alive = true
		}

		nextPlayers = append(nextPlayers, state)
	}

	session.gameState.Players = nextPlayers
}

func assignRandomAbilitiesLocked(session *matchSession) {
	if session == nil || len(session.gameState.Players) == 0 {
		return
	}

	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		player.AssignedAbility = deterministicAssignedAbility(session.matchID, *player)
	}
}

func deterministicAssignedAbility(matchID model.MatchID, player model.PlayerState) model.AbilityType {
	eligible := abilities.AbilitiesForPlayer(player)
	if len(eligible) == 0 {
		return ""
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))
	_, _ = hasher.Write([]byte(player.ID))
	_, _ = hasher.Write([]byte(player.Role))
	choice := hasher.Sum64() % uint64(len(eligible))
	return eligible[choice]
}

func setPlayerConnectedLocked(session *matchSession, playerID model.PlayerID, connected bool) bool {
	if session == nil || playerID == "" {
		return false
	}

	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if player.ID != playerID {
			continue
		}
		player.Connected = connected
		if !connected {
			player.Velocity = model.Vector2{}
		}
		return true
	}
	return false
}

func assignCellsLocked(session *matchSession) {
	if len(session.gameState.Players) == 0 || len(session.gameState.Map.Cells) == 0 {
		return
	}

	for idx := range session.gameState.Map.Cells {
		session.gameState.Map.Cells[idx].OwnerPlayerID = ""
		session.gameState.Map.Cells[idx].OccupantPlayerIDs = nil
	}

	players := make([]*model.PlayerState, 0, len(session.gameState.Players))
	for idx := range session.gameState.Players {
		players = append(players, &session.gameState.Players[idx])
	}
	sort.Slice(players, func(i int, j int) bool {
		return players[i].ID < players[j].ID
	})

	prisonerSpawnIndex := 0
	deputySpawnIndex := 0
	for idx, player := range players {
		player.LockedInCell = 0
		player.Velocity = model.Vector2{}
		player.AssignedCell = 0

		if idx >= len(session.gameState.Map.Cells) {
			// Keep role-based spawn behavior even when player count exceeds seeded cell count.
			switch player.Role {
			case model.RoleWarden:
				player.CurrentRoomID = gamemap.RoomWardenHQ
				player.Position = spawnPositionInRoom(gamemap.RoomWardenHQ, 0)
			case model.RoleDeputy:
				player.CurrentRoomID = gamemap.RoomAmmoRoom
				player.Position = spawnPositionInRoom(gamemap.RoomAmmoRoom, deputySpawnIndex)
				deputySpawnIndex++
			default:
				player.CurrentRoomID = gamemap.RoomCellBlockA
				player.Position = spawnPositionForPlayerIndex(prisonerSpawnIndex)
				prisonerSpawnIndex++
			}
			continue
		}

		cell := &session.gameState.Map.Cells[idx]
		cell.OwnerPlayerID = player.ID
		cell.OccupantPlayerIDs = []model.PlayerID{player.ID}
		player.AssignedCell = cell.ID

		switch player.Role {
		case model.RoleWarden:
			player.CurrentRoomID = gamemap.RoomWardenHQ
			player.Position = spawnPositionInRoom(gamemap.RoomWardenHQ, 0)
		case model.RoleDeputy:
			player.CurrentRoomID = gamemap.RoomAmmoRoom
			player.Position = spawnPositionInRoom(gamemap.RoomAmmoRoom, deputySpawnIndex)
			deputySpawnIndex++
		default:
			player.CurrentRoomID = gamemap.RoomCellBlockA
			player.Position = spawnPositionForPlayerIndex(prisonerSpawnIndex)
			prisonerSpawnIndex++
		}
	}
}

func applyInventoryLoadoutsLocked(state *model.GameState) {
	if state == nil {
		return
	}

	for index := range state.Players {
		player := &state.Players[index]
		player.InventorySlots = playerInventorySlotCount

		if gamemap.IsAuthorityPlayer(*player) {
			player.Inventory = nil
			_ = items.AddItem(player, model.ItemBaton, 1)
			_ = items.AddItem(player, model.ItemPistol, 1)

			if player.Bullets == 0 {
				player.Bullets = authorityAmmoMax
			}
			if player.Bullets > authorityAmmoMax {
				player.Bullets = authorityAmmoMax
			}
			_ = syncAuthorityBulletStackLocked(player)
			player.EquippedItem = model.ItemPistol
			continue
		}

		normalizeInventoryToPlayerSlotsLocked(player)
		ensureEquippedItemForPlayerLocked(player)
	}
}

func normalizeInventoryToPlayerSlotsLocked(player *model.PlayerState) {
	if player == nil {
		return
	}

	canonical := append([]model.ItemStack(nil), player.Inventory...)
	sort.Slice(canonical, func(i int, j int) bool {
		return canonical[i].Item < canonical[j].Item
	})

	rebuilt := model.PlayerState{
		InventorySlots: player.InventorySlots,
	}
	for _, stack := range canonical {
		if stack.Item == "" || stack.Quantity == 0 || !items.IsKnownItem(stack.Item) {
			continue
		}
		_ = items.AddItem(&rebuilt, stack.Item, stack.Quantity)
	}
	player.Inventory = rebuilt.Inventory
}

func ensureEquippedItemForPlayerLocked(player *model.PlayerState) bool {
	if player == nil {
		return false
	}

	if player.EquippedItem != "" && combat.CanUseWeapon(*player, player.EquippedItem) {
		return false
	}

	next := preferredEquippedWeapon(*player)
	if player.EquippedItem == next {
		return false
	}
	player.EquippedItem = next
	return true
}

func preferredEquippedWeapon(player model.PlayerState) model.ItemType {
	priority := []model.ItemType{
		model.ItemPistol,
		model.ItemHuntingRifle,
		model.ItemBaton,
		model.ItemShiv,
	}
	for _, weapon := range priority {
		if combat.CanUseWeapon(player, weapon) {
			return weapon
		}
	}
	return ""
}

func syncAuthorityBulletStackLocked(player *model.PlayerState) bool {
	if player == nil || !gamemap.IsAuthorityPlayer(*player) {
		return false
	}

	changed := false
	if player.Bullets > authorityAmmoMax {
		player.Bullets = authorityAmmoMax
		changed = true
	}

	current := inventoryItemQuantity(player.Inventory, model.ItemBullet)
	if current > 0 {
		if items.RemoveItem(player, model.ItemBullet, current) {
			changed = true
		}
	}
	if player.Bullets > 0 {
		if items.AddItem(player, model.ItemBullet, player.Bullets) {
			changed = true
		}
	}

	return changed
}

func inventoryItemQuantity(inventory []model.ItemStack, item model.ItemType) uint8 {
	for _, stack := range inventory {
		if stack.Item != item {
			continue
		}
		return stack.Quantity
	}
	return 0
}

func spawnPositionInRoom(roomID model.RoomID, index int) model.Vector2 {
	if index < 0 {
		index = 0
	}

	room, exists := defaultMatchLayout.Room(roomID)
	if !exists {
		return spawnPositionForPlayerIndex(index)
	}

	switch roomID {
	case gamemap.RoomWardenHQ:
		return spawnPositionAtRoomFraction(room, 0.5, 0.5)
	case gamemap.RoomAmmoRoom:
		spawns := [][2]float32{
			{0.22, 0.22},
			{0.78, 0.22},
			{0.22, 0.78},
			{0.78, 0.78},
			{0.50, 0.38},
			{0.50, 0.62},
		}
		fraction := spawns[index%len(spawns)]
		return spawnPositionAtRoomFraction(room, fraction[0], fraction[1])
	default:
		return spawnPositionForPlayerIndex(index)
	}
}

func spawnPositionForPlayerIndex(index int) model.Vector2 {
	room, exists := defaultMatchLayout.Room(gamemap.RoomCellBlockA)
	if !exists {
		return model.Vector2{X: 2, Y: 13}
	}
	minX := room.Min.X
	minY := room.Min.Y
	maxX := room.Max.X
	maxY := room.Max.Y

	width := (maxX - minX) + 1
	if width <= 0 {
		return model.Vector2{X: float32(minX), Y: float32(minY)}
	}

	column := index % width
	row := index / width
	y := minY + row
	if y > maxY {
		y = maxY
	}

	return model.Vector2{
		X: float32(minX + column),
		Y: float32(y),
	}
}

func spawnPositionAtRoomFraction(room gamemap.Room, fractionX float32, fractionY float32) model.Vector2 {
	if fractionX < 0 {
		fractionX = 0
	}
	if fractionX > 1 {
		fractionX = 1
	}
	if fractionY < 0 {
		fractionY = 0
	}
	if fractionY > 1 {
		fractionY = 1
	}

	width := room.Max.X - room.Min.X
	height := room.Max.Y - room.Min.Y
	x := room.Min.X + int(float32(width)*fractionX+0.5)
	y := room.Min.Y + int(float32(height)*fractionY+0.5)
	if x < room.Min.X {
		x = room.Min.X
	}
	if x > room.Max.X {
		x = room.Max.X
	}
	if y < room.Min.Y {
		y = room.Min.Y
	}
	if y > room.Max.Y {
		y = room.Max.Y
	}

	return model.Vector2{X: float32(x), Y: float32(y)}
}

func consumeLifeAndRespawnIfAvailableLocked(player *model.PlayerState) (lifeLost bool, permanentlyEliminated bool) {
	if player == nil {
		return false, false
	}

	if player.LivesRemaining == 0 {
		player.LivesRemaining = combat.DefaultPlayerLives
	}
	if player.LivesRemaining > 0 {
		player.LivesRemaining--
	}
	if player.LivesRemaining == 0 {
		return true, true
	}

	player.Alive = true
	player.HeartsHalf = combat.MaxHeartsHalfForRole(player.Role)
	player.TempHeartsHalf = 0
	player.StunnedUntilTick = 0
	player.SolitaryUntilTick = 0
	player.LockedInCell = 0
	player.Velocity = model.Vector2{}
	player.Effects = nil

	roomID, position := respawnLocationForPlayer(*player)
	player.CurrentRoomID = roomID
	player.Position = position

	return true, false
}

func respawnLocationForPlayer(player model.PlayerState) (model.RoomID, model.Vector2) {
	switch player.Role {
	case model.RoleWarden:
		return gamemap.RoomWardenHQ, spawnPositionInRoom(gamemap.RoomWardenHQ, 0)
	case model.RoleDeputy:
		return gamemap.RoomAmmoRoom, spawnPositionInRoom(gamemap.RoomAmmoRoom, deterministicRespawnIndex(player.ID, 6))
	default:
		spawnIndex := 0
		if player.AssignedCell > 0 {
			spawnIndex = int(player.AssignedCell - 1)
		}
		return gamemap.RoomCellBlockA, spawnPositionForPlayerIndex(spawnIndex)
	}
}

func deterministicRespawnIndex(playerID model.PlayerID, modulo int) int {
	if modulo <= 1 || playerID == "" {
		return 0
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(playerID))
	return int(hasher.Sum64() % uint64(modulo))
}

func applyInputsToGameStateLocked(session *matchSession, commands []model.InputCommand) tickMutations {
	playerIndex := make(map[model.PlayerID]int, len(session.gameState.Players))
	for idx := range session.gameState.Players {
		playerIndex[session.gameState.Players[idx].ID] = idx
	}

	changed := make(map[model.PlayerID]struct{}, len(commands))
	changedDoors := make(map[model.DoorID]struct{})
	changedCells := make(map[model.CellID]struct{})
	changedEntities := make(map[model.EntityID]struct{})
	removedEntities := make(map[model.EntityID]struct{})
	resolveAmmoRestockCompletionsLocked(session, changed)
	expireProjectileEntitiesLocked(session, changedEntities, removedEntities)
	occupiedTiles := physics.BuildOccupiedTiles(session.gameState.Players)
	for _, command := range commands {
		idx, exists := playerIndex[command.PlayerID]
		if !exists {
			continue
		}

		if command.ClientSeq > session.lastProcessedClientSeq[command.PlayerID] {
			session.lastProcessedClientSeq[command.PlayerID] = command.ClientSeq
		}

		player := &session.gameState.Players[idx]
		if combat.IsActionBlocked(*player, session.tickID) &&
			command.Type != model.CmdAimIntent &&
			command.Type != model.CmdUseCard {
			continue
		}
		switch command.Type {
		case model.CmdMoveIntent:
			var payload model.MovementInputPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if isAmmoRestockInProgressLocked(session, player.ID) {
				if player.Velocity != (model.Vector2{}) {
					player.Velocity = model.Vector2{}
					changed[command.PlayerID] = struct{}{}
				}
				continue
			}
			if payload.MoveX != 0 || payload.MoveY != 0 {
				if session.lastMoveIntentTick == nil {
					session.lastMoveIntentTick = make(map[model.PlayerID]uint64)
				}
				session.lastMoveIntentTick[player.ID] = session.tickID
				if session.chameleonPendingUntil != nil {
					delete(session.chameleonPendingUntil, player.ID)
				}
				if removeEffectLocked(player, model.EffectChameleon) {
					changed[command.PlayerID] = struct{}{}
					if applyActionFeedbackLocked(
						player,
						model.ActionFeedbackKindSystem,
						model.ActionFeedbackLevelInfo,
						"Chameleon broken by movement.",
						session.tickID,
					) {
						changed[command.PlayerID] = struct{}{}
					}
				}
			}
			beforePosition := player.Position
			beforeVelocity := player.Velocity
			beforeRoomID := player.CurrentRoomID

			motion := physics.ResolveMoveIntent(
				*player,
				payload,
				defaultMatchLayout,
				session.gameState.Map,
				occupiedTiles,
				session.tickID,
			)
			player.Position = motion.Position
			player.Velocity = motion.Velocity
			if roomID, exists := defaultMatchLayout.RoomAt(physics.TileFromPosition(player.Position)); exists {
				player.CurrentRoomID = roomID
			}

			if player.Position != beforePosition || player.Velocity != beforeVelocity || player.CurrentRoomID != beforeRoomID {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdAimIntent:
			var payload model.AimInputPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			player.Facing = model.Vector2{
				X: payload.AimX,
				Y: payload.AimY,
			}
			changed[command.PlayerID] = struct{}{}

		case model.CmdReload:
			if applyReloadCommandLocked(session, player) {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdEquipItem:
			var payload model.EquipItemPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if applyEquipItemCommandLocked(session, player, payload) {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdFireWeapon:
			var payload model.FireWeaponPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if applyFireWeaponCommandLocked(
				session,
				command.PlayerID,
				payload,
				playerIndex,
				occupiedTiles,
				changed,
				changedEntities,
			) {
				// Mutations are tracked by helper.
			}

		case model.CmdInteract:
			var payload model.InteractPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if applyInteractCommandLocked(
				session,
				player,
				payload,
				changedDoors,
				changedCells,
				changedEntities,
				removedEntities,
			) {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdUseAbility:
			var payload model.AbilityUsePayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if applyAbilityCommandLocked(session, player, payload, playerIndex, changed, changedDoors) {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdUseCard:
			var payload model.CardUsePayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if applyCardCommandLocked(session, player, payload, playerIndex, changed, changedDoors, changedEntities) {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdUseItem:
			var payload model.ItemUsePayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if payload.TargetPlayerID == "" {
				continue
			}
			amount := payload.Amount
			if amount == 0 {
				amount = 1
			}
			if tryTransferItemBetweenPlayersLocked(
				session,
				playerIndex,
				command.PlayerID,
				payload.TargetPlayerID,
				payload.Item,
				amount,
			) {
				source := &session.gameState.Players[playerIndex[command.PlayerID]]
				target := &session.gameState.Players[playerIndex[payload.TargetPlayerID]]
				if ensureEquippedItemForPlayerLocked(source) {
					changed[command.PlayerID] = struct{}{}
				}
				if ensureEquippedItemForPlayerLocked(target) {
					changed[payload.TargetPlayerID] = struct{}{}
				}
				changed[command.PlayerID] = struct{}{}
				changed[payload.TargetPlayerID] = struct{}{}
			}

		case model.CmdBlackMarketBuy:
			var payload model.BlackMarketPurchasePayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			purchased, feedbackMessage, feedbackLevel := applyBlackMarketPurchaseLocked(session, player, payload)
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindPurchase,
				feedbackLevel,
				feedbackMessage,
				session.tickID,
			) {
				changed[command.PlayerID] = struct{}{}
			}
			if purchased {
				changed[command.PlayerID] = struct{}{}
			}

		case model.CmdDropItem:
			var payload model.DropItemPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			amount := payload.Amount
			if amount == 0 {
				amount = 1
			}
			if !items.RemoveItem(player, payload.Item, amount) {
				continue
			}

			droppedEntity := newDroppedItemEntityLocked(session, *player, payload.Item, amount)
			session.gameState.Entities = append(session.gameState.Entities, droppedEntity)
			if ensureEquippedItemForPlayerLocked(player) {
				changed[command.PlayerID] = struct{}{}
			}
			changed[command.PlayerID] = struct{}{}
			changedEntities[droppedEntity.ID] = struct{}{}

		case model.CmdCraftItem:
			var payload model.CraftItemPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if items.Craft(player, payload.Item) {
				if ensureEquippedItemForPlayerLocked(player) {
					changed[command.PlayerID] = struct{}{}
				}
				changed[command.PlayerID] = struct{}{}
			}

		default:
			// Shell tick pipeline: command accepted and acked, but no state mutation yet.
		}
	}

	for _, playerID := range roles.ApplyGangLeaderSuccession(&session.gameState, session.matchID, session.tickID) {
		changed[playerID] = struct{}{}
	}

	expireTimedStatesLocked(session, changed, changedDoors)
	applyAlarmAndGuardSystemLocked(
		session,
		playerIndex,
		changed,
		changedEntities,
		removedEntities,
	)

	return tickMutations{
		players:          collectChangedPlayers(session, playerIndex, changed),
		doors:            collectChangedDoors(session, changedDoors),
		cells:            collectChangedCells(session, changedCells),
		entities:         collectChangedEntities(session, changedEntities),
		removedEntityIDs: collectRemovedEntityIDs(removedEntities),
	}
}

func applyInteractCommandLocked(
	session *matchSession,
	player *model.PlayerState,
	payload model.InteractPayload,
	changedDoors map[model.DoorID]struct{},
	changedCells map[model.CellID]struct{},
	changedEntities map[model.EntityID]struct{},
	removedEntities map[model.EntityID]struct{},
) bool {
	changed := false

	if payload.TargetRoomID == "" &&
		payload.TargetCellID == 0 &&
		payload.TargetDoorID == 0 &&
		payload.TargetEntityID == 0 &&
		payload.EscapeRoute == "" &&
		payload.MarketRoomID == "" &&
		payload.NightCardChoice == "" &&
		payload.StashAction == "" &&
		payload.StashItem == "" {
		if tryTogglePowerIfEligibleLocked(session, player, changedDoors) {
			changed = true
			powerState := "OFF"
			if session.gameState.Map.PowerOn {
				powerState = "ON"
			}
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindDoor,
				model.ActionFeedbackLevelInfo,
				fmt.Sprintf("Power switched %s.", powerState),
				session.tickID,
			) {
				changed = true
			}
		}
	}

	if payload.NightCardChoice != "" {
		selected, reason := trySelectNightCardLocked(session, player, payload.NightCardChoice)
		level := model.ActionFeedbackLevelWarning
		if selected {
			level = model.ActionFeedbackLevelSuccess
		}
		if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			level,
			reason,
			session.tickID,
		) {
			changed = true
		}
		if selected {
			changed = true
		}
	}

	if payload.StashAction != "" && payload.StashItem != "" {
		applied, reason, level, changedCellID := applyCellStashInteractLocked(session, *player, payload)
		if changedCellID != 0 {
			changedCells[changedCellID] = struct{}{}
		}
		if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			level,
			reason,
			session.tickID,
		) {
			changed = true
		}
		if applied {
			changed = true
		}
	}

	if payload.EscapeRoute != "" {
		escaped, reason := tryEscapeRouteLocked(session, player, payload.EscapeRoute)
		if applyEscapeFeedbackLocked(player, payload.EscapeRoute, escaped, reason, session.tickID) {
			changed = true
		}
		level := model.ActionFeedbackLevelWarning
		if escaped {
			level = model.ActionFeedbackLevelSuccess
		}
		if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindEscape,
			level,
			fmt.Sprintf("%s: %s", payload.EscapeRoute, strings.TrimSpace(reason)),
			session.tickID,
		) {
			changed = true
		}
		if escaped {
			changed = true
		}
	}

	if payload.TargetRoomID != "" {
		if tryMovePlayerToRoomLocked(session, player, payload.TargetRoomID) {
			changed = true
		}
	}

	if payload.TargetCellID != 0 {
		if tryToggleCellDoorLocked(session, *player, payload.TargetCellID, changedDoors, changedCells) {
			changed = true
			doorID := model.DoorID(0)
			for _, cell := range session.gameState.Map.Cells {
				if cell.ID == payload.TargetCellID {
					doorID = cell.DoorID
					break
				}
			}
			message := "Cell door toggled."
			if doorID != 0 {
				if door, exists := doorStateForIDLocked(session.gameState.Map.Doors, doorID); exists {
					message = fmt.Sprintf("Cell door %d %s.", doorID, doorOpenLabel(door.Open))
				}
			}
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindDoor,
				model.ActionFeedbackLevelInfo,
				message,
				session.tickID,
			) {
				changed = true
			}
		} else if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindDoor,
			model.ActionFeedbackLevelWarning,
			"Cell door action denied.",
			session.tickID,
		) {
			changed = true
		}
	}

	if payload.TargetDoorID != 0 {
		if tryToggleDoorForPlayerLocked(session, *player, payload.TargetDoorID, changedDoors) {
			changed = true
			message := fmt.Sprintf("Door %d toggled.", payload.TargetDoorID)
			if door, exists := doorStateForIDLocked(session.gameState.Map.Doors, payload.TargetDoorID); exists {
				message = fmt.Sprintf("Door %d %s.", payload.TargetDoorID, doorOpenLabel(door.Open))
			}
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindDoor,
				model.ActionFeedbackLevelInfo,
				message,
				session.tickID,
			) {
				changed = true
			}
		} else if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindDoor,
			model.ActionFeedbackLevelWarning,
			fmt.Sprintf("Door %d action denied.", payload.TargetDoorID),
			session.tickID,
		) {
			changed = true
		}
	}
	if payload.TargetEntityID != 0 {
		if handledTask, taskChanged, feedback := tryHandleNPCPrisonerTaskInteractLocked(session, player, payload.TargetEntityID); handledTask {
			if taskChanged {
				changed = true
			}
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindSystem,
				feedback.Level,
				feedback.Message,
				session.tickID,
			) {
				changed = true
			}
		} else if tryPickupDroppedEntityLocked(session, player, payload.TargetEntityID, changedEntities, removedEntities) {
			changed = true
			if ensureEquippedItemForPlayerLocked(player) {
				changed = true
			}
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelSuccess,
				"Picked up dropped item.",
				session.tickID,
			) {
				changed = true
			}
		}
	}
	if payload.MarketRoomID != "" {
		if trySetNightlyBlackMarketLocked(session, *player, payload.MarketRoomID) {
			changed = true
			if applyActionFeedbackLocked(
				player,
				model.ActionFeedbackKindPurchase,
				model.ActionFeedbackLevelSuccess,
				fmt.Sprintf("Nightly market moved to %s.", payload.MarketRoomID),
				session.tickID,
			) {
				changed = true
			}
		} else if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindPurchase,
			model.ActionFeedbackLevelWarning,
			"Market move denied.",
			session.tickID,
		) {
			changed = true
		}
	}

	return changed
}

func tryEscapeRouteLocked(
	session *matchSession,
	player *model.PlayerState,
	route model.EscapeRouteType,
) (bool, string) {
	if session == nil || player == nil || route == "" {
		return false, "Escape attempt failed."
	}
	if player.CurrentRoomID == winconditions.EscapedRoomID {
		return false, "Already escaped."
	}
	evaluation := escape.EvaluateRoute(route, *player, session.gameState.Map)
	if !evaluation.CanAttempt {
		reason := evaluation.FailureReason
		if reason == "" {
			reason = "Escape requirements not met."
		}
		return false, reason
	}

	player.CurrentRoomID = winconditions.EscapedRoomID
	player.Velocity = model.Vector2{}
	return true, "Escape route successful."
}

type actionFeedbackSummary struct {
	Message string
	Level   model.ActionFeedbackLevel
}

func assignNightCardOffersForCycleLocked(session *matchSession, cycle uint8) []model.PlayerID {
	if session == nil || len(session.gameState.Players) == 0 {
		return nil
	}

	changed := make([]model.PlayerID, 0, len(session.gameState.Players))
	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if !player.Alive {
			if len(player.NightCardChoices) > 0 {
				player.NightCardChoices = nil
				changed = append(changed, player.ID)
			}
			continue
		}

		if len(player.Cards) >= cards.MaxCardsHeld {
			if len(player.NightCardChoices) > 0 {
				player.NightCardChoices = nil
				changed = append(changed, player.ID)
			}
			continue
		}

		next := deterministicNightCardChoices(session.matchID, player.ID, cycle, 3)
		if equalCardSlices(player.NightCardChoices, next) {
			continue
		}
		player.NightCardChoices = append([]model.CardType(nil), next...)
		changed = append(changed, player.ID)
	}

	return changed
}

func clearNightCardChoicesLocked(session *matchSession) []model.PlayerID {
	if session == nil || len(session.gameState.Players) == 0 {
		return nil
	}

	changed := make([]model.PlayerID, 0, len(session.gameState.Players))
	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if len(player.NightCardChoices) == 0 {
			continue
		}
		player.NightCardChoices = nil
		changed = append(changed, player.ID)
	}
	return changed
}

func deterministicNightCardChoices(
	matchID model.MatchID,
	playerID model.PlayerID,
	cycle uint8,
	choiceCount int,
) []model.CardType {
	if choiceCount <= 0 {
		return nil
	}

	catalog := cards.KnownCards()
	if len(catalog) == 0 {
		return nil
	}
	if choiceCount > len(catalog) {
		choiceCount = len(catalog)
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(playerID))
	_, _ = hasher.Write([]byte{byte(cycle)})
	seed := hasher.Sum64()

	picked := make(map[model.CardType]struct{}, choiceCount)
	out := make([]model.CardType, 0, choiceCount)
	for len(out) < choiceCount {
		index := int(seed % uint64(len(catalog)))
		candidate := catalog[index]
		seed = (seed * 1099511628211) ^ 1469598103934665603
		if _, exists := picked[candidate]; exists {
			continue
		}
		picked[candidate] = struct{}{}
		out = append(out, candidate)
	}

	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func equalCardSlices(left []model.CardType, right []model.CardType) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func trySelectNightCardLocked(
	session *matchSession,
	player *model.PlayerState,
	choice model.CardType,
) (bool, string) {
	if session == nil || player == nil || !cards.IsKnownCard(choice) {
		return false, "Night card selection failed."
	}
	if len(player.NightCardChoices) == 0 {
		return false, "No night card selection is pending."
	}

	allowed := false
	for _, candidate := range player.NightCardChoices {
		if candidate == choice {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, "Selected card is not in your night options."
	}
	if len(player.Cards) >= cards.MaxCardsHeld {
		return false, fmt.Sprintf("Card slots are full (%d/%d).", len(player.Cards), cards.MaxCardsHeld)
	}
	if !cards.AddCard(player, choice) {
		return false, "Failed to add selected card."
	}

	player.NightCardChoices = nil
	return true, fmt.Sprintf("Night card selected: %s.", choice)
}

func applyCellStashInteractLocked(
	session *matchSession,
	player model.PlayerState,
	payload model.InteractPayload,
) (bool, string, model.ActionFeedbackLevel, model.CellID) {
	if session == nil {
		return false, "Cell stash unavailable.", model.ActionFeedbackLevelWarning, 0
	}
	action := strings.ToLower(strings.TrimSpace(payload.StashAction))
	if action != "deposit" && action != "withdraw" {
		return false, "Unknown stash action.", model.ActionFeedbackLevelWarning, 0
	}
	if payload.StashItem == "" {
		return false, "Select an item for stash action.", model.ActionFeedbackLevelWarning, 0
	}
	if player.AssignedCell == 0 {
		return false, "No assigned cell stash.", model.ActionFeedbackLevelWarning, 0
	}
	if player.CurrentRoomID != gamemap.RoomCellBlockA {
		return false, "Go to your cell block to use stash.", model.ActionFeedbackLevelWarning, 0
	}

	cellIndex := -1
	for index := range session.gameState.Map.Cells {
		if session.gameState.Map.Cells[index].ID == player.AssignedCell {
			cellIndex = index
			break
		}
	}
	if cellIndex < 0 {
		return false, "Assigned cell was not found.", model.ActionFeedbackLevelWarning, 0
	}
	cell := &session.gameState.Map.Cells[cellIndex]
	if cell.OwnerPlayerID != player.ID {
		return false, "Only the cell owner can use this stash.", model.ActionFeedbackLevelWarning, 0
	}

	playerIndex := -1
	for index := range session.gameState.Players {
		if session.gameState.Players[index].ID == player.ID {
			playerIndex = index
			break
		}
	}
	if playerIndex < 0 {
		return false, "Player state unavailable.", model.ActionFeedbackLevelWarning, 0
	}
	livePlayer := &session.gameState.Players[playerIndex]

	amount := payload.StashAmount
	if amount == 0 {
		amount = 1
	}

	switch action {
	case "deposit":
		if !items.RemoveItem(livePlayer, payload.StashItem, amount) {
			return false, fmt.Sprintf("Not enough %s to stash.", payload.StashItem), model.ActionFeedbackLevelWarning, 0
		}
		stashCarrier := model.PlayerState{Inventory: append([]model.ItemStack(nil), cell.Stash...)}
		if !items.AddItem(&stashCarrier, payload.StashItem, amount) {
			_ = items.AddItem(livePlayer, payload.StashItem, amount)
			return false, "Cell stash is full.", model.ActionFeedbackLevelWarning, 0
		}
		cell.Stash = stashCarrier.Inventory
		_ = ensureEquippedItemForPlayerLocked(livePlayer)
		return true, fmt.Sprintf("Stashed %s x%d.", payload.StashItem, amount), model.ActionFeedbackLevelSuccess, cell.ID

	case "withdraw":
		stashCarrier := model.PlayerState{Inventory: append([]model.ItemStack(nil), cell.Stash...)}
		if !items.RemoveItem(&stashCarrier, payload.StashItem, amount) {
			return false, fmt.Sprintf("No %s in cell stash.", payload.StashItem), model.ActionFeedbackLevelWarning, 0
		}
		if !items.AddItem(livePlayer, payload.StashItem, amount) {
			_ = items.AddItem(&stashCarrier, payload.StashItem, amount)
			return false, "Inventory full; cannot withdraw.", model.ActionFeedbackLevelWarning, 0
		}
		cell.Stash = stashCarrier.Inventory
		_ = ensureEquippedItemForPlayerLocked(livePlayer)
		return true, fmt.Sprintf("Withdrew %s x%d.", payload.StashItem, amount), model.ActionFeedbackLevelSuccess, cell.ID
	}

	return false, "Stash action failed.", model.ActionFeedbackLevelWarning, 0
}

func tryHandleNPCPrisonerTaskInteractLocked(
	session *matchSession,
	player *model.PlayerState,
	entityID model.EntityID,
) (bool, bool, actionFeedbackSummary) {
	if session == nil || player == nil || entityID == 0 {
		return false, false, actionFeedbackSummary{}
	}
	if !player.Alive || !gamemap.IsPrisonerPlayer(*player) {
		return false, false, actionFeedbackSummary{}
	}

	entityIndex, exists := findEntityIndexByID(session.gameState.Entities, entityID)
	if !exists {
		return false, false, actionFeedbackSummary{}
	}
	entity := session.gameState.Entities[entityIndex]
	if !entity.Active || entity.Kind != model.EntityKindNPCPrisoner {
		return false, false, actionFeedbackSummary{}
	}
	if entity.RoomID == "" || player.CurrentRoomID == "" || entity.RoomID != player.CurrentRoomID {
		return false, false, actionFeedbackSummary{}
	}

	if session.gameState.Phase.Current != model.PhaseDay {
		return true, false, actionFeedbackSummary{
			Message: "NPC tasks can only be managed during day phase.",
			Level:   model.ActionFeedbackLevelWarning,
		}
	}
	dayStart := session.gameState.Phase.StartedTick
	if dayStart == 0 {
		return true, false, actionFeedbackSummary{
			Message: "Task board unavailable until day starts.",
			Level:   model.ActionFeedbackLevelWarning,
		}
	}
	if session.npcTaskByPlayer == nil {
		session.npcTaskByPlayer = make(map[model.PlayerID]npcTaskState)
	}

	task, exists := session.npcTaskByPlayer[player.ID]
	if !exists || task.DayStartTick != dayStart {
		task = deterministicNPCTaskForDay(session.matchID, player.ID, dayStart, entity.ID)
		task.DayStartTick = dayStart
		task.AssignedBy = entity.ID
		if task.RewardCards == 0 {
			task.RewardCards = 1
		}
		session.npcTaskByPlayer[player.ID] = task
		return true, false, actionFeedbackSummary{
			Message: "Task assigned: " + npcTaskDescription(task),
			Level:   model.ActionFeedbackLevelInfo,
		}
	}
	if task.Type == "" {
		return true, false, actionFeedbackSummary{
			Message: "No task assigned this day.",
			Level:   model.ActionFeedbackLevelInfo,
		}
	}
	if !isNPCTaskComplete(task, *player) {
		return true, false, actionFeedbackSummary{
			Message: "Task in progress: " + npcTaskProgress(task, *player),
			Level:   model.ActionFeedbackLevelInfo,
		}
	}

	added := uint8(0)
	for added < task.RewardCards {
		if !cards.AddCard(player, model.CardMoney) {
			break
		}
		added++
	}
	task.Type = ""
	task.TargetRoomID = ""
	task.TargetItem = ""
	task.RewardCards = 0
	session.npcTaskByPlayer[player.ID] = task
	if added == 0 {
		return true, false, actionFeedbackSummary{
			Message: "Task complete, but card slots are full. Spend cards and retry.",
			Level:   model.ActionFeedbackLevelWarning,
		}
	}
	return true, true, actionFeedbackSummary{
		Message: fmt.Sprintf("Task complete. Earned %d money card(s).", added),
		Level:   model.ActionFeedbackLevelSuccess,
	}
}

func deterministicNPCTaskForDay(
	matchID model.MatchID,
	playerID model.PlayerID,
	dayStartTick uint64,
	entityID model.EntityID,
) npcTaskState {
	visitRooms := append([]model.RoomID(nil), npcPrisonerRooms...)
	holdItems := []model.ItemType{
		model.ItemWood,
		model.ItemMetalSlab,
		model.ItemLockPick,
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(playerID))
	var tickBytes [8]byte
	for index := range tickBytes {
		tickBytes[index] = byte(dayStartTick >> (uint(index) * 8))
	}
	_, _ = hasher.Write(tickBytes[:])
	entityBytes := []byte{
		byte(entityID),
		byte(entityID >> 8),
		byte(entityID >> 16),
		byte(entityID >> 24),
	}
	_, _ = hasher.Write(entityBytes)
	seed := hasher.Sum64()

	task := npcTaskState{
		RewardCards: 1,
	}
	if seed%10 < 7 {
		task.Type = npcTaskVisitRoom
		task.TargetRoomID = visitRooms[int(seed%uint64(len(visitRooms)))]
		return task
	}

	task.Type = npcTaskHoldItem
	task.TargetItem = holdItems[int(seed%uint64(len(holdItems)))]
	return task
}

func npcTaskDescription(task npcTaskState) string {
	switch task.Type {
	case npcTaskVisitRoom:
		if task.TargetRoomID == "" {
			return "Visit the requested location."
		}
		return fmt.Sprintf("Go to %s, then interact with an NPC prisoner for reward.", roomLabelForFeedback(task.TargetRoomID))
	case npcTaskHoldItem:
		if task.TargetItem == "" {
			return "Bring a requested item."
		}
		return fmt.Sprintf("Bring %s in inventory, then interact with an NPC prisoner.", task.TargetItem)
	default:
		return "No active task."
	}
}

func npcTaskProgress(task npcTaskState, player model.PlayerState) string {
	switch task.Type {
	case npcTaskVisitRoom:
		if task.TargetRoomID == "" {
			return "Visit the assigned room."
		}
		if player.CurrentRoomID == task.TargetRoomID {
			return "Return to an NPC prisoner to claim reward."
		}
		return fmt.Sprintf("Go to %s.", roomLabelForFeedback(task.TargetRoomID))
	case npcTaskHoldItem:
		if task.TargetItem == "" {
			return "Hold the requested item."
		}
		if items.HasItem(player, task.TargetItem, 1) {
			return "Return to an NPC prisoner to claim reward."
		}
		return fmt.Sprintf("Carry %s x1 in inventory.", task.TargetItem)
	default:
		return "No active task."
	}
}

func isNPCTaskComplete(task npcTaskState, player model.PlayerState) bool {
	switch task.Type {
	case npcTaskVisitRoom:
		return task.TargetRoomID != "" && player.CurrentRoomID == task.TargetRoomID
	case npcTaskHoldItem:
		return task.TargetItem != "" && items.HasItem(player, task.TargetItem, 1)
	default:
		return false
	}
}

func roomLabelForFeedback(roomID model.RoomID) string {
	switch roomID {
	case gamemap.RoomCorridorMain:
		return "Main Corridor"
	case gamemap.RoomCellBlockA:
		return "Cell Block A"
	case gamemap.RoomWardenHQ:
		return "Warden HQ"
	case gamemap.RoomCameraRoom:
		return "Camera Room"
	case gamemap.RoomPowerRoom:
		return "Power Room"
	case gamemap.RoomAmmoRoom:
		return "Ammo Room"
	case gamemap.RoomBlackMarket:
		return "Black Market"
	case gamemap.RoomCafeteria:
		return "Cafeteria"
	case gamemap.RoomMailRoom:
		return "Mail Room"
	case gamemap.RoomCourtyard:
		return "Courtyard"
	case gamemap.RoomRoofLookout:
		return "Roof Lookout"
	default:
		return strings.ReplaceAll(string(roomID), "_", " ")
	}
}

func applyActionFeedbackLocked(
	player *model.PlayerState,
	kind model.ActionFeedbackKind,
	level model.ActionFeedbackLevel,
	message string,
	tickID uint64,
) bool {
	if player == nil || kind == "" {
		return false
	}

	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		return false
	}

	if level == "" {
		level = model.ActionFeedbackLevelInfo
	}

	next := model.ActionFeedback{
		Kind:    kind,
		Level:   level,
		Message: trimmedMessage,
		TickID:  tickID,
	}
	if player.LastActionFeedback == next {
		return false
	}
	player.LastActionFeedback = next
	return true
}

func applyEscapeFeedbackLocked(
	player *model.PlayerState,
	route model.EscapeRouteType,
	success bool,
	reason string,
	tickID uint64,
) bool {
	if player == nil || route == "" {
		return false
	}

	status := model.EscapeAttemptStatusFailed
	if success {
		status = model.EscapeAttemptStatusSuccess
	}
	next := model.EscapeAttemptFeedback{
		Route:  route,
		Status: status,
		Reason: strings.TrimSpace(reason),
		TickID: tickID,
	}
	if player.LastEscapeAttempt == next {
		return false
	}
	player.LastEscapeAttempt = next
	return true
}

func doorStateForIDLocked(doors []model.DoorState, doorID model.DoorID) (model.DoorState, bool) {
	for _, door := range doors {
		if door.ID == doorID {
			return door, true
		}
	}
	return model.DoorState{}, false
}

func doorOpenLabel(open bool) string {
	if open {
		return "opened"
	}
	return "closed"
}

func formatHalfHearts(halfHearts uint8) string {
	whole := halfHearts / 2
	if halfHearts%2 == 0 {
		return fmt.Sprintf("%d", whole)
	}
	return fmt.Sprintf("%d.5", whole)
}

func isAmmoRestockInProgressLocked(session *matchSession, playerID model.PlayerID) bool {
	if session == nil || playerID == "" {
		return false
	}
	until := session.ammoRestockUntil[playerID]
	if until == 0 {
		return false
	}
	return session.tickID < until
}

func applyReloadCommandLocked(session *matchSession, player *model.PlayerState) bool {
	if session == nil || player == nil {
		return false
	}

	if !player.Alive {
		return false
	}
	if !gamemap.IsAuthorityPlayer(*player) {
		return applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelWarning,
			"Only guards and the warden can restock ammo.",
			session.tickID,
		)
	}
	if player.CurrentRoomID != gamemap.RoomAmmoRoom {
		return applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelWarning,
			"Go to the ammo room to restock.",
			session.tickID,
		)
	}
	if player.Bullets >= authorityAmmoMax {
		return applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelInfo,
			"Ammo already full.",
			session.tickID,
		)
	}
	if isAmmoRestockInProgressLocked(session, player.ID) {
		remaining := session.ammoRestockUntil[player.ID] - session.tickID
		rate := uint64(maxTickRate(session.tickRateHz))
		seconds := int((remaining + rate - 1) / rate)
		return applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelInfo,
			fmt.Sprintf("Ammo restock in progress (%ds left).", seconds),
			session.tickID,
		)
	}

	durationTicks := uint64(maxTickRate(session.tickRateHz)) * authorityRestockSeconds
	if durationTicks == 0 {
		durationTicks = authorityRestockSeconds
	}
	if session.ammoRestockUntil == nil {
		session.ammoRestockUntil = make(map[model.PlayerID]uint64)
	}
	session.ammoRestockUntil[player.ID] = session.tickID + durationTicks
	player.Velocity = model.Vector2{}

	return applyActionFeedbackLocked(
		player,
		model.ActionFeedbackKindSystem,
		model.ActionFeedbackLevelInfo,
		fmt.Sprintf("Restocking ammo (10s). Hold position in %s.", roomLabelForFeedback(gamemap.RoomAmmoRoom)),
		session.tickID,
	)
}

func resolveAmmoRestockCompletionsLocked(session *matchSession, changedPlayers map[model.PlayerID]struct{}) {
	if session == nil || len(session.ammoRestockUntil) == 0 {
		return
	}

	playerIndex := make(map[model.PlayerID]int, len(session.gameState.Players))
	for index := range session.gameState.Players {
		playerIndex[session.gameState.Players[index].ID] = index
	}

	for playerID, untilTick := range session.ammoRestockUntil {
		if untilTick == 0 || session.tickID < untilTick {
			continue
		}
		delete(session.ammoRestockUntil, playerID)

		index, exists := playerIndex[playerID]
		if !exists {
			continue
		}
		player := &session.gameState.Players[index]
		if !player.Alive || !gamemap.IsAuthorityPlayer(*player) {
			continue
		}

		if player.Bullets != authorityAmmoMax {
			player.Bullets = authorityAmmoMax
			changedPlayers[player.ID] = struct{}{}
		}
		if syncAuthorityBulletStackLocked(player) {
			changedPlayers[player.ID] = struct{}{}
		}
		if applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelSuccess,
			"Ammo restocked to full.",
			session.tickID,
		) {
			changedPlayers[player.ID] = struct{}{}
		}
	}
}

func restoreAuthoritySidearmsForNewDayLocked(session *matchSession) []model.PlayerID {
	if session == nil || len(session.gameState.Players) == 0 {
		return nil
	}

	changed := make([]model.PlayerID, 0, len(session.gameState.Players))
	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if !gamemap.IsAuthorityPlayer(*player) {
			continue
		}
		player.InventorySlots = playerInventorySlotCount
		normalizeInventoryToPlayerSlotsLocked(player)

		updated := false
		if !items.HasItem(*player, model.ItemBaton, 1) {
			updated = items.AddItem(player, model.ItemBaton, 1) || updated
		}
		if !items.HasItem(*player, model.ItemPistol, 1) {
			if !items.AddItem(player, model.ItemPistol, 1) {
				if evictAuthorityOverflowItemForSidearmLocked(player) {
					updated = items.AddItem(player, model.ItemPistol, 1) || updated
				}
			} else {
				updated = true
			}
		}
		if syncAuthorityBulletStackLocked(player) {
			updated = true
		}
		if ensureEquippedItemForPlayerLocked(player) {
			updated = true
		}
		if updated {
			changed = append(changed, player.ID)
		}
	}
	return changed
}

func evictAuthorityOverflowItemForSidearmLocked(player *model.PlayerState) bool {
	if player == nil || len(player.Inventory) == 0 {
		return false
	}

	for _, stack := range player.Inventory {
		if stack.Item == model.ItemBaton || stack.Item == model.ItemPistol || stack.Item == model.ItemBullet {
			continue
		}
		if items.RemoveItem(player, stack.Item, stack.Quantity) {
			return true
		}
	}

	for _, stack := range player.Inventory {
		if stack.Item == model.ItemBullet && stack.Quantity > 0 {
			return items.RemoveItem(player, stack.Item, stack.Quantity)
		}
	}
	return false
}

func applyEquipItemCommandLocked(
	session *matchSession,
	player *model.PlayerState,
	payload model.EquipItemPayload,
) bool {
	if session == nil || player == nil || !combat.IsSupportedWeapon(payload.Item) {
		return false
	}

	if !combat.CanUseWeapon(*player, payload.Item) {
		return applyActionFeedbackLocked(
			player,
			model.ActionFeedbackKindSystem,
			model.ActionFeedbackLevelWarning,
			fmt.Sprintf("Cannot equip %s; not in inventory.", payload.Item),
			session.tickID,
		)
	}
	if player.EquippedItem == payload.Item {
		return false
	}

	player.EquippedItem = payload.Item
	changed := true
	if applyActionFeedbackLocked(
		player,
		model.ActionFeedbackKindSystem,
		model.ActionFeedbackLevelInfo,
		fmt.Sprintf("Equipped %s.", payload.Item),
		session.tickID,
	) {
		changed = true
	}
	return changed
}

func expireProjectileEntitiesLocked(
	session *matchSession,
	changedEntities map[model.EntityID]struct{},
	removedEntities map[model.EntityID]struct{},
) {
	if session == nil || len(session.projectileExpireTick) == 0 {
		return
	}

	keep := make([]model.EntityState, 0, len(session.gameState.Entities))
	for _, entity := range session.gameState.Entities {
		expireTick := session.projectileExpireTick[entity.ID]
		if entity.Kind == model.EntityKindProjectile && expireTick != 0 && session.tickID >= expireTick {
			delete(session.projectileExpireTick, entity.ID)
			delete(changedEntities, entity.ID)
			removedEntities[entity.ID] = struct{}{}
			continue
		}
		if entity.Kind == model.EntityKindProjectile {
			entity.Position.X += entity.Velocity.X
			entity.Position.Y += entity.Velocity.Y
			changedEntities[entity.ID] = struct{}{}
		}
		keep = append(keep, entity)
	}
	session.gameState.Entities = keep
}

func spawnProjectileEntityLocked(
	session *matchSession,
	shooter model.PlayerState,
	targetX float32,
	targetY float32,
	changedEntities map[model.EntityID]struct{},
) {
	if session == nil {
		return
	}

	aimX := targetX - shooter.Position.X
	aimY := targetY - shooter.Position.Y
	length := float32((aimX * aimX) + (aimY * aimY))
	if length <= 0 {
		aimX = shooter.Facing.X
		aimY = shooter.Facing.Y
		length = float32((aimX * aimX) + (aimY * aimY))
	}
	if length <= 0 {
		aimX = 1
		aimY = 0
		length = 1
	}
	magnitude := float32(1.0)
	if length > 1 {
		magnitude = float32(1.0 / float32(math.Sqrt(float64(length))))
	}
	unitX := aimX * magnitude
	unitY := aimY * magnitude

	session.nextEntityID++
	entity := model.EntityState{
		ID:            session.nextEntityID,
		Kind:          model.EntityKindProjectile,
		OwnerPlayerID: shooter.ID,
		Position: model.Vector2{
			X: shooter.Position.X + (unitX * 0.4),
			Y: shooter.Position.Y + (unitY * 0.4),
		},
		Velocity: model.Vector2{
			X: unitX * 1.8,
			Y: unitY * 1.8,
		},
		Active: true,
		RoomID: shooter.CurrentRoomID,
	}

	if session.projectileExpireTick == nil {
		session.projectileExpireTick = make(map[model.EntityID]uint64)
	}
	lifetimeTicks := uint64(maxTickRate(session.tickRateHz) / 5)
	if lifetimeTicks == 0 {
		lifetimeTicks = 1
	}
	session.projectileExpireTick[entity.ID] = session.tickID + lifetimeTicks
	session.gameState.Entities = append(session.gameState.Entities, entity)
	changedEntities[entity.ID] = struct{}{}
}

func maxTickRate(rate uint32) uint32 {
	if rate == 0 {
		return 1
	}
	return rate
}

func trySetNightlyBlackMarketLocked(
	session *matchSession,
	player model.PlayerState,
	targetRoomID model.RoomID,
) bool {
	if session == nil ||
		targetRoomID == "" ||
		!player.Alive ||
		player.Role != model.RoleGangLeader ||
		session.gameState.Phase.Current != model.PhaseNight {
		return false
	}
	if !gamemap.IsNightlyBlackMarketCandidate(targetRoomID) {
		return false
	}
	if session.gameState.Map.BlackMarketRoomID == targetRoomID {
		return false
	}

	session.gameState.Map.BlackMarketRoomID = targetRoomID
	return true
}

func applyBlackMarketPurchaseLocked(
	session *matchSession,
	player *model.PlayerState,
	payload model.BlackMarketPurchasePayload,
) (bool, string, model.ActionFeedbackLevel) {
	if session == nil || player == nil {
		return false, "Purchase unavailable.", model.ActionFeedbackLevelError
	}

	offer, exists := items.BlackMarketOfferForItem(payload.Item)
	if !exists || offer.Quantity == 0 || offer.MoneyCardCost == 0 {
		return false, "Offer unavailable.", model.ActionFeedbackLevelError
	}
	if !player.Alive || !gamemap.IsPrisonerPlayer(*player) {
		return false, "Only living prisoners can buy.", model.ActionFeedbackLevelWarning
	}
	if session.gameState.Phase.Current != model.PhaseNight {
		return false, "Market opens at night.", model.ActionFeedbackLevelWarning
	}
	marketRoomID := session.gameState.Map.BlackMarketRoomID
	if marketRoomID == "" || player.CurrentRoomID == "" || player.CurrentRoomID != marketRoomID {
		if marketRoomID == "" {
			return false, "Market room is not set.", model.ActionFeedbackLevelWarning
		}
		return false, fmt.Sprintf("Go to %s to buy.", marketRoomID), model.ActionFeedbackLevelWarning
	}
	if offer.Item == model.ItemGoldenBullet &&
		(session.blackMarketGoldenBulletSold || matchContainsGoldenBulletLocked(session)) {
		return false, "Golden bullet already sold this match.", model.ActionFeedbackLevelWarning
	}

	if countPlayerCards(*player, model.CardMoney) < int(offer.MoneyCardCost) {
		return false, fmt.Sprintf("Need %d money cards.", offer.MoneyCardCost), model.ActionFeedbackLevelWarning
	}
	if !items.AddItem(player, offer.Item, offer.Quantity) {
		return false, "Inventory full; purchase cancelled.", model.ActionFeedbackLevelWarning
	}
	if !consumePlayerCards(player, model.CardMoney, offer.MoneyCardCost) {
		_ = items.RemoveItem(player, offer.Item, offer.Quantity)
		return false, "Payment failed; purchase cancelled.", model.ActionFeedbackLevelError
	}
	_ = ensureEquippedItemForPlayerLocked(player)

	if offer.Item == model.ItemGoldenBullet {
		session.blackMarketGoldenBulletSold = true
	}
	return true, fmt.Sprintf("Purchased %s x%d for %d money card(s).", offer.Item, offer.Quantity, offer.MoneyCardCost), model.ActionFeedbackLevelSuccess
}

func matchContainsGoldenBulletLocked(session *matchSession) bool {
	if session == nil {
		return false
	}

	for _, player := range session.gameState.Players {
		if items.HasItem(player, model.ItemGoldenBullet, 1) {
			return true
		}
	}
	for _, entity := range session.gameState.Entities {
		item, quantity, ok := items.ParseDroppedItem(entity)
		if !ok || quantity == 0 {
			continue
		}
		if item == model.ItemGoldenBullet {
			return true
		}
	}
	return false
}

func countPlayerCards(player model.PlayerState, target model.CardType) int {
	if target == "" || len(player.Cards) == 0 {
		return 0
	}

	count := 0
	for _, card := range player.Cards {
		if card == target {
			count++
		}
	}
	return count
}

func consumePlayerCards(player *model.PlayerState, target model.CardType, quantity uint8) bool {
	if player == nil || target == "" {
		return false
	}
	if quantity == 0 {
		return true
	}

	removed := uint8(0)
	for removed < quantity {
		if !cards.RemoveCard(player, target) {
			for removed > 0 {
				_ = cards.AddCard(player, target)
				removed--
			}
			return false
		}
		removed++
	}
	return true
}

func tryMovePlayerToRoomLocked(
	session *matchSession,
	player *model.PlayerState,
	targetRoomID model.RoomID,
) bool {
	fromRoomID := player.CurrentRoomID
	if fromRoomID == "" {
		fromRoomID = gamemap.RoomCellBlockA
	}

	access, err := defaultMatchLayout.CheckRoomAccess(fromRoomID, targetRoomID)
	if err != nil || !access.Reachable {
		return false
	}
	if !canPlayerEnterRoomLocked(session, *player, targetRoomID, session.gameState.Map) {
		return false
	}
	if player.CurrentRoomID == targetRoomID {
		return false
	}

	player.CurrentRoomID = targetRoomID
	return true
}

func tryToggleCellDoorLocked(
	session *matchSession,
	player model.PlayerState,
	cellID model.CellID,
	changedDoors map[model.DoorID]struct{},
	changedCells map[model.CellID]struct{},
) bool {
	cellIndex := -1
	for idx := range session.gameState.Map.Cells {
		if session.gameState.Map.Cells[idx].ID == cellID {
			cellIndex = idx
			break
		}
	}
	if cellIndex < 0 {
		return false
	}

	cell := session.gameState.Map.Cells[cellIndex]
	if !gamemap.CanOperateCellDoor(player, cell) {
		return false
	}
	doorChanged := tryToggleDoorForPlayerLocked(session, player, cell.DoorID, changedDoors)
	if doorChanged {
		changedCells[cell.ID] = struct{}{}
	}
	return doorChanged
}

func tryToggleDoorForPlayerLocked(
	session *matchSession,
	player model.PlayerState,
	doorID model.DoorID,
	changedDoors map[model.DoorID]struct{},
) bool {
	for idx := range session.gameState.Map.Cells {
		cell := session.gameState.Map.Cells[idx]
		if cell.DoorID == doorID && !gamemap.CanOperateCellDoor(player, cell) {
			return false
		}
	}

	doorIndex := -1
	for idx := range session.gameState.Map.Doors {
		if session.gameState.Map.Doors[idx].ID == doorID {
			doorIndex = idx
			break
		}
	}
	if doorIndex < 0 {
		return false
	}

	door := &session.gameState.Map.Doors[doorIndex]
	if door.BlockedUntilTick != 0 && session.tickID <= door.BlockedUntilTick {
		return false
	}
	if !session.gameState.Map.PowerOn {
		if !door.Open {
			door.Open = true
			changedDoors[door.ID] = struct{}{}
			return true
		}
		return false
	}

	door.Open = !door.Open
	changedDoors[door.ID] = struct{}{}
	return true
}

func tryTogglePowerIfEligibleLocked(
	session *matchSession,
	player *model.PlayerState,
	changedDoors map[model.DoorID]struct{},
) bool {
	if session == nil || player == nil || player.CurrentRoomID != gamemap.RoomPowerRoom {
		return false
	}

	nextPower := !session.gameState.Map.PowerOn
	if !prison.ApplyPowerState(&session.gameState.Map, nextPower) {
		return false
	}

	for _, door := range session.gameState.Map.Doors {
		changedDoors[door.ID] = struct{}{}
	}

	return true
}

func applyAbilityCommandLocked(
	session *matchSession,
	player *model.PlayerState,
	payload model.AbilityUsePayload,
	playerIndex map[model.PlayerID]int,
	changedPlayers map[model.PlayerID]struct{},
	changedDoors map[model.DoorID]struct{},
) bool {
	if session == nil || player == nil {
		return false
	}

	if !abilities.IsKnownAbility(payload.Ability) {
		return denyAbilityUseLocked(session, player, changedPlayers, "unknown ability")
	}
	if player.AssignedAbility != "" && payload.Ability != player.AssignedAbility {
		return denyAbilityUseLocked(
			session,
			player,
			changedPlayers,
			fmt.Sprintf("you are assigned %s this match", player.AssignedAbility),
		)
	}
	if !abilities.CanPlayerUse(*player, payload.Ability) {
		return denyAbilityUseLocked(session, player, changedPlayers, "your role cannot use this ability")
	}
	if allowed, reason := canUseAbilityAtCurrentTick(session, *player, payload.Ability); !allowed {
		return denyAbilityUseLocked(session, player, changedPlayers, reason)
	}

	payload = resolveAbilityTargetsForContextLocked(session, *player, payload)

	applied := false
	feedbackKind := model.ActionFeedbackKindSystem
	feedbackLevel := model.ActionFeedbackLevelInfo
	feedbackMessage := ""
	switch payload.Ability {
	case model.AbilityAlarm:
		durationTicks := prison.AlarmDurationTicks(session.tickRateHz)
		if durationTicks == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "alarm duration is unavailable")
		}
		session.gameState.Map.Alarm = model.AlarmState{
			Active:      true,
			EndsTick:    session.tickID + durationTicks,
			TriggeredBy: player.ID,
		}
		applied = true
		feedbackKind = model.ActionFeedbackKindAlarm
		feedbackLevel = model.ActionFeedbackLevelSuccess
		feedbackMessage = fmt.Sprintf("Alarm triggered (%dt).", durationTicks)

	case model.AbilitySearch:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" {
			return denyAbilityUseLocked(session, player, changedPlayers, "stand in the same room as a living target to search")
		}
		if targetID == player.ID {
			return denyAbilityUseLocked(session, player, changedPlayers, "you cannot search yourself")
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive {
			return denyAbilityUseLocked(session, player, changedPlayers, "target is already down")
		}
		if target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return denyAbilityUseLocked(session, player, changedPlayers, "search requires target in your current room")
		}

		inventorySummary := summarizeInventoryForFeedback(target.Inventory)
		confiscated := make([]string, 0, 4)
		for _, contraband := range items.ContrabandStacks(*target) {
			if items.RemoveItem(target, contraband.Item, contraband.Quantity) {
				confiscated = append(confiscated, fmt.Sprintf("%s x%d", contraband.Item, contraband.Quantity))
			}
		}
		applied = true
		if len(confiscated) > 0 {
			if ensureEquippedItemForPlayerLocked(target) {
				changedPlayers[target.ID] = struct{}{}
			}
			changedPlayers[target.ID] = struct{}{}
			feedbackMessage = fmt.Sprintf(
				"Search report on %s: inv=%s | confiscated=%s.",
				target.Name,
				inventorySummary,
				strings.Join(confiscated, ", "),
			)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelWarning,
				fmt.Sprintf("Searched by %s; contraband confiscated.", player.Name),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		} else {
			feedbackMessage = fmt.Sprintf("Search report on %s: inv=%s.", target.Name, inventorySummary)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelInfo,
				fmt.Sprintf("Searched by %s.", player.Name),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		}

	case model.AbilityCameraMan:
		if player.CurrentRoomID != gamemap.RoomCameraRoom {
			return denyAbilityUseLocked(session, player, changedPlayers, "go to the camera room")
		}
		if !session.gameState.Map.PowerOn {
			return denyAbilityUseLocked(session, player, changedPlayers, "restore power before using cameras")
		}
		duration := abilities.EffectDurationTicks(model.AbilityCameraMan, session.tickRateHz)
		if duration == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "camera system cooldown is unavailable")
		}
		if upsertEffectLocked(player, model.EffectCameraView, session.tickID+duration, player.ID, 0, 1) {
			applied = true
			feedbackMessage = fmt.Sprintf("Camera feed active for %dt.", duration)
		} else {
			return denyAbilityUseLocked(session, player, changedPlayers, "camera feed is already active")
		}

	case model.AbilityDetainer:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" {
			return denyAbilityUseLocked(session, player, changedPlayers, "stand in the same room as a living target to detain")
		}
		if targetID == player.ID {
			return denyAbilityUseLocked(session, player, changedPlayers, "you cannot detain yourself")
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive {
			return denyAbilityUseLocked(session, player, changedPlayers, "target is already down")
		}
		if target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return denyAbilityUseLocked(session, player, changedPlayers, "detainer requires target in your current room")
		}

		duration := abilities.EffectDurationTicks(model.AbilityDetainer, session.tickRateHz)
		if duration == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "detainer duration is unavailable")
		}
		untilTick := session.tickID + duration
		changed := false
		if untilTick > target.SolitaryUntilTick {
			target.SolitaryUntilTick = untilTick
			changed = true
		}
		if target.AssignedCell != 0 && target.LockedInCell != target.AssignedCell {
			target.LockedInCell = target.AssignedCell
			changed = true
		}
		if changed {
			changedPlayers[target.ID] = struct{}{}
			applied = true
			feedbackKind = model.ActionFeedbackKindStun
			feedbackMessage = fmt.Sprintf("Detainer applied to %s.", target.Name)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindStun,
				model.ActionFeedbackLevelWarning,
				fmt.Sprintf("Detained by %s until tick %d.", player.Name, untilTick),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		}
		if !changed {
			return denyAbilityUseLocked(session, player, changedPlayers, "target is already fully detained")
		}

	case model.AbilityTracker:
		duration := abilities.EffectDurationTicks(model.AbilityTracker, session.tickRateHz)
		if duration == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "tracker duration is unavailable")
		}
		if upsertEffectLocked(player, model.EffectTrackerView, session.tickID+duration, player.ID, 0, 1) {
			applied = true
			feedbackMessage = fmt.Sprintf("Tracker view active for %dt.", duration)
		} else {
			return denyAbilityUseLocked(session, player, changedPlayers, "tracker view is already active")
		}

	case model.AbilityPickPocket:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" {
			return denyAbilityUseLocked(session, player, changedPlayers, "stand in the same room as a living target to pick-pocket")
		}
		if targetID == player.ID {
			return denyAbilityUseLocked(session, player, changedPlayers, "you cannot pick-pocket yourself")
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive {
			return denyAbilityUseLocked(session, player, changedPlayers, "target is already down")
		}
		if target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return denyAbilityUseLocked(session, player, changedPlayers, "pick-pocket requires target in your current room")
		}
		item := cards.DeterministicGrabFromInventory(player.ID, target.ID, session.tickID, target.Inventory)
		if item == "" {
			return denyAbilityUseLocked(session, player, changedPlayers, "target has no stealable items")
		}
		if items.TransferItem(target, player, item, 1) {
			if ensureEquippedItemForPlayerLocked(target) {
				changedPlayers[target.ID] = struct{}{}
			}
			if ensureEquippedItemForPlayerLocked(player) {
				changedPlayers[player.ID] = struct{}{}
			}
			changedPlayers[target.ID] = struct{}{}
			applied = true
			feedbackMessage = fmt.Sprintf("Pick-pocket stole %s from %s.", item, target.Name)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelWarning,
				fmt.Sprintf("An item was stolen by %s.", player.Name),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		}
		if !applied {
			return denyAbilityUseLocked(session, player, changedPlayers, "pick-pocket failed")
		}

	case model.AbilityHacker:
		if !session.gameState.Map.PowerOn {
			return denyAbilityUseLocked(session, player, changedPlayers, "power is already off")
		}
		if !prison.ApplyPowerState(&session.gameState.Map, false) {
			return denyAbilityUseLocked(session, player, changedPlayers, "power controls are unavailable")
		}
		for _, door := range session.gameState.Map.Doors {
			changedDoors[door.ID] = struct{}{}
		}
		applied = true
		feedbackKind = model.ActionFeedbackKindDoor
		feedbackMessage = "Hacker cut power OFF."

	case model.AbilityDisguise:
		duration := abilities.EffectDurationTicks(model.AbilityDisguise, session.tickRateHz)
		if duration == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "disguise duration is unavailable")
		}
		applied = upsertEffectLocked(player, model.EffectDisguised, session.tickID+duration, player.ID, 0, 1)
		if applied {
			feedbackMessage = fmt.Sprintf("Disguise active for %dt.", duration)
		} else {
			return denyAbilityUseLocked(session, player, changedPlayers, "disguise is already active")
		}

	case model.AbilityLocksmith:
		if !session.gameState.Map.PowerOn {
			return denyAbilityUseLocked(session, player, changedPlayers, "power must be on to use locksmith")
		}
		if payload.TargetDoorID == 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "stand by an accessible door")
		}
		doorIndex := -1
		for index := range session.gameState.Map.Doors {
			if session.gameState.Map.Doors[index].ID == payload.TargetDoorID {
				doorIndex = index
				break
			}
		}
		if doorIndex < 0 {
			return denyAbilityUseLocked(session, player, changedPlayers, "target door no longer exists")
		}
		door := &session.gameState.Map.Doors[doorIndex]
		changed := false
		if door.Locked {
			door.Locked = false
			changed = true
		}
		if !door.Open {
			door.Open = true
			changed = true
		}
		unlockedRoomID, unlockedRoom := unlockRoomForPrisonersFromDoorLocked(session, *door)
		if changed || unlockedRoom {
			if changed {
				changedDoors[door.ID] = struct{}{}
			}
			applied = true
			feedbackKind = model.ActionFeedbackKindDoor
			if unlockedRoom {
				roomLabel := "restricted room"
				switch unlockedRoomID {
				case gamemap.RoomPowerRoom:
					roomLabel = "Power Room"
				case gamemap.RoomAmmoRoom:
					roomLabel = "Ammunition Room"
				}
				feedbackMessage = fmt.Sprintf("Locksmith unlocked %s access for prisoners.", roomLabel)
			} else {
				feedbackMessage = fmt.Sprintf("Locksmith opened door %d.", door.ID)
			}
		} else {
			return denyAbilityUseLocked(session, player, changedPlayers, "target door is already unlocked and open")
		}

	case model.AbilityChameleon:
		if hasActiveEffect(*player, model.EffectChameleon, session.tickID) {
			return denyAbilityUseLocked(session, player, changedPlayers, "you are already hidden; move to reveal")
		}
		if session.chameleonPendingUntil == nil {
			session.chameleonPendingUntil = make(map[model.PlayerID]uint64)
		}
		if pending := session.chameleonPendingUntil[player.ID]; pending != 0 && session.tickID < pending {
			return denyAbilityUseLocked(session, player, changedPlayers, "chameleon is already primed")
		}
		pendingTicks := uint64(session.tickRateHz) * 5
		if pendingTicks == 0 {
			pendingTicks = 5
		}
		session.chameleonPendingUntil[player.ID] = session.tickID + pendingTicks
		applied = true
		feedbackMessage = "Stay still for 5s to trigger chameleon invisibility."

	default:
		return denyAbilityUseLocked(session, player, changedPlayers, "ability is not implemented")
	}

	if !applied {
		return denyAbilityUseLocked(session, player, changedPlayers, "ability had no effect")
	}
	registerAbilityUseLocked(session, *player, payload.Ability)
	if applyActionFeedbackLocked(player, feedbackKind, feedbackLevel, feedbackMessage, session.tickID) {
		changedPlayers[player.ID] = struct{}{}
	}
	return true
}

func denyAbilityUseLocked(
	session *matchSession,
	player *model.PlayerState,
	changedPlayers map[model.PlayerID]struct{},
	reason string,
) bool {
	if session == nil || player == nil {
		return false
	}

	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		trimmed = "that action is unavailable right now"
	}
	message := fmt.Sprintf("can't use that here: %s", trimmed)
	if !strings.HasSuffix(message, ".") {
		message += "."
	}
	if applyActionFeedbackLocked(
		player,
		model.ActionFeedbackKindSystem,
		model.ActionFeedbackLevelWarning,
		message,
		session.tickID,
	) {
		if changedPlayers != nil {
			changedPlayers[player.ID] = struct{}{}
		}
		return true
	}
	return false
}

func resolveAbilityTargetsForContextLocked(
	session *matchSession,
	player model.PlayerState,
	payload model.AbilityUsePayload,
) model.AbilityUsePayload {
	if session == nil {
		return payload
	}

	resolved := payload
	switch payload.Ability {
	case model.AbilitySearch, model.AbilityDetainer, model.AbilityPickPocket:
		if resolved.TargetPlayerID == "" {
			resolved.TargetPlayerID = nearestPlayerTargetLocked(player, session.gameState.Players, true)
		}
	case model.AbilityTracker:
		if resolved.TargetPlayerID == "" {
			resolved.TargetPlayerID = nearestPlayerTargetLocked(player, session.gameState.Players, false)
		}
	case model.AbilityLocksmith:
		if resolved.TargetDoorID == 0 {
			resolved.TargetDoorID = firstReachableDoorForPlayerLocked(
				session,
				player,
				session.gameState.Map,
				true,
			)
		}
	}

	return resolved
}

func nearestPlayerTargetLocked(
	actor model.PlayerState,
	players []model.PlayerState,
	requireSameRoom bool,
) model.PlayerID {
	var bestID model.PlayerID
	bestDistance := float32(0)

	for _, candidate := range players {
		if candidate.ID == "" || candidate.ID == actor.ID || !candidate.Alive {
			continue
		}
		if requireSameRoom {
			if actor.CurrentRoomID == "" || candidate.CurrentRoomID != actor.CurrentRoomID {
				continue
			}
		}

		dx := candidate.Position.X - actor.Position.X
		dy := candidate.Position.Y - actor.Position.Y
		distanceSquared := (dx * dx) + (dy * dy)
		if bestID == "" || distanceSquared < bestDistance || (distanceSquared == bestDistance && candidate.ID < bestID) {
			bestID = candidate.ID
			bestDistance = distanceSquared
		}
	}

	return bestID
}

func firstReachableDoorForPlayerLocked(
	session *matchSession,
	player model.PlayerState,
	mapState model.MapState,
	allowRestrictedPrisonerTarget bool,
) model.DoorID {
	bestDoorID := model.DoorID(0)
	for _, door := range mapState.Doors {
		if door.ID == 0 {
			continue
		}
		if !canPlayerTargetDoorLocked(session, player, door, mapState, allowRestrictedPrisonerTarget) {
			continue
		}
		if bestDoorID == 0 || door.ID < bestDoorID {
			bestDoorID = door.ID
		}
	}
	return bestDoorID
}

func canPlayerTargetDoorLocked(
	session *matchSession,
	player model.PlayerState,
	door model.DoorState,
	mapState model.MapState,
	allowRestrictedPrisonerTarget bool,
) bool {
	if player.CurrentRoomID != "" {
		targetRoomID, adjacent := adjacentTargetRoomForDoor(player.CurrentRoomID, door)
		if !adjacent {
			return false
		}
		if targetRoomID != "" && !canPlayerEnterRoomLocked(session, player, targetRoomID, mapState) {
			if !allowRestrictedPrisonerTarget ||
				!isPrisonerRestrictedRoom(targetRoomID) ||
				!gamemap.IsPrisonerPlayer(player) {
				return false
			}
		}
	}

	if cell, exists := cellStateForDoorID(mapState.Cells, door.ID); exists && !gamemap.CanOperateCellDoor(player, cell) {
		return false
	}
	return true
}

func canPlayerEnterRoomLocked(
	session *matchSession,
	player model.PlayerState,
	targetRoomID model.RoomID,
	mapState model.MapState,
) bool {
	decision := gamemap.EvaluateRoomEntry(player, targetRoomID, mapState)
	if decision.Allowed {
		return true
	}

	if session == nil || !gamemap.IsPrisonerPlayer(player) {
		return false
	}
	if _, unlocked := session.prisonerUnlockedRooms[targetRoomID]; !unlocked {
		return false
	}
	return decision.Verdict == gamemap.AccessDenyPowerRoomAuthorityOnly ||
		decision.Verdict == gamemap.AccessDenyAmmoAuthorityOnly
}

func isPrisonerRestrictedRoom(roomID model.RoomID) bool {
	return roomID == gamemap.RoomPowerRoom || roomID == gamemap.RoomAmmoRoom
}

func unlockRoomForPrisonersFromDoorLocked(session *matchSession, door model.DoorState) (model.RoomID, bool) {
	if session == nil {
		return "", false
	}

	var roomID model.RoomID
	switch {
	case door.RoomA == gamemap.RoomPowerRoom || door.RoomB == gamemap.RoomPowerRoom:
		roomID = gamemap.RoomPowerRoom
	case door.RoomA == gamemap.RoomAmmoRoom || door.RoomB == gamemap.RoomAmmoRoom:
		roomID = gamemap.RoomAmmoRoom
	default:
		return "", false
	}

	if session.prisonerUnlockedRooms == nil {
		session.prisonerUnlockedRooms = make(map[model.RoomID]struct{})
	}
	if _, exists := session.prisonerUnlockedRooms[roomID]; exists {
		return roomID, false
	}
	session.prisonerUnlockedRooms[roomID] = struct{}{}
	return roomID, true
}

func adjacentTargetRoomForDoor(currentRoomID model.RoomID, door model.DoorState) (model.RoomID, bool) {
	switch currentRoomID {
	case door.RoomA:
		return door.RoomB, true
	case door.RoomB:
		return door.RoomA, true
	default:
		return "", false
	}
}

func cellStateForDoorID(cells []model.CellState, doorID model.DoorID) (model.CellState, bool) {
	if doorID == 0 {
		return model.CellState{}, false
	}
	for _, cell := range cells {
		if cell.DoorID == doorID {
			return cell, true
		}
	}
	return model.CellState{}, false
}

func applyCardCommandLocked(
	session *matchSession,
	player *model.PlayerState,
	payload model.CardUsePayload,
	playerIndex map[model.PlayerID]int,
	changedPlayers map[model.PlayerID]struct{},
	changedDoors map[model.DoorID]struct{},
	changedEntities map[model.EntityID]struct{},
) bool {
	if session == nil || player == nil || !cards.IsKnownCard(payload.Card) {
		return false
	}
	if !cards.RemoveCard(player, payload.Card) {
		return false
	}

	restoreCard := func() {
		_ = cards.AddCard(player, payload.Card)
	}

	applied := false
	feedbackKind := model.ActionFeedbackKindSystem
	feedbackLevel := model.ActionFeedbackLevelInfo
	feedbackMessage := ""
	switch payload.Card {
	case model.CardMorphine:
		maxHearts := combat.MaxHeartsHalfForRole(player.Role)
		if maxHearts == 0 {
			maxHearts = 6
		}
		before := player.HeartsHalf
		if before < maxHearts {
			heal := uint8(2)
			space := maxHearts - before
			if heal > space {
				heal = space
			}
			player.HeartsHalf += heal
			applied = heal > 0
		}

	case model.CardBullet:
		maxBullets := uint8(255)
		if gamemap.IsAuthorityPlayer(*player) {
			maxBullets = authorityAmmoMax
		}
		if player.Bullets < maxBullets {
			player.Bullets++
			if syncAuthorityBulletStackLocked(player) {
				changedPlayers[player.ID] = struct{}{}
			}
			applied = true
		}

	case model.CardMoney:
		applied, feedbackMessage, feedbackLevel = tryBribeNPCPrisonerWithMoneyCardLocked(
			session,
			player,
			payload.TargetEntityID,
			changedEntities,
		)
		feedbackKind = model.ActionFeedbackKindPurchase

	case model.CardSpeed:
		duration := cards.SpeedDurationTicks(session.tickRateHz)
		if duration > 0 {
			applied = upsertEffectLocked(player, model.EffectSpeedBoost, session.tickID+duration, player.ID, 0, 1)
		}

	case model.CardArmorPlate:
		endTick := session.gameState.Phase.EndsTick
		if endTick == 0 || endTick <= session.tickID {
			endTick = session.tickID + uint64(session.tickRateHz)
		}
		changed := false
		if player.TempHeartsHalf < 2 {
			player.TempHeartsHalf = 2
			changed = true
		}
		if upsertEffectLocked(player, model.EffectArmorPlate, endTick, player.ID, 0, 1) {
			changed = true
		}
		applied = changed

	case model.CardLockSnap:
		if payload.TargetDoorID == 0 {
			break
		}
		for index := range session.gameState.Map.Doors {
			door := &session.gameState.Map.Doors[index]
			if door.ID != payload.TargetDoorID {
				continue
			}
			if !door.Locked {
				break
			}

			restoreTick := session.gameState.Phase.EndsTick
			if restoreTick == 0 || restoreTick <= session.tickID {
				restoreTick = session.tickID + uint64(session.tickRateHz)
			}
			if restoreTick <= session.tickID {
				restoreTick = session.tickID + 1
			}

			if session.lockSnapDoorRestores == nil {
				session.lockSnapDoorRestores = make(map[model.DoorID]lockSnapDoorRestoreState)
			}
			session.lockSnapDoorRestores[door.ID] = lockSnapDoorRestoreState{
				RestoreTick: restoreTick,
				Locked:      door.Locked,
				Open:        door.Open,
			}

			changed := false
			door.Locked = false
			changed = true
			if !door.Open {
				door.Open = true
				changed = true
			}
			if changed {
				changedDoors[door.ID] = struct{}{}
				applied = true
			}
			break
		}

	case model.CardItemSteal:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			break
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive || target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			break
		}
		appliedSteal := false
		if payload.TargetItem != "" {
			appliedSteal = items.TransferItem(target, player, payload.TargetItem, 1)
		} else {
			appliedSteal = transferFirstStackQuantityLocked(target, player, 1)
		}
		if appliedSteal {
			if ensureEquippedItemForPlayerLocked(target) {
				changedPlayers[target.ID] = struct{}{}
			}
			if ensureEquippedItemForPlayerLocked(player) {
				changedPlayers[player.ID] = struct{}{}
			}
			changedPlayers[target.ID] = struct{}{}
			applied = true
		}

	case model.CardItemGrab:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			break
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive || target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			break
		}
		item := cards.DeterministicGrabFromInventory(player.ID, target.ID, session.tickID, target.Inventory)
		if item == "" {
			break
		}
		if items.TransferItem(target, player, item, 1) {
			if ensureEquippedItemForPlayerLocked(target) {
				changedPlayers[target.ID] = struct{}{}
			}
			if ensureEquippedItemForPlayerLocked(player) {
				changedPlayers[player.ID] = struct{}{}
			}
			changedPlayers[target.ID] = struct{}{}
			applied = true
		}

	case model.CardScrapBundle:
		addedWood := items.AddItem(player, model.ItemWood, 1)
		addedMetal := items.AddItem(player, model.ItemMetalSlab, 1)
		applied = addedWood || addedMetal
		if applied && ensureEquippedItemForPlayerLocked(player) {
			changedPlayers[player.ID] = struct{}{}
		}

	case model.CardDoorStop:
		if payload.TargetDoorID == 0 || !session.gameState.Map.PowerOn {
			break
		}
		duration := cards.DoorStopDurationTicks(session.tickRateHz)
		if duration == 0 {
			break
		}
		for index := range session.gameState.Map.Doors {
			door := &session.gameState.Map.Doors[index]
			if door.ID != payload.TargetDoorID {
				continue
			}
			changed := false
			if door.Open {
				door.Open = false
				changed = true
			}
			blockUntil := session.tickID + duration
			if blockUntil > door.BlockedUntilTick {
				door.BlockedUntilTick = blockUntil
				changed = true
			}
			if door.LockedByPlayerID != player.ID {
				door.LockedByPlayerID = player.ID
				changed = true
			}
			if changed {
				changedDoors[door.ID] = struct{}{}
				applied = true
			}
			break
		}

	case model.CardGetOutOfJailFree:
		if player.SolitaryUntilTick != 0 || player.LockedInCell != 0 {
			player.SolitaryUntilTick = 0
			player.LockedInCell = 0
			applied = true
		}

	default:
	}

	if !applied {
		restoreCard()
		if applyActionFeedbackLocked(player, feedbackKind, feedbackLevel, feedbackMessage, session.tickID) {
			changedPlayers[player.ID] = struct{}{}
			return true
		}
		return false
	}

	if applyActionFeedbackLocked(player, feedbackKind, feedbackLevel, feedbackMessage, session.tickID) {
		changedPlayers[player.ID] = struct{}{}
	}

	return true
}

func canUseAbilityAtCurrentTick(
	session *matchSession,
	player model.PlayerState,
	ability model.AbilityType,
) (bool, string) {
	cooldowns := abilityCooldownMapLocked(session, player.ID)
	cooldownUntil := cooldowns[ability]
	if cooldownUntil != 0 && session.tickID < cooldownUntil {
		return false, fmt.Sprintf("%s is on cooldown for %dt", ability, cooldownUntil-session.tickID)
	}

	dailyLimit := abilities.DailyUseLimit(ability)
	if dailyLimit == 0 {
		return true, ""
	}
	if session.gameState.Phase.Current != model.PhaseDay {
		return false, "this ability is only available during day phase"
	}
	dayStart := session.gameState.Phase.StartedTick
	if dayStart == 0 {
		return false, "day phase has not started yet"
	}

	usage := abilityDailyUsageMapLocked(session, player.ID)
	record := usage[ability]
	if record.DayStartTick != dayStart {
		return true, ""
	}
	if record.UseCount >= dailyLimit {
		if dailyLimit == 1 {
			return false, "you already used this ability today"
		}
		return false, fmt.Sprintf("you already used this ability %d/%d times today", record.UseCount, dailyLimit)
	}
	return true, ""
}

func registerAbilityUseLocked(
	session *matchSession,
	player model.PlayerState,
	ability model.AbilityType,
) {
	cooldownTicks := abilities.CooldownTicks(ability, session.tickRateHz)
	if cooldownTicks > 0 {
		cooldowns := abilityCooldownMapLocked(session, player.ID)
		cooldowns[ability] = session.tickID + cooldownTicks
	}
	dailyLimit := abilities.DailyUseLimit(ability)
	if dailyLimit > 0 && session.gameState.Phase.Current == model.PhaseDay {
		usage := abilityDailyUsageMapLocked(session, player.ID)
		record := usage[ability]
		if record.DayStartTick != session.gameState.Phase.StartedTick {
			record.DayStartTick = session.gameState.Phase.StartedTick
			record.UseCount = 0
		}
		record.UseCount++
		usage[ability] = record
	}
}

func abilityCooldownMapLocked(session *matchSession, playerID model.PlayerID) map[model.AbilityType]uint64 {
	if session.abilityCooldownUntil == nil {
		session.abilityCooldownUntil = make(map[model.PlayerID]map[model.AbilityType]uint64)
	}
	perPlayer, exists := session.abilityCooldownUntil[playerID]
	if !exists {
		perPlayer = make(map[model.AbilityType]uint64)
		session.abilityCooldownUntil[playerID] = perPlayer
	}
	return perPlayer
}

func abilityUsedDayMapLocked(session *matchSession, playerID model.PlayerID) map[model.AbilityType]uint64 {
	if session.abilityUsedDayStart == nil {
		session.abilityUsedDayStart = make(map[model.PlayerID]map[model.AbilityType]uint64)
	}
	perPlayer, exists := session.abilityUsedDayStart[playerID]
	if !exists {
		perPlayer = make(map[model.AbilityType]uint64)
		session.abilityUsedDayStart[playerID] = perPlayer
	}
	return perPlayer
}

func abilityDailyUsageMapLocked(session *matchSession, playerID model.PlayerID) map[model.AbilityType]dailyAbilityUsage {
	if session.abilityDailyUsage == nil {
		session.abilityDailyUsage = make(map[model.PlayerID]map[model.AbilityType]dailyAbilityUsage)
	}
	perPlayer, exists := session.abilityDailyUsage[playerID]
	if !exists {
		perPlayer = make(map[model.AbilityType]dailyAbilityUsage)
		session.abilityDailyUsage[playerID] = perPlayer
	}
	return perPlayer
}

func transferFirstStackQuantityLocked(
	from *model.PlayerState,
	to *model.PlayerState,
	quantity uint8,
) bool {
	if from == nil || to == nil || quantity == 0 || len(from.Inventory) == 0 {
		return false
	}

	choice := firstInventoryItem(from.Inventory)
	if choice == "" {
		return false
	}
	return items.TransferItem(from, to, choice, quantity)
}

func firstInventoryItem(inventory []model.ItemStack) model.ItemType {
	if len(inventory) == 0 {
		return ""
	}
	first := model.ItemType("")
	for _, stack := range inventory {
		if stack.Quantity == 0 {
			continue
		}
		if first == "" || stack.Item < first {
			first = stack.Item
		}
	}
	return first
}

func upsertEffectLocked(
	player *model.PlayerState,
	effect model.EffectType,
	endsTick uint64,
	sourcePID model.PlayerID,
	sourceID model.EntityID,
	stacks uint8,
) bool {
	if player == nil || effect == "" {
		return false
	}
	if stacks == 0 {
		stacks = 1
	}

	for index := range player.Effects {
		if player.Effects[index].Effect != effect {
			continue
		}
		changed := false
		if endsTick > player.Effects[index].EndsTick {
			player.Effects[index].EndsTick = endsTick
			changed = true
		}
		if sourcePID != "" && player.Effects[index].SourcePID != sourcePID {
			player.Effects[index].SourcePID = sourcePID
			changed = true
		}
		if sourceID != 0 && player.Effects[index].SourceID != sourceID {
			player.Effects[index].SourceID = sourceID
			changed = true
		}
		if stacks > player.Effects[index].Stacks {
			player.Effects[index].Stacks = stacks
			changed = true
		}
		return changed
	}

	player.Effects = append(player.Effects, model.EffectState{
		Effect:    effect,
		EndsTick:  endsTick,
		Stacks:    stacks,
		SourceID:  sourceID,
		SourcePID: sourcePID,
	})
	return true
}

func removeEffectLocked(player *model.PlayerState, effect model.EffectType) bool {
	if player == nil || effect == "" || len(player.Effects) == 0 {
		return false
	}

	next := player.Effects[:0]
	removed := false
	for _, existing := range player.Effects {
		if existing.Effect == effect {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	if !removed {
		return false
	}
	player.Effects = append([]model.EffectState(nil), next...)
	return true
}

func hasActiveEffect(player model.PlayerState, effect model.EffectType, tickID uint64) bool {
	for _, existing := range player.Effects {
		if existing.Effect != effect {
			continue
		}
		if existing.EndsTick != 0 && tickID > existing.EndsTick {
			continue
		}
		return true
	}
	return false
}

func summarizeInventoryForFeedback(inventory []model.ItemStack) string {
	if len(inventory) == 0 {
		return "empty"
	}

	parts := make([]string, 0, len(inventory))
	for _, stack := range inventory {
		if stack.Item == "" || stack.Quantity == 0 {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s x%d", stack.Item, stack.Quantity))
	}
	if len(parts) == 0 {
		return "empty"
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func expireTimedStatesLocked(
	session *matchSession,
	changedPlayers map[model.PlayerID]struct{},
	changedDoors map[model.DoorID]struct{},
) {
	if session == nil {
		return
	}

	for index := range session.gameState.Map.Doors {
		door := &session.gameState.Map.Doors[index]
		if door.BlockedUntilTick == 0 || session.tickID <= door.BlockedUntilTick {
			continue
		}
		door.BlockedUntilTick = 0
		door.LockedByPlayerID = ""
		changedDoors[door.ID] = struct{}{}
	}

	if len(session.lockSnapDoorRestores) > 0 {
		for doorID, restore := range session.lockSnapDoorRestores {
			if restore.RestoreTick == 0 || session.tickID <= restore.RestoreTick {
				continue
			}

			doorIndex := -1
			for index := range session.gameState.Map.Doors {
				if session.gameState.Map.Doors[index].ID == doorID {
					doorIndex = index
					break
				}
			}
			if doorIndex >= 0 {
				door := &session.gameState.Map.Doors[doorIndex]
				changed := false
				if door.Locked != restore.Locked {
					door.Locked = restore.Locked
					changed = true
				}
				if session.gameState.Map.PowerOn && door.Open != restore.Open {
					door.Open = restore.Open
					changed = true
				}
				if changed {
					changedDoors[door.ID] = struct{}{}
				}
			}

			delete(session.lockSnapDoorRestores, doorID)
		}
	}

	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if len(player.Effects) == 0 {
			continue
		}

		nextEffects := player.Effects[:0]
		removedAny := false
		armorActive := false
		for _, effect := range player.Effects {
			if effect.EndsTick != 0 && session.tickID > effect.EndsTick {
				removedAny = true
				continue
			}
			if effect.Effect == model.EffectArmorPlate {
				armorActive = true
			}
			nextEffects = append(nextEffects, effect)
		}
		if removedAny {
			player.Effects = append([]model.EffectState(nil), nextEffects...)
			if !armorActive && player.TempHeartsHalf > 0 {
				player.TempHeartsHalf = 0
			}
			changedPlayers[player.ID] = struct{}{}
		}
	}

	if len(session.chameleonPendingUntil) > 0 {
		for index := range session.gameState.Players {
			player := &session.gameState.Players[index]
			pendingUntil := session.chameleonPendingUntil[player.ID]
			if pendingUntil == 0 || session.tickID < pendingUntil {
				continue
			}
			lastMoveTick := uint64(0)
			if session.lastMoveIntentTick != nil {
				lastMoveTick = session.lastMoveIntentTick[player.ID]
			}
			if lastMoveTick >= pendingUntil {
				delete(session.chameleonPendingUntil, player.ID)
				continue
			}
			if hasActiveEffect(*player, model.EffectChameleon, session.tickID) {
				delete(session.chameleonPendingUntil, player.ID)
				continue
			}

			if upsertEffectLocked(player, model.EffectChameleon, 0, player.ID, 0, 1) {
				changedPlayers[player.ID] = struct{}{}
				if applyActionFeedbackLocked(
					player,
					model.ActionFeedbackKindSystem,
					model.ActionFeedbackLevelSuccess,
					"Chameleon active: hidden until you move.",
					session.tickID,
				) {
					changedPlayers[player.ID] = struct{}{}
				}
			}
			delete(session.chameleonPendingUntil, player.ID)
		}
	}

	for index := range session.gameState.Players {
		player := &session.gameState.Players[index]
		if ensureEquippedItemForPlayerLocked(player) {
			changedPlayers[player.ID] = struct{}{}
		}
	}
}

func applyAlarmAndGuardSystemLocked(
	session *matchSession,
	playerIndex map[model.PlayerID]int,
	changedPlayers map[model.PlayerID]struct{},
	changedEntities map[model.EntityID]struct{},
	removedEntities map[model.EntityID]struct{},
) {
	if session == nil {
		return
	}

	alarm := session.gameState.Map.Alarm
	if !alarm.Active {
		clearAlarmGuardEntitiesLocked(session, changedEntities, removedEntities)
		return
	}

	if alarm.EndsTick != 0 && session.tickID >= alarm.EndsTick {
		session.gameState.Map.Alarm = model.AlarmState{Active: false}
		clearAlarmGuardEntitiesLocked(session, changedEntities, removedEntities)
		return
	}

	targetIDs := prison.RestrictedPrisonerIDs(session.gameState.Players, session.gameState.Map)
	if len(targetIDs) == 0 {
		session.gameState.Map.Alarm = model.AlarmState{Active: false}
		clearAlarmGuardEntitiesLocked(session, changedEntities, removedEntities)
		return
	}

	ensureAlarmGuardEntitiesLocked(session, changedEntities)

	guardInterval := prison.GuardShotIntervalTicks(session.tickRateHz)
	if guardInterval == 0 {
		guardInterval = 1
	}

	for _, targetID := range targetIDs {
		targetIdx, exists := playerIndex[targetID]
		if !exists {
			continue
		}

		target := &session.gameState.Players[targetIdx]
		if !target.Alive || !prison.IsRestrictedRoom(target.CurrentRoomID, session.gameState.Map) || !gamemap.IsPrisonerPlayer(*target) {
			continue
		}

		lastShotTick := session.guardLastShotTick[targetID]
		if lastShotTick != 0 && session.tickID < (lastShotTick+guardInterval) {
			continue
		}

		result := combat.ApplyDamage(target, prison.GuardShotDamageHalf)
		permanentlyEliminated := false
		if result.Eliminated {
			_, permanentlyEliminated = consumeLifeAndRespawnIfAvailableLocked(target)
		}
		if result.AppliedHalf == 0 {
			continue
		}

		session.guardLastShotTick[targetID] = session.tickID
		changedPlayers[targetID] = struct{}{}
		feedbackMessage := fmt.Sprintf("Alarm guard hit you for %s heart(s).", formatHalfHearts(result.AppliedHalf))
		if result.Eliminated {
			if permanentlyEliminated {
				feedbackMessage = "Alarm guards eliminated you in a restricted zone."
			} else {
				feedbackMessage = fmt.Sprintf("Alarm guards took a life. %d lives remaining.", target.LivesRemaining)
			}
		}
		feedbackLevel := model.ActionFeedbackLevelWarning
		if result.Eliminated && permanentlyEliminated {
			feedbackLevel = model.ActionFeedbackLevelError
		}
		if applyActionFeedbackLocked(
			target,
			model.ActionFeedbackKindAlarm,
			feedbackLevel,
			feedbackMessage,
			session.tickID,
		) {
			changedPlayers[targetID] = struct{}{}
		}
	}
}

func ensureAlarmGuardEntitiesLocked(
	session *matchSession,
	changedEntities map[model.EntityID]struct{},
) {
	if session.guardEntityByRoom == nil {
		session.guardEntityByRoom = make(map[model.RoomID]model.EntityID)
	}

	for _, zone := range session.gameState.Map.RestrictedZones {
		if !zone.Restricted || zone.RoomID == "" {
			continue
		}

		if existingID, exists := session.guardEntityByRoom[zone.RoomID]; exists {
			if _, exists := findEntityIndexByID(session.gameState.Entities, existingID); exists {
				continue
			}
		}

		guard := newAlarmGuardEntityLocked(session, zone.RoomID)
		session.gameState.Entities = append(session.gameState.Entities, guard)
		session.guardEntityByRoom[zone.RoomID] = guard.ID
		changedEntities[guard.ID] = struct{}{}
	}
}

func clearAlarmGuardEntitiesLocked(
	session *matchSession,
	changedEntities map[model.EntityID]struct{},
	removedEntities map[model.EntityID]struct{},
) {
	if len(session.gameState.Entities) == 0 {
		session.guardEntityByRoom = make(map[model.RoomID]model.EntityID)
		session.guardLastShotTick = make(map[model.PlayerID]uint64)
		return
	}

	kept := make([]model.EntityState, 0, len(session.gameState.Entities))
	for _, entity := range session.gameState.Entities {
		if entity.Kind == model.EntityKindNPCGuard {
			delete(changedEntities, entity.ID)
			removedEntities[entity.ID] = struct{}{}
			continue
		}
		kept = append(kept, entity)
	}

	session.gameState.Entities = kept
	session.guardEntityByRoom = make(map[model.RoomID]model.EntityID)
	session.guardLastShotTick = make(map[model.PlayerID]uint64)
}

func newAlarmGuardEntityLocked(session *matchSession, roomID model.RoomID) model.EntityState {
	session.nextEntityID++

	position := model.Vector2{}
	if room, exists := defaultMatchLayout.Room(roomID); exists {
		position = model.Vector2{
			X: float32(room.Min.X+room.Max.X) / 2,
			Y: float32(room.Min.Y+room.Max.Y) / 2,
		}
	}

	return model.EntityState{
		ID:       session.nextEntityID,
		Kind:     model.EntityKindNPCGuard,
		Position: position,
		Velocity: model.Vector2{},
		Active:   true,
		RoomID:   roomID,
		Tags: []string{
			"alarm_guard",
			"restricted_zone",
		},
	}
}

func spawnNPCPrisonersLocked(session *matchSession) {
	if session == nil {
		return
	}
	if session.npcPrisonerBribeState == nil {
		session.npcPrisonerBribeState = make(map[model.EntityID]npcPrisonerBribeState)
	}

	for _, roomID := range npcPrisonerRooms {
		session.nextEntityID++
		entityID := session.nextEntityID
		offerItem, offerCost, ok := deterministicNPCPrisonerOffer(session.matchID, roomID, entityID, 0)
		if !ok {
			continue
		}

		position := model.Vector2{}
		if room, exists := defaultMatchLayout.Room(roomID); exists {
			position = model.Vector2{
				X: float32(room.Min.X+room.Max.X) / 2,
				Y: float32(room.Min.Y+room.Max.Y) / 2,
			}
		}

		state := npcPrisonerBribeState{
			OfferItem: offerItem,
			OfferCost: offerCost,
			Stock:     1,
		}
		session.npcPrisonerBribeState[entityID] = state
		session.gameState.Entities = append(session.gameState.Entities, model.EntityState{
			ID:       entityID,
			Kind:     model.EntityKindNPCPrisoner,
			Position: position,
			Velocity: model.Vector2{},
			Active:   true,
			RoomID:   roomID,
			Tags:     npcPrisonerTags(state),
		})
	}
}

func refreshNPCPrisonerOffersForNightLocked(session *matchSession, cycle uint8) []model.EntityState {
	if session == nil || len(session.gameState.Entities) == 0 {
		return nil
	}
	if session.npcPrisonerBribeState == nil {
		session.npcPrisonerBribeState = make(map[model.EntityID]npcPrisonerBribeState)
	}

	changedEntityIDs := make(map[model.EntityID]struct{})
	for index := range session.gameState.Entities {
		entity := &session.gameState.Entities[index]
		if entity.Kind != model.EntityKindNPCPrisoner {
			continue
		}

		offerItem, offerCost, ok := deterministicNPCPrisonerOffer(session.matchID, entity.RoomID, entity.ID, cycle)
		if !ok {
			continue
		}

		state := session.npcPrisonerBribeState[entity.ID]
		next := npcPrisonerBribeState{
			OfferItem: offerItem,
			OfferCost: offerCost,
			Stock:     1,
		}
		if state == next {
			continue
		}

		session.npcPrisonerBribeState[entity.ID] = next
		entity.Tags = npcPrisonerTags(next)
		changedEntityIDs[entity.ID] = struct{}{}
	}

	return collectChangedEntities(session, changedEntityIDs)
}

func tryBribeNPCPrisonerWithMoneyCardLocked(
	session *matchSession,
	player *model.PlayerState,
	entityID model.EntityID,
	changedEntities map[model.EntityID]struct{},
) (bool, string, model.ActionFeedbackLevel) {
	if session == nil || player == nil || entityID == 0 {
		return false, "Select a valid NPC prisoner.", model.ActionFeedbackLevelWarning
	}
	if !player.Alive || !gamemap.IsPrisonerPlayer(*player) {
		return false, "Only living prisoners can bribe NPC prisoners.", model.ActionFeedbackLevelWarning
	}
	if session.gameState.Phase.Current != model.PhaseNight {
		return false, "NPC deals only happen at night.", model.ActionFeedbackLevelWarning
	}

	entityIndex, exists := findEntityIndexByID(session.gameState.Entities, entityID)
	if !exists {
		return false, "NPC prisoner not available.", model.ActionFeedbackLevelWarning
	}
	entity := &session.gameState.Entities[entityIndex]
	if !entity.Active || entity.Kind != model.EntityKindNPCPrisoner {
		return false, "NPC prisoner not available.", model.ActionFeedbackLevelWarning
	}
	if entity.RoomID == "" || player.CurrentRoomID == "" || entity.RoomID != player.CurrentRoomID {
		return false, "Move into the same room to bribe.", model.ActionFeedbackLevelWarning
	}

	state, exists := session.npcPrisonerBribeState[entityID]
	if !exists || state.OfferItem == "" || state.OfferCost == 0 || state.Stock == 0 {
		if exists && state.Stock == 0 {
			return false, "NPC offer sold out.", model.ActionFeedbackLevelWarning
		}
		return false, "NPC offer unavailable.", model.ActionFeedbackLevelWarning
	}

	if state.PayerID == "" || state.PayerID != player.ID {
		state.PayerID = player.ID
		state.PaidCards = 0
	}

	if state.PaidCards+1 < state.OfferCost {
		state.PaidCards++
		session.npcPrisonerBribeState[entityID] = state
		entity.Tags = npcPrisonerTags(state)
		if changedEntities != nil {
			changedEntities[entityID] = struct{}{}
		}
		return true, fmt.Sprintf("Bribe progress %d/%d for %s.", state.PaidCards, state.OfferCost, state.OfferItem), model.ActionFeedbackLevelInfo
	}

	if !items.AddItem(player, state.OfferItem, 1) {
		return false, "Inventory full; NPC deal cancelled.", model.ActionFeedbackLevelWarning
	}

	state.Stock--
	state.PayerID = ""
	state.PaidCards = 0
	session.npcPrisonerBribeState[entityID] = state
	entity.Tags = npcPrisonerTags(state)
	if changedEntities != nil {
		changedEntities[entityID] = struct{}{}
	}
	return true, fmt.Sprintf("NPC deal complete: received %s.", state.OfferItem), model.ActionFeedbackLevelSuccess
}

func deterministicNPCPrisonerOffer(
	matchID model.MatchID,
	roomID model.RoomID,
	entityID model.EntityID,
	cycle uint8,
) (model.ItemType, uint8, bool) {
	catalog := items.BlackMarketCatalog()
	if len(catalog) == 0 {
		return "", 0, false
	}

	eligible := make([]items.BlackMarketOffer, 0, len(catalog))
	for _, offer := range catalog {
		if offer.Quantity == 0 || offer.MoneyCardCost == 0 {
			continue
		}
		if offer.Item == model.ItemGoldenBullet ||
			offer.Item == model.ItemPistol ||
			offer.Item == model.ItemHuntingRifle {
			continue
		}
		eligible = append(eligible, offer)
	}
	if len(eligible) == 0 {
		return "", 0, false
	}

	sort.Slice(eligible, func(i int, j int) bool {
		return eligible[i].Item < eligible[j].Item
	})

	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(matchID))
	_, _ = hasher.Write([]byte(roomID))
	_, _ = hasher.Write([]byte(strconv.FormatUint(uint64(entityID), 10)))
	_, _ = hasher.Write([]byte{cycle})
	choice := hasher.Sum64() % uint64(len(eligible))

	selected := eligible[choice]
	return selected.Item, selected.MoneyCardCost, true
}

func npcPrisonerTags(state npcPrisonerBribeState) []string {
	tags := []string{
		"npc_prisoner",
		fmt.Sprintf("offer_item:%s", state.OfferItem),
		fmt.Sprintf("offer_cost:%d", state.OfferCost),
		fmt.Sprintf("stock:%d", state.Stock),
	}
	if state.PayerID != "" {
		tags = append(tags, fmt.Sprintf("payer:%s", state.PayerID))
	}
	if state.PaidCards > 0 {
		tags = append(tags, fmt.Sprintf("paid:%d", state.PaidCards))
	}
	sort.Strings(tags)
	return tags
}

func findEntityIndexByID(entities []model.EntityState, entityID model.EntityID) (int, bool) {
	for index := range entities {
		if entities[index].ID == entityID {
			return index, true
		}
	}
	return -1, false
}

func applyFireWeaponCommandLocked(
	session *matchSession,
	shooterID model.PlayerID,
	payload model.FireWeaponPayload,
	playerIndex map[model.PlayerID]int,
	occupiedTiles map[gamemap.Point]model.PlayerID,
	changedPlayers map[model.PlayerID]struct{},
	changedEntities map[model.EntityID]struct{},
) bool {
	shooterIndex, shooterExists := playerIndex[shooterID]
	if !shooterExists {
		return false
	}

	shooter := &session.gameState.Players[shooterIndex]
	weapon := payload.Weapon
	if !combat.IsSupportedWeapon(weapon) {
		weapon = shooter.EquippedItem
	}
	if !combat.IsSupportedWeapon(weapon) {
		return false
	}
	if !combat.CanUseWeapon(*shooter, weapon) {
		if weapon != shooter.EquippedItem &&
			combat.IsSupportedWeapon(shooter.EquippedItem) &&
			combat.CanUseWeapon(*shooter, shooter.EquippedItem) {
			weapon = shooter.EquippedItem
		}
	}
	if !combat.CanUseWeapon(*shooter, weapon) {
		if ensureEquippedItemForPlayerLocked(shooter) {
			changedPlayers[shooter.ID] = struct{}{}
		}
		return false
	}
	if shooter.EquippedItem != weapon {
		shooter.EquippedItem = weapon
		changedPlayers[shooter.ID] = struct{}{}
	}

	attackRange := combat.WeaponRangeTiles(weapon)
	targetID, hasTarget := combat.SelectTarget(
		session.gameState.Players,
		shooterID,
		model.Vector2{
			X: payload.TargetX,
			Y: payload.TargetY,
		},
		attackRange,
		combat.AimAssistRadiusTiles,
	)
	if !hasTarget {
		return false
	}

	targetIndex, targetExists := playerIndex[targetID]
	if !targetExists {
		return false
	}
	target := &session.gameState.Players[targetIndex]
	if !target.Alive {
		return false
	}
	targetStateBeforeDamage := *target

	shooterLabel := shooter.Name
	if shooterLabel == "" {
		shooterLabel = string(shooter.ID)
	}
	targetLabel := target.Name
	if targetLabel == "" {
		targetLabel = string(target.ID)
	}

	damageHalf, ok := combat.ConsumeShotCostAndResolveDamage(shooter, weapon, payload.UseGoldenRound)
	if !ok {
		return false
	}
	changedPlayers[shooterID] = struct{}{}
	if syncAuthorityBulletStackLocked(shooter) {
		changedPlayers[shooterID] = struct{}{}
	}
	if combat.IsFirearm(weapon) {
		spawnProjectileEntityLocked(session, *shooter, payload.TargetX, payload.TargetY, changedEntities)
	}

	if weapon == combat.WeaponBaton {
		nextTarget, _ := physics.ApplyKnockback(
			*target,
			combat.BatonImpulse(*shooter, *target),
			defaultMatchLayout,
			session.gameState.Map,
			occupiedTiles,
			session.tickID,
			combat.BatonStunDurationTicks(session.tickRateHz),
		)
		if roomID, exists := defaultMatchLayout.RoomAt(physics.TileFromPosition(nextTarget.Position)); exists {
			nextTarget.CurrentRoomID = roomID
		}
		*target = nextTarget
		changedPlayers[target.ID] = struct{}{}
		if applyActionFeedbackLocked(
			shooter,
			model.ActionFeedbackKindStun,
			model.ActionFeedbackLevelSuccess,
			fmt.Sprintf("Baton stunned %s.", targetLabel),
			session.tickID,
		) {
			changedPlayers[shooterID] = struct{}{}
		}
		if applyActionFeedbackLocked(
			target,
			model.ActionFeedbackKindStun,
			model.ActionFeedbackLevelWarning,
			fmt.Sprintf("Stunned by %s.", shooterLabel),
			session.tickID,
		) {
			changedPlayers[target.ID] = struct{}{}
		}
		return true
	}

	if damageHalf == 0 {
		return true
	}
	result := combat.ApplyDamage(target, damageHalf)
	permanentlyEliminated := false
	if result.Eliminated {
		_, permanentlyEliminated = consumeLifeAndRespawnIfAvailableLocked(target)
	}
	if result.AppliedHalf == 0 {
		if applyActionFeedbackLocked(
			shooter,
			model.ActionFeedbackKindCombat,
			model.ActionFeedbackLevelInfo,
			fmt.Sprintf("Shot on %s was absorbed.", targetLabel),
			session.tickID,
		) {
			changedPlayers[shooterID] = struct{}{}
		}
		if applyActionFeedbackLocked(
			target,
			model.ActionFeedbackKindCombat,
			model.ActionFeedbackLevelInfo,
			fmt.Sprintf("Absorbed shot from %s.", shooterLabel),
			session.tickID,
		) {
			changedPlayers[target.ID] = struct{}{}
		}
		return true
	}

	changedPlayers[target.ID] = struct{}{}
	damageText := formatHalfHearts(result.AppliedHalf)
	if applyActionFeedbackLocked(
		shooter,
		model.ActionFeedbackKindCombat,
		model.ActionFeedbackLevelSuccess,
		fmt.Sprintf("Hit %s for %s heart(s).", targetLabel, damageText),
		session.tickID,
	) {
		changedPlayers[shooterID] = struct{}{}
	}

	targetFeedbackLevel := model.ActionFeedbackLevelWarning
	targetFeedbackMessage := fmt.Sprintf("Hit by %s for %s heart(s).", shooterLabel, damageText)
	if result.Eliminated {
		if permanentlyEliminated {
			targetFeedbackLevel = model.ActionFeedbackLevelError
			targetFeedbackMessage = fmt.Sprintf("Eliminated by %s.", shooterLabel)
		} else {
			targetFeedbackMessage = fmt.Sprintf(
				"Life lost to %s. %d lives remaining.",
				shooterLabel,
				target.LivesRemaining,
			)
		}
	}
	if applyActionFeedbackLocked(
		target,
		model.ActionFeedbackKindCombat,
		targetFeedbackLevel,
		targetFeedbackMessage,
		session.tickID,
	) {
		changedPlayers[target.ID] = struct{}{}
	}

	if gamemap.IsAuthorityPlayer(*shooter) &&
		combat.IsFirearm(weapon) &&
		combat.IsUnjustAuthorityShot(targetStateBeforeDamage, session.gameState.Map) {
		if combat.ApplyUnjustShotPenalty(shooter, session.gameState.Phase, session.tickID) {
			changedPlayers[shooterID] = struct{}{}
			if applyActionFeedbackLocked(
				shooter,
				model.ActionFeedbackKindCombat,
				model.ActionFeedbackLevelError,
				fmt.Sprintf("Unjust shot penalty applied until tick %d.", shooter.SolitaryUntilTick),
				session.tickID,
			) {
				changedPlayers[shooterID] = struct{}{}
			}
		}
	}

	return true
}

func tryTransferItemBetweenPlayersLocked(
	session *matchSession,
	playerIndex map[model.PlayerID]int,
	fromPlayerID model.PlayerID,
	toPlayerID model.PlayerID,
	item model.ItemType,
	amount uint8,
) bool {
	if fromPlayerID == "" || toPlayerID == "" || fromPlayerID == toPlayerID || amount == 0 {
		return false
	}

	sourceIndex, sourceExists := playerIndex[fromPlayerID]
	if !sourceExists {
		return false
	}
	targetIndex, targetExists := playerIndex[toPlayerID]
	if !targetExists {
		return false
	}

	source := &session.gameState.Players[sourceIndex]
	target := &session.gameState.Players[targetIndex]
	if !source.Alive || !target.Alive {
		return false
	}
	if source.CurrentRoomID == "" || source.CurrentRoomID != target.CurrentRoomID {
		return false
	}

	return items.TransferItem(source, target, item, amount)
}

func newDroppedItemEntityLocked(
	session *matchSession,
	owner model.PlayerState,
	item model.ItemType,
	quantity uint8,
) model.EntityState {
	session.nextEntityID++

	return model.EntityState{
		ID:            session.nextEntityID,
		Kind:          model.EntityKindDroppedItem,
		OwnerPlayerID: owner.ID,
		Position:      owner.Position,
		Velocity:      model.Vector2{},
		Active:        true,
		RoomID:        owner.CurrentRoomID,
		Tags:          items.BuildDroppedItemTags(item, quantity),
	}
}

func tryPickupDroppedEntityLocked(
	session *matchSession,
	player *model.PlayerState,
	entityID model.EntityID,
	changedEntities map[model.EntityID]struct{},
	removedEntities map[model.EntityID]struct{},
) bool {
	if player == nil || entityID == 0 {
		return false
	}

	entityIndex := -1
	for idx := range session.gameState.Entities {
		if session.gameState.Entities[idx].ID == entityID {
			entityIndex = idx
			break
		}
	}
	if entityIndex < 0 {
		return false
	}

	entity := session.gameState.Entities[entityIndex]
	if !entity.Active || entity.RoomID == "" || player.CurrentRoomID == "" || entity.RoomID != player.CurrentRoomID {
		return false
	}

	item, quantity, ok := items.ParseDroppedItem(entity)
	if !ok || quantity == 0 {
		return false
	}
	if !items.AddItem(player, item, quantity) {
		return false
	}

	session.gameState.Entities = append(session.gameState.Entities[:entityIndex], session.gameState.Entities[entityIndex+1:]...)
	delete(changedEntities, entityID)
	removedEntities[entityID] = struct{}{}
	return true
}

func collectChangedPlayers(
	session *matchSession,
	playerIndex map[model.PlayerID]int,
	changed map[model.PlayerID]struct{},
) []model.PlayerState {
	if len(changed) == 0 {
		return nil
	}

	ids := make([]model.PlayerID, 0, len(changed))
	for playerID := range changed {
		ids = append(ids, playerID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.PlayerState, 0, len(ids))
	for _, playerID := range ids {
		idx := playerIndex[playerID]
		out = append(out, clonePlayerState(session.gameState.Players[idx]))
	}
	return out
}

func mergeChangedPlayersByIDLocked(
	session *matchSession,
	baseline []model.PlayerState,
	extraIDs map[model.PlayerID]struct{},
) []model.PlayerState {
	if len(extraIDs) == 0 {
		return baseline
	}

	byID := make(map[model.PlayerID]model.PlayerState, len(baseline)+len(extraIDs))
	for _, player := range baseline {
		byID[player.ID] = clonePlayerState(player)
	}

	for _, player := range session.gameState.Players {
		if _, include := extraIDs[player.ID]; !include {
			continue
		}
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

func collectChangedDoors(session *matchSession, changed map[model.DoorID]struct{}) []model.DoorState {
	if len(changed) == 0 {
		return nil
	}

	ids := make([]model.DoorID, 0, len(changed))
	for doorID := range changed {
		ids = append(ids, doorID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.DoorState, 0, len(ids))
	for _, doorID := range ids {
		for idx := range session.gameState.Map.Doors {
			if session.gameState.Map.Doors[idx].ID == doorID {
				out = append(out, session.gameState.Map.Doors[idx])
				break
			}
		}
	}

	return out
}

func collectChangedCells(session *matchSession, changed map[model.CellID]struct{}) []model.CellState {
	if len(changed) == 0 {
		return nil
	}

	ids := make([]model.CellID, 0, len(changed))
	for cellID := range changed {
		ids = append(ids, cellID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.CellState, 0, len(ids))
	for _, cellID := range ids {
		for idx := range session.gameState.Map.Cells {
			if session.gameState.Map.Cells[idx].ID == cellID {
				cell := session.gameState.Map.Cells[idx]
				cell.OccupantPlayerIDs = append([]model.PlayerID(nil), cell.OccupantPlayerIDs...)
				cell.Stash = append([]model.ItemStack(nil), cell.Stash...)
				out = append(out, cell)
				break
			}
		}
	}

	return out
}

func collectChangedEntities(session *matchSession, changed map[model.EntityID]struct{}) []model.EntityState {
	if len(changed) == 0 {
		return nil
	}

	ids := make([]model.EntityID, 0, len(changed))
	for entityID := range changed {
		ids = append(ids, entityID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	out := make([]model.EntityState, 0, len(ids))
	for _, entityID := range ids {
		for idx := range session.gameState.Entities {
			if session.gameState.Entities[idx].ID != entityID {
				continue
			}
			entity := session.gameState.Entities[idx]
			entity.Tags = append([]string(nil), entity.Tags...)
			out = append(out, entity)
			break
		}
	}

	return out
}

func mergeChangedEntityStates(first []model.EntityState, second []model.EntityState) []model.EntityState {
	if len(first) == 0 && len(second) == 0 {
		return nil
	}

	byID := make(map[model.EntityID]model.EntityState, len(first)+len(second))
	for _, entity := range first {
		entity.Tags = append([]string(nil), entity.Tags...)
		byID[entity.ID] = entity
	}
	for _, entity := range second {
		entity.Tags = append([]string(nil), entity.Tags...)
		byID[entity.ID] = entity
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

func collectRemovedEntityIDs(removed map[model.EntityID]struct{}) []model.EntityID {
	if len(removed) == 0 {
		return nil
	}

	ids := make([]model.EntityID, 0, len(removed))
	for entityID := range removed {
		ids = append(ids, entityID)
	}
	sort.Slice(ids, func(i int, j int) bool {
		return ids[i] < ids[j]
	})

	return ids
}

func buildPlayerAcksLocked(session *matchSession) []model.PlayerAck {
	if len(session.players) == 0 {
		return nil
	}

	playerIDs := make([]model.PlayerID, 0, len(session.players))
	for playerID := range session.players {
		playerIDs = append(playerIDs, playerID)
	}
	sort.Slice(playerIDs, func(i int, j int) bool {
		return playerIDs[i] < playerIDs[j]
	})

	acks := make([]model.PlayerAck, 0, len(playerIDs))
	for _, playerID := range playerIDs {
		acks = append(acks, model.PlayerAck{
			PlayerID:               playerID,
			LastProcessedClientSeq: session.lastProcessedClientSeq[playerID],
		})
	}

	return acks
}

func makeFullSnapshotLocked(session *matchSession) model.Snapshot {
	state := cloneGameState(session.gameState)
	return model.Snapshot{
		Kind:       model.SnapshotKindFull,
		TickID:     session.tickID,
		State:      &state,
		PlayerAcks: buildPlayerAcksLocked(session),
	}
}

func storeSnapshotLocked(session *matchSession, snapshot model.Snapshot) {
	session.snapshotHistory[snapshot.TickID] = cloneSnapshot(snapshot)

	const historyLimit uint64 = 256
	if snapshot.TickID <= historyLimit {
		return
	}

	cutoff := snapshot.TickID - historyLimit
	for tickID := range session.snapshotHistory {
		if tickID < cutoff {
			delete(session.snapshotHistory, tickID)
		}
	}
}

func cloneSnapshot(in model.Snapshot) model.Snapshot {
	out := in
	if in.State != nil {
		stateCopy := cloneGameState(*in.State)
		out.State = &stateCopy
	}
	if in.Delta != nil {
		deltaCopy := cloneGameDelta(*in.Delta)
		out.Delta = &deltaCopy
	}
	out.PlayerAcks = append([]model.PlayerAck(nil), in.PlayerAcks...)
	return out
}

func cloneGameDelta(in model.GameDelta) model.GameDelta {
	out := in
	out.ChangedPlayers = make([]model.PlayerState, len(in.ChangedPlayers))
	for idx := range in.ChangedPlayers {
		out.ChangedPlayers[idx] = clonePlayerState(in.ChangedPlayers[idx])
	}
	out.RemovedPlayerIDs = append([]model.PlayerID(nil), in.RemovedPlayerIDs...)

	out.ChangedEntities = make([]model.EntityState, len(in.ChangedEntities))
	for idx := range in.ChangedEntities {
		entity := in.ChangedEntities[idx]
		entity.Tags = append([]string(nil), entity.Tags...)
		out.ChangedEntities[idx] = entity
	}
	out.RemovedEntityIDs = append([]model.EntityID(nil), in.RemovedEntityIDs...)

	out.ChangedDoors = append([]model.DoorState(nil), in.ChangedDoors...)
	out.ChangedCells = append([]model.CellState(nil), in.ChangedCells...)
	for idx := range out.ChangedCells {
		out.ChangedCells[idx].OccupantPlayerIDs = append([]model.PlayerID(nil), in.ChangedCells[idx].OccupantPlayerIDs...)
		out.ChangedCells[idx].Stash = append([]model.ItemStack(nil), in.ChangedCells[idx].Stash...)
	}
	out.ChangedZones = append([]model.ZoneState(nil), in.ChangedZones...)

	if in.Phase != nil {
		phaseCopy := *in.Phase
		out.Phase = &phaseCopy
	}
	if in.Status != nil {
		statusCopy := *in.Status
		out.Status = &statusCopy
	}
	if in.CycleCount != nil {
		cycleCopy := *in.CycleCount
		out.CycleCount = &cycleCopy
	}
	if in.PowerOn != nil {
		powerCopy := *in.PowerOn
		out.PowerOn = &powerCopy
	}
	if in.Alarm != nil {
		alarmCopy := *in.Alarm
		out.Alarm = &alarmCopy
	}
	if in.BlackMarketRoomID != nil {
		roomCopy := *in.BlackMarketRoomID
		out.BlackMarketRoomID = &roomCopy
	}
	if in.GameOver != nil {
		gameOverCopy := *in.GameOver
		gameOverCopy.WinnerPlayerIDs = append([]model.PlayerID(nil), in.GameOver.WinnerPlayerIDs...)
		out.GameOver = &gameOverCopy
	}

	return out
}

func cloneGameState(in model.GameState) model.GameState {
	out := in
	out.Players = make([]model.PlayerState, len(in.Players))
	for idx := range in.Players {
		out.Players[idx] = clonePlayerState(in.Players[idx])
	}

	out.Entities = make([]model.EntityState, len(in.Entities))
	for idx := range in.Entities {
		entity := in.Entities[idx]
		entity.Tags = append([]string(nil), entity.Tags...)
		out.Entities[idx] = entity
	}

	out.Map.Doors = append([]model.DoorState(nil), in.Map.Doors...)
	out.Map.Cells = append([]model.CellState(nil), in.Map.Cells...)
	for idx := range out.Map.Cells {
		out.Map.Cells[idx].OccupantPlayerIDs = append([]model.PlayerID(nil), in.Map.Cells[idx].OccupantPlayerIDs...)
		out.Map.Cells[idx].Stash = append([]model.ItemStack(nil), in.Map.Cells[idx].Stash...)
	}
	out.Map.RestrictedZones = append([]model.ZoneState(nil), in.Map.RestrictedZones...)

	if in.GameOver != nil {
		gameOverCopy := *in.GameOver
		gameOverCopy.WinnerPlayerIDs = append([]model.PlayerID(nil), in.GameOver.WinnerPlayerIDs...)
		out.GameOver = &gameOverCopy
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

func snapshotFromSession(session *matchSession) MatchSnapshot {
	players := make([]PlayerSession, 0, len(session.players))
	for _, player := range session.players {
		players = append(players, player)
	}
	sort.Slice(players, func(i int, j int) bool {
		return players[i].PlayerID < players[j].PlayerID
	})

	snapshot := MatchSnapshot{
		MatchID:     session.matchID,
		Status:      session.status,
		CreatedAt:   session.createdAt,
		EndedReason: session.endedReason,
		TickID:      session.tickID,
		Players:     players,
	}

	if session.startedAt != nil {
		startedAtCopy := *session.startedAt
		snapshot.StartedAt = &startedAtCopy
	}
	if session.endedAt != nil {
		endedAtCopy := *session.endedAt
		snapshot.EndedAt = &endedAtCopy
	}

	return snapshot
}
