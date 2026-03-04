package game

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
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
	lockSnapDoorRestores        map[model.DoorID]lockSnapDoorRestoreState
	npcPrisonerBribeState       map[model.EntityID]npcPrisonerBribeState
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
		lockSnapDoorRestores:   make(map[model.DoorID]lockSnapDoorRestoreState),
		npcPrisonerBribeState:  make(map[model.EntityID]npcPrisonerBribeState),
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
	session.lockSnapDoorRestores = make(map[model.DoorID]lockSnapDoorRestoreState)
	session.npcPrisonerBribeState = make(map[model.EntityID]npcPrisonerBribeState)
	session.replayEntries = make([]ReplayEntry, 0, 256)
	syncGameStatePlayersLocked(session)
	assignCellsLocked(session)
	_ = roles.ApplyAssignments(&session.gameState, session.matchID)
	combat.ApplyRoleLoadouts(&session.gameState)
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
			for _, transition := range phaseTransitions {
				if transition.To != model.PhaseNight {
					continue
				}
				nightlyEconomyEntityChanges = mergeChangedEntityStates(
					nightlyEconomyEntityChanges,
					refreshNPCPrisonerOffersForNightLocked(session, transition.Cycle),
				)
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
			if len(mutations.players) > 0 {
				delta.ChangedPlayers = mutations.players
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
				ID:         playerID,
				Name:       playerSession.Name,
				Connected:  true,
				Alive:      true,
				HeartsHalf: 6,
				Position:   model.Vector2{},
				Velocity:   model.Vector2{},
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

	for idx, player := range players {
		player.CurrentRoomID = gamemap.RoomCellBlockA
		player.LockedInCell = 0
		player.Velocity = model.Vector2{}
		player.Position = spawnPositionForPlayerIndex(idx)

		if idx >= len(session.gameState.Map.Cells) {
			player.AssignedCell = 0
			continue
		}

		cell := &session.gameState.Map.Cells[idx]
		cell.OwnerPlayerID = player.ID
		cell.OccupantPlayerIDs = []model.PlayerID{player.ID}

		player.AssignedCell = cell.ID
	}
}

func spawnPositionForPlayerIndex(index int) model.Vector2 {
	const (
		minX = 2
		minY = 13
		maxX = 7
		maxY = 19
	)

	width := (maxX - minX) + 1
	if width <= 0 {
		return model.Vector2{X: minX, Y: minY}
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
			if player.Bullets < 255 {
				player.Bullets++
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
			changed[command.PlayerID] = struct{}{}
			changedEntities[droppedEntity.ID] = struct{}{}

		case model.CmdCraftItem:
			var payload model.CraftItemPayload
			if err := json.Unmarshal(command.Payload, &payload); err != nil {
				continue
			}
			if items.Craft(player, payload.Item) {
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
		payload.MarketRoomID == "" {
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
		if tryPickupDroppedEntityLocked(session, player, payload.TargetEntityID, changedEntities, removedEntities) {
			changed = true
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
	if !gamemap.CanEnterRoom(*player, targetRoomID, session.gameState.Map) {
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
	if !abilities.IsKnownAbility(payload.Ability) || !abilities.CanPlayerUse(*player, payload.Ability) {
		return false
	}
	if !canUseAbilityAtCurrentTick(session, *player, payload.Ability) {
		return false
	}

	applied := false
	feedbackKind := model.ActionFeedbackKindSystem
	feedbackLevel := model.ActionFeedbackLevelInfo
	feedbackMessage := ""
	switch payload.Ability {
	case model.AbilityAlarm:
		durationTicks := prison.AlarmDurationTicks(session.tickRateHz)
		if durationTicks > 0 {
			session.gameState.Map.Alarm = model.AlarmState{
				Active:      true,
				EndsTick:    session.tickID + durationTicks,
				TriggeredBy: player.ID,
			}
			applied = true
			feedbackKind = model.ActionFeedbackKindAlarm
			feedbackLevel = model.ActionFeedbackLevelSuccess
			feedbackMessage = fmt.Sprintf("Alarm triggered (%dt).", durationTicks)
		}

	case model.AbilitySearch:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			return false
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive || target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return false
		}

		confiscated := false
		for _, contraband := range items.ContrabandStacks(*target) {
			if items.RemoveItem(target, contraband.Item, contraband.Quantity) {
				confiscated = true
			}
		}
		if confiscated {
			changedPlayers[target.ID] = struct{}{}
			applied = true
			feedbackMessage = fmt.Sprintf("Search confiscated contraband from %s.", target.Name)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelWarning,
				fmt.Sprintf("Searched by %s; contraband confiscated.", player.Name),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		}

	case model.AbilityCameraMan:
		if player.CurrentRoomID != gamemap.RoomCameraRoom || !session.gameState.Map.PowerOn {
			return false
		}
		duration := abilities.EffectDurationTicks(model.AbilityCameraMan, session.tickRateHz)
		if duration == 0 {
			return false
		}
		trackedCount := 0
		for index := range session.gameState.Players {
			target := &session.gameState.Players[index]
			if !target.Alive || !gamemap.IsPrisonerPlayer(*target) {
				continue
			}
			if !prison.IsRestrictedRoom(target.CurrentRoomID, session.gameState.Map) {
				continue
			}
			if upsertEffectLocked(target, model.EffectTracked, session.tickID+duration, player.ID, 0, 1) {
				changedPlayers[target.ID] = struct{}{}
				applied = true
				trackedCount++
			}
		}
		if applied {
			feedbackMessage = fmt.Sprintf("Camera sweep marked %d restricted prisoner(s).", trackedCount)
		}

	case model.AbilityDetainer:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			return false
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive || target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return false
		}

		duration := abilities.EffectDurationTicks(model.AbilityDetainer, session.tickRateHz)
		if duration == 0 {
			return false
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

	case model.AbilityTracker:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			return false
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive {
			return false
		}
		duration := abilities.EffectDurationTicks(model.AbilityTracker, session.tickRateHz)
		if duration == 0 {
			return false
		}
		if upsertEffectLocked(target, model.EffectTracked, session.tickID+duration, player.ID, 0, 1) {
			changedPlayers[target.ID] = struct{}{}
			applied = true
			feedbackMessage = fmt.Sprintf("Tracking placed on %s.", target.Name)
			if applyActionFeedbackLocked(
				target,
				model.ActionFeedbackKindSystem,
				model.ActionFeedbackLevelWarning,
				fmt.Sprintf("You are tracked by %s.", player.Name),
				session.tickID,
			) {
				changedPlayers[target.ID] = struct{}{}
			}
		}

	case model.AbilityPickPocket:
		targetID := payload.TargetPlayerID
		targetIndex, exists := playerIndex[targetID]
		if !exists || targetID == "" || targetID == player.ID {
			return false
		}
		target := &session.gameState.Players[targetIndex]
		if !target.Alive || target.CurrentRoomID == "" || target.CurrentRoomID != player.CurrentRoomID {
			return false
		}
		if transferFirstStackQuantityLocked(target, player, 1) {
			changedPlayers[target.ID] = struct{}{}
			applied = true
			feedbackMessage = fmt.Sprintf("Pick-pocket stole 1 item from %s.", target.Name)
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

	case model.AbilityHacker:
		nextPower := !session.gameState.Map.PowerOn
		if !prison.ApplyPowerState(&session.gameState.Map, nextPower) {
			return false
		}
		for _, door := range session.gameState.Map.Doors {
			changedDoors[door.ID] = struct{}{}
		}
		applied = true
		feedbackKind = model.ActionFeedbackKindDoor
		powerState := "OFF"
		if session.gameState.Map.PowerOn {
			powerState = "ON"
		}
		feedbackMessage = fmt.Sprintf("Hacker toggled power %s.", powerState)

	case model.AbilityDisguise:
		duration := abilities.EffectDurationTicks(model.AbilityDisguise, session.tickRateHz)
		if duration == 0 {
			return false
		}
		applied = upsertEffectLocked(player, model.EffectDisguised, session.tickID+duration, player.ID, 0, 1)
		if applied {
			feedbackMessage = fmt.Sprintf("Disguise active for %dt.", duration)
		}

	case model.AbilityLocksmith:
		if payload.TargetDoorID == 0 {
			return false
		}
		doorIndex := -1
		for index := range session.gameState.Map.Doors {
			if session.gameState.Map.Doors[index].ID == payload.TargetDoorID {
				doorIndex = index
				break
			}
		}
		if doorIndex < 0 || !session.gameState.Map.PowerOn {
			return false
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
		if changed {
			changedDoors[door.ID] = struct{}{}
			applied = true
			feedbackKind = model.ActionFeedbackKindDoor
			feedbackMessage = fmt.Sprintf("Locksmith opened door %d.", door.ID)
		}

	case model.AbilityChameleon:
		duration := abilities.EffectDurationTicks(model.AbilityChameleon, session.tickRateHz)
		if duration == 0 {
			return false
		}
		applied = upsertEffectLocked(player, model.EffectChameleon, session.tickID+duration, player.ID, 0, 1)
		if applied {
			feedbackMessage = fmt.Sprintf("Chameleon active for %dt.", duration)
		}

	default:
		return false
	}

	if !applied {
		return false
	}
	registerAbilityUseLocked(session, *player, payload.Ability)
	if applyActionFeedbackLocked(player, feedbackKind, feedbackLevel, feedbackMessage, session.tickID) {
		changedPlayers[player.ID] = struct{}{}
	}
	return true
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
		if player.Bullets < 255 {
			player.Bullets++
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
			changedPlayers[target.ID] = struct{}{}
			applied = true
		}

	case model.CardScrapBundle:
		addedWood := items.AddItem(player, model.ItemWood, 1)
		addedMetal := items.AddItem(player, model.ItemMetalSlab, 1)
		applied = addedWood || addedMetal

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
) bool {
	cooldowns := abilityCooldownMapLocked(session, player.ID)
	cooldownUntil := cooldowns[ability]
	if cooldownUntil != 0 && session.tickID < cooldownUntil {
		return false
	}

	if !abilities.OncePerDay(ability) {
		return true
	}
	if session.gameState.Phase.Current != model.PhaseDay {
		return false
	}
	dayStart := session.gameState.Phase.StartedTick
	if dayStart == 0 {
		return false
	}

	usedByDay := abilityUsedDayMapLocked(session, player.ID)
	return usedByDay[ability] != dayStart
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
	if abilities.OncePerDay(ability) && session.gameState.Phase.Current == model.PhaseDay {
		usedByDay := abilityUsedDayMapLocked(session, player.ID)
		usedByDay[ability] = session.gameState.Phase.StartedTick
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
		if result.AppliedHalf == 0 {
			continue
		}

		session.guardLastShotTick[targetID] = session.tickID
		changedPlayers[targetID] = struct{}{}
		feedbackMessage := fmt.Sprintf("Alarm guard hit you for %s heart(s).", formatHalfHearts(result.AppliedHalf))
		if result.Eliminated {
			feedbackMessage = "Alarm guards eliminated you in a restricted zone."
		}
		feedbackLevel := model.ActionFeedbackLevelWarning
		if result.Eliminated {
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
) bool {
	if !combat.IsSupportedWeapon(payload.Weapon) {
		return false
	}

	shooterIndex, shooterExists := playerIndex[shooterID]
	if !shooterExists {
		return false
	}

	shooter := &session.gameState.Players[shooterIndex]
	if !combat.CanUseWeapon(*shooter, payload.Weapon) {
		return false
	}

	attackRange := combat.WeaponRangeTiles(payload.Weapon)
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

	shooterLabel := shooter.Name
	if shooterLabel == "" {
		shooterLabel = string(shooter.ID)
	}
	targetLabel := target.Name
	if targetLabel == "" {
		targetLabel = string(target.ID)
	}

	damageHalf, ok := combat.ConsumeShotCostAndResolveDamage(shooter, payload.Weapon, payload.UseGoldenRound)
	if !ok {
		return false
	}
	changedPlayers[shooterID] = struct{}{}

	if payload.Weapon == combat.WeaponBaton {
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
		targetFeedbackLevel = model.ActionFeedbackLevelError
		targetFeedbackMessage = fmt.Sprintf("Eliminated by %s.", shooterLabel)
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
		combat.IsFirearm(payload.Weapon) &&
		combat.IsUnjustAuthorityShot(*target, session.gameState.Map) {
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
