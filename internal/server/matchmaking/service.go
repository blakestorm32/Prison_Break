package matchmaking

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"prison-break/internal/server/game"
	"prison-break/internal/shared/model"
)

const (
	DefaultRegionID = "global"

	MaxRegionIDLength       = 24
	MaxRegionLatencyEntries = 12
	MaxRegionLatencyMS      = 2000
)

type manager interface {
	CreateMatch() game.MatchSnapshot
	ListMatchSnapshots() []game.MatchSnapshot
	MatchConstraints() (uint8, uint8)
}

type Lobby struct {
	MatchID      model.MatchID     `json:"match_id"`
	Region       string            `json:"region"`
	Status       model.MatchStatus `json:"status"`
	PlayerCount  uint8             `json:"player_count"`
	MinPlayers   uint8             `json:"min_players"`
	MaxPlayers   uint8             `json:"max_players"`
	OpenSlots    uint8             `json:"open_slots"`
	Joinable     bool              `json:"joinable"`
	ReadyToStart bool              `json:"ready_to_start"`
	CreatedAt    time.Time         `json:"created_at"`
}

type QueueRequest struct {
	PlayerID        model.PlayerID    `json:"player_id,omitempty"`
	PreferredRegion string            `json:"preferred_region,omitempty"`
	RegionLatencyMS map[string]uint16 `json:"region_latency_ms,omitempty"`
	ExcludeMatchIDs []model.MatchID   `json:"exclude_match_ids,omitempty"`
}

type QueueMetrics struct {
	QueuedTotal    uint64 `json:"queued_total"`
	AllocatedTotal uint64 `json:"allocated_total"`
	CurrentDepth   int    `json:"current_depth"`
	MaxDepth       int    `json:"max_depth"`
}

type QueueEntrySnapshot struct {
	TicketID        string         `json:"ticket_id"`
	PlayerID        model.PlayerID `json:"player_id,omitempty"`
	PreferredRegion string         `json:"preferred_region,omitempty"`
	EnqueuedAt      time.Time      `json:"enqueued_at"`
	QueuePosition   int            `json:"queue_position"`
}

type queueEntry struct {
	ticketID        string
	playerID        model.PlayerID
	preferredRegion string
	regionLatencyMS map[string]uint16
	excludeMatchIDs map[model.MatchID]struct{}
	enqueuedAt      time.Time
}

type Service struct {
	manager manager

	mu                 sync.Mutex
	now                func() time.Time
	nextTicketSequence uint64
	queue              []queueEntry
	matchRegions       map[model.MatchID]string
	queueMetrics       QueueMetrics
}

func NewService(manager manager) *Service {
	return newServiceWithClock(manager, time.Now)
}

func newServiceWithClock(manager manager, now func() time.Time) *Service {
	if manager == nil {
		panic("matchmaking: manager is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Service{
		manager:      manager,
		now:          now,
		queue:        make([]queueEntry, 0, 64),
		matchRegions: make(map[model.MatchID]string),
	}
}

func (s *Service) FindOrCreateLobby() Lobby {
	return s.FindOrCreateLobbyForRequest(QueueRequest{})
}

func (s *Service) FindOrCreateLobbyForRequest(request QueueRequest) Lobby {
	s.mu.Lock()
	defer s.mu.Unlock()

	normalized := normalizeQueueRequest(request)
	entry := s.enqueueRequestLocked(normalized)

	lobbies := s.listLobbiesLocked(false, normalized)
	if normalized.PreferredRegion != "" {
		for _, lobby := range lobbies {
			if !lobby.Joinable {
				continue
			}
			if lobby.Region != normalized.PreferredRegion {
				continue
			}
			if _, blocked := entry.excludeMatchIDs[lobby.MatchID]; blocked {
				continue
			}
			s.dequeueByTicketIDLocked(entry.ticketID)
			s.queueMetrics.AllocatedTotal++
			return lobby
		}

		region := chooseCreationRegion(normalized)
		minPlayers, maxPlayers := s.manager.MatchConstraints()
		created := s.manager.CreateMatch()
		s.matchRegions[created.MatchID] = region
		s.dequeueByTicketIDLocked(entry.ticketID)
		s.queueMetrics.AllocatedTotal++
		return lobbyFromMatchSnapshot(created, minPlayers, maxPlayers, region)
	}

	for _, lobby := range lobbies {
		if !lobby.Joinable {
			continue
		}
		if _, blocked := entry.excludeMatchIDs[lobby.MatchID]; blocked {
			continue
		}
		s.dequeueByTicketIDLocked(entry.ticketID)
		s.queueMetrics.AllocatedTotal++
		return lobby
	}

	region := chooseCreationRegion(normalized)
	minPlayers, maxPlayers := s.manager.MatchConstraints()
	created := s.manager.CreateMatch()
	s.matchRegions[created.MatchID] = region
	s.dequeueByTicketIDLocked(entry.ticketID)
	s.queueMetrics.AllocatedTotal++
	return lobbyFromMatchSnapshot(created, minPlayers, maxPlayers, region)
}

func (s *Service) ListLobbies(includeRunning bool) []Lobby {
	return s.ListLobbiesForRequest(includeRunning, QueueRequest{})
}

func (s *Service) ListLobbiesForRequest(includeRunning bool, request QueueRequest) []Lobby {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.listLobbiesLocked(includeRunning, normalizeQueueRequest(request))
}

func (s *Service) QueueMetrics() QueueMetrics {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := s.queueMetrics
	out.CurrentDepth = len(s.queue)
	return out
}

func (s *Service) QueueSnapshot() []QueueEntrySnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]QueueEntrySnapshot, 0, len(s.queue))
	for idx, entry := range s.queue {
		out = append(out, QueueEntrySnapshot{
			TicketID:        entry.ticketID,
			PlayerID:        entry.playerID,
			PreferredRegion: entry.preferredRegion,
			EnqueuedAt:      entry.enqueuedAt,
			QueuePosition:   idx + 1,
		})
	}
	return out
}

func (s *Service) MatchRegion(matchID model.MatchID) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.regionForMatchLocked(matchID)
}

func (s *Service) enqueueRequestLocked(request QueueRequest) queueEntry {
	s.nextTicketSequence++

	entry := queueEntry{
		ticketID:        fmt.Sprintf("queue-%06d", s.nextTicketSequence),
		playerID:        request.PlayerID,
		preferredRegion: request.PreferredRegion,
		regionLatencyMS: cloneRegionLatencyMap(request.RegionLatencyMS),
		excludeMatchIDs: make(map[model.MatchID]struct{}, len(request.ExcludeMatchIDs)),
		enqueuedAt:      s.now().UTC(),
	}
	for _, matchID := range request.ExcludeMatchIDs {
		trimmed := model.MatchID(strings.TrimSpace(string(matchID)))
		if trimmed == "" {
			continue
		}
		entry.excludeMatchIDs[trimmed] = struct{}{}
	}

	// Keep one active queue entry per player to prevent stale preference buildup.
	if entry.playerID != "" {
		compacted := s.queue[:0]
		for _, existing := range s.queue {
			if existing.playerID == entry.playerID {
				continue
			}
			compacted = append(compacted, existing)
		}
		s.queue = compacted
	}

	s.queue = append(s.queue, entry)
	sort.SliceStable(s.queue, func(i int, j int) bool {
		if s.queue[i].enqueuedAt.Equal(s.queue[j].enqueuedAt) {
			return s.queue[i].ticketID < s.queue[j].ticketID
		}
		return s.queue[i].enqueuedAt.Before(s.queue[j].enqueuedAt)
	})

	s.queueMetrics.QueuedTotal++
	s.queueMetrics.CurrentDepth = len(s.queue)
	if len(s.queue) > s.queueMetrics.MaxDepth {
		s.queueMetrics.MaxDepth = len(s.queue)
	}

	return entry
}

func (s *Service) dequeueByTicketIDLocked(ticketID string) {
	if ticketID == "" || len(s.queue) == 0 {
		return
	}

	compacted := s.queue[:0]
	for _, entry := range s.queue {
		if entry.ticketID == ticketID {
			continue
		}
		compacted = append(compacted, entry)
	}
	s.queue = compacted
	s.queueMetrics.CurrentDepth = len(s.queue)
}

func (s *Service) listLobbiesLocked(includeRunning bool, request QueueRequest) []Lobby {
	minPlayers, maxPlayers := s.manager.MatchConstraints()
	matchSnapshots := s.manager.ListMatchSnapshots()

	lobbies := make([]Lobby, 0, len(matchSnapshots))
	for _, snapshot := range matchSnapshots {
		region := s.regionForMatchLocked(snapshot.MatchID)
		lobby := lobbyFromMatchSnapshot(snapshot, minPlayers, maxPlayers, region)
		if !includeRunning && lobby.Status != model.MatchStatusLobby {
			continue
		}
		lobbies = append(lobbies, lobby)
	}

	sortLobbiesForRequest(lobbies, request)
	return lobbies
}

func (s *Service) regionForMatchLocked(matchID model.MatchID) string {
	if matchID == "" {
		return DefaultRegionID
	}
	if region, exists := s.matchRegions[matchID]; exists {
		return region
	}
	s.matchRegions[matchID] = DefaultRegionID
	return DefaultRegionID
}

func lobbyFromMatchSnapshot(snapshot game.MatchSnapshot, minPlayers uint8, maxPlayers uint8, region string) Lobby {
	playerCount := uint8(len(snapshot.Players))
	openSlots := uint8(0)
	if maxPlayers > playerCount {
		openSlots = maxPlayers - playerCount
	}

	return Lobby{
		MatchID:      snapshot.MatchID,
		Region:       region,
		Status:       snapshot.Status,
		PlayerCount:  playerCount,
		MinPlayers:   minPlayers,
		MaxPlayers:   maxPlayers,
		OpenSlots:    openSlots,
		Joinable:     snapshot.Status == model.MatchStatusLobby && openSlots > 0,
		ReadyToStart: playerCount >= minPlayers,
		CreatedAt:    snapshot.CreatedAt,
	}
}

func sortLobbiesForRequest(lobbies []Lobby, request QueueRequest) {
	normalizedPreferredRegion := request.PreferredRegion
	regionLatency := request.RegionLatencyMS

	sort.Slice(lobbies, func(i int, j int) bool {
		if lobbies[i].Joinable != lobbies[j].Joinable {
			return lobbies[i].Joinable
		}

		if normalizedPreferredRegion != "" {
			leftPreferred := lobbies[i].Region == normalizedPreferredRegion
			rightPreferred := lobbies[j].Region == normalizedPreferredRegion
			if leftPreferred != rightPreferred {
				return leftPreferred
			}
		}

		leftLatency, leftLatencyKnown := regionLatency[lobbies[i].Region]
		rightLatency, rightLatencyKnown := regionLatency[lobbies[j].Region]
		if leftLatencyKnown != rightLatencyKnown {
			return leftLatencyKnown
		}
		if leftLatencyKnown && rightLatencyKnown && leftLatency != rightLatency {
			return leftLatency < rightLatency
		}

		if lobbies[i].PlayerCount != lobbies[j].PlayerCount {
			return lobbies[i].PlayerCount > lobbies[j].PlayerCount
		}
		if !lobbies[i].CreatedAt.Equal(lobbies[j].CreatedAt) {
			return lobbies[i].CreatedAt.Before(lobbies[j].CreatedAt)
		}
		return lobbies[i].MatchID < lobbies[j].MatchID
	})
}

func chooseCreationRegion(request QueueRequest) string {
	if request.PreferredRegion != "" {
		return request.PreferredRegion
	}
	if len(request.RegionLatencyMS) == 0 {
		return DefaultRegionID
	}

	regions := make([]string, 0, len(request.RegionLatencyMS))
	for region := range request.RegionLatencyMS {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	bestRegion := DefaultRegionID
	bestLatency := uint16(MaxRegionLatencyMS + 1)
	for _, region := range regions {
		latency := request.RegionLatencyMS[region]
		if latency < bestLatency {
			bestLatency = latency
			bestRegion = region
		}
	}
	return bestRegion
}

func ValidateQueueRequest(request QueueRequest) error {
	if regionErr := validateRegionID(request.PreferredRegion); regionErr != nil {
		return regionErr
	}
	if len(request.RegionLatencyMS) > MaxRegionLatencyEntries {
		return fmt.Errorf("region_latency_ms exceeds %d entries", MaxRegionLatencyEntries)
	}
	for region, latency := range request.RegionLatencyMS {
		if regionErr := validateRegionID(region); regionErr != nil {
			return fmt.Errorf("region_latency_ms key %q: %w", region, regionErr)
		}
		if latency == 0 || latency > MaxRegionLatencyMS {
			return fmt.Errorf("region_latency_ms value for %q must be within 1-%d", region, MaxRegionLatencyMS)
		}
	}
	return nil
}

func normalizeQueueRequest(request QueueRequest) QueueRequest {
	out := QueueRequest{
		PlayerID:        model.PlayerID(strings.TrimSpace(string(request.PlayerID))),
		PreferredRegion: NormalizeRegionID(request.PreferredRegion),
		RegionLatencyMS: make(map[string]uint16, len(request.RegionLatencyMS)),
		ExcludeMatchIDs: make([]model.MatchID, 0, len(request.ExcludeMatchIDs)),
	}

	for region, latency := range request.RegionLatencyMS {
		normalizedRegion := NormalizeRegionID(region)
		if normalizedRegion == "" || latency == 0 || latency > MaxRegionLatencyMS {
			continue
		}
		out.RegionLatencyMS[normalizedRegion] = latency
	}

	for _, matchID := range request.ExcludeMatchIDs {
		trimmed := model.MatchID(strings.TrimSpace(string(matchID)))
		if trimmed == "" {
			continue
		}
		out.ExcludeMatchIDs = append(out.ExcludeMatchIDs, trimmed)
	}

	return out
}

func NormalizeRegionID(region string) string {
	trimmed := strings.ToLower(strings.TrimSpace(region))
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > MaxRegionIDLength {
		return ""
	}
	if !isSafeIdentifier(trimmed) {
		return ""
	}
	return trimmed
}

func validateRegionID(region string) error {
	trimmed := strings.TrimSpace(region)
	if trimmed == "" {
		return nil
	}
	normalized := NormalizeRegionID(trimmed)
	if normalized == "" {
		return fmt.Errorf("invalid region id")
	}
	return nil
}

func cloneRegionLatencyMap(input map[string]uint16) map[string]uint16 {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]uint16, len(input))
	for region, latency := range input {
		out[region] = latency
	}
	return out
}

func isSafeIdentifier(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
