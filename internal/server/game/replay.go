package game

import (
	"encoding/json"
	"time"

	"prison-break/internal/shared/model"
)

type ReplayEntry struct {
	AcceptedTick uint64             `json:"accepted_tick"`
	IngressSeq   uint64             `json:"ingress_seq"`
	AcceptedAt   time.Time          `json:"accepted_at"`
	Command      model.InputCommand `json:"command"`
}

type MatchReplay struct {
	MatchID     model.MatchID     `json:"match_id"`
	Status      model.MatchStatus `json:"status"`
	TickRateHz  uint32            `json:"tick_rate_hz"`
	CreatedAt   time.Time         `json:"created_at"`
	StartedAt   *time.Time        `json:"started_at,omitempty"`
	EndedAt     *time.Time        `json:"ended_at,omitempty"`
	EndedReason string            `json:"ended_reason,omitempty"`
	Entries     []ReplayEntry     `json:"entries"`
}

func (m *Manager) ReplayLog(matchID model.MatchID) (MatchReplay, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.matches[matchID]
	if !exists {
		return MatchReplay{}, ErrMatchNotFound
	}

	entries := make([]ReplayEntry, len(session.replayEntries))
	for idx := range session.replayEntries {
		entry := session.replayEntries[idx]
		entry.Command = cloneInputCommand(entry.Command)
		entries[idx] = entry
	}

	log := MatchReplay{
		MatchID:     session.matchID,
		Status:      session.status,
		TickRateHz:  session.tickRateHz,
		CreatedAt:   session.createdAt,
		EndedReason: session.endedReason,
		Entries:     entries,
	}
	if session.startedAt != nil {
		startedAt := *session.startedAt
		log.StartedAt = &startedAt
	}
	if session.endedAt != nil {
		endedAt := *session.endedAt
		log.EndedAt = &endedAt
	}

	return log, nil
}

func cloneInputCommand(in model.InputCommand) model.InputCommand {
	out := in
	if in.Payload != nil {
		out.Payload = append(json.RawMessage(nil), in.Payload...)
	}
	return out
}
