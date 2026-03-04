package game

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"prison-break/internal/gamecore/abilities"
	"prison-break/internal/gamecore/cards"
	"prison-break/internal/gamecore/combat"
	"prison-break/internal/gamecore/escape"
	"prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/constants"
	"prison-break/internal/shared/model"
)

type InputSubmitResult struct {
	MatchID       model.MatchID  `json:"match_id"`
	PlayerID      model.PlayerID `json:"player_id"`
	ClientSeq     uint64         `json:"client_seq"`
	IngressSeq    uint64         `json:"ingress_seq"`
	ScheduledTick uint64         `json:"scheduled_tick"`
}

func (m *Manager) SubmitInput(matchID model.MatchID, command model.InputCommand) (InputSubmitResult, error) {
	playerID := model.PlayerID(strings.TrimSpace(string(command.PlayerID)))
	if playerID == "" || command.ClientSeq == 0 || command.Type == "" {
		return InputSubmitResult{}, ErrInvalidInputCommand
	}
	if !isSupportedInputCommand(command.Type) {
		return InputSubmitResult{}, ErrInvalidInputCommand
	}
	if err := validateInputPayload(command.Type, command.Payload); err != nil {
		return InputSubmitResult{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.matches[matchID]
	if !exists {
		return InputSubmitResult{}, ErrMatchNotFound
	}
	if session.status != model.MatchStatusRunning {
		return InputSubmitResult{}, ErrMatchNotRunning
	}

	if _, exists := session.players[playerID]; !exists {
		return InputSubmitResult{}, ErrInputPlayerMismatch
	}

	seenForPlayer, exists := session.seenClientSeq[playerID]
	if !exists {
		seenForPlayer = make(map[uint64]struct{})
		session.seenClientSeq[playerID] = seenForPlayer
	}
	if _, seen := seenForPlayer[command.ClientSeq]; seen {
		return InputSubmitResult{}, ErrDuplicateInput
	}

	currentTick := session.tickID
	windowCounters, exists := session.acceptedByTick[currentTick]
	if !exists {
		windowCounters = make(map[model.PlayerID]uint8)
		session.acceptedByTick[currentTick] = windowCounters
	}
	if windowCounters[playerID] >= constants.MaxAcceptedCommandsPerPlayerPerTick {
		return InputSubmitResult{}, ErrInputRateLimited
	}

	scheduledTick, scheduleErr := computeScheduledTick(currentTick, command.TargetTick, command.Type)
	if scheduleErr != nil {
		return InputSubmitResult{}, scheduleErr
	}

	session.nextIngressSeq++
	command.PlayerID = playerID
	command.IngressSeq = session.nextIngressSeq
	command.TargetTick = scheduledTick
	session.scheduledInputs[scheduledTick] = append(session.scheduledInputs[scheduledTick], command)
	session.replayEntries = append(session.replayEntries, ReplayEntry{
		AcceptedTick: scheduledTick,
		IngressSeq:   command.IngressSeq,
		AcceptedAt:   m.now().UTC(),
		Command:      cloneInputCommand(command),
	})

	seenForPlayer[command.ClientSeq] = struct{}{}
	windowCounters[playerID]++
	pruneRateWindows(session, currentTick)

	return InputSubmitResult{
		MatchID:       matchID,
		PlayerID:      playerID,
		ClientSeq:     command.ClientSeq,
		IngressSeq:    command.IngressSeq,
		ScheduledTick: scheduledTick,
	}, nil
}

func (m *Manager) ConsumeScheduledInputs(matchID model.MatchID, tickID uint64) ([]model.InputCommand, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.matches[matchID]
	if !exists {
		return nil, ErrMatchNotFound
	}

	scheduled := session.scheduledInputs[tickID]
	delete(session.scheduledInputs, tickID)
	sortInputCommands(scheduled)

	out := make([]model.InputCommand, len(scheduled))
	copy(out, scheduled)
	return out, nil
}

func (m *Manager) PendingInputCounts(matchID model.MatchID) (map[uint64]int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.matches[matchID]
	if !exists {
		return nil, ErrMatchNotFound
	}

	counts := make(map[uint64]int, len(session.scheduledInputs))
	for tickID, commands := range session.scheduledInputs {
		counts[tickID] = len(commands)
	}

	return counts, nil
}

func isSupportedInputCommand(commandType model.InputCommandType) bool {
	switch commandType {
	case model.CmdMoveIntent,
		model.CmdAimIntent,
		model.CmdInteract,
		model.CmdUseAbility,
		model.CmdUseCard,
		model.CmdUseItem,
		model.CmdBlackMarketBuy,
		model.CmdFireWeapon,
		model.CmdReload,
		model.CmdDropItem,
		model.CmdCraftItem:
		return true
	default:
		return false
	}
}

func isContinuousInput(commandType model.InputCommandType) bool {
	switch commandType {
	case model.CmdMoveIntent, model.CmdAimIntent:
		return true
	default:
		return false
	}
}

func computeScheduledTick(currentTick uint64, requestedTick uint64, commandType model.InputCommandType) (uint64, error) {
	earliest := currentTick + 1
	latest := currentTick + 2

	if requestedTick == 0 {
		return earliest, nil
	}

	if requestedTick < currentTick {
		if isContinuousInput(commandType) {
			return earliest, nil
		}
		return 0, ErrInputTooLateDropped
	}
	if requestedTick < earliest {
		return earliest, nil
	}
	if requestedTick > latest {
		return latest, nil
	}

	return requestedTick, nil
}

func pruneRateWindows(session *matchSession, currentTick uint64) {
	if currentTick <= 3 {
		return
	}

	cutoff := currentTick - 3
	for tickID := range session.acceptedByTick {
		if tickID < cutoff {
			delete(session.acceptedByTick, tickID)
		}
	}
}

func sortInputCommands(commands []model.InputCommand) {
	sort.SliceStable(commands, func(i int, j int) bool {
		left := commands[i]
		right := commands[j]

		if left.IngressSeq != right.IngressSeq {
			return left.IngressSeq < right.IngressSeq
		}
		if left.PlayerID != right.PlayerID {
			return left.PlayerID < right.PlayerID
		}
		if left.ClientSeq != right.ClientSeq {
			return left.ClientSeq < right.ClientSeq
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}

		return bytes.Compare(left.Payload, right.Payload) < 0
	})
}

func validateInputPayload(commandType model.InputCommandType, payload json.RawMessage) error {
	decode := func(target any) error {
		if len(payload) == 0 {
			return ErrInvalidInputPayload
		}
		if err := json.Unmarshal(payload, target); err != nil {
			return ErrInvalidInputPayload
		}
		return nil
	}

	switch commandType {
	case model.CmdMoveIntent:
		var msg model.MovementInputPayload
		return decode(&msg)
	case model.CmdAimIntent:
		var msg model.AimInputPayload
		return decode(&msg)
	case model.CmdInteract:
		var msg model.InteractPayload
		if err := decode(&msg); err != nil {
			return err
		}
		if msg.EscapeRoute != "" && !escape.IsKnownRoute(msg.EscapeRoute) {
			return ErrInvalidInputPayload
		}
		if msg.MarketRoomID != "" && !gamemap.IsNightlyBlackMarketCandidate(msg.MarketRoomID) {
			return ErrInvalidInputPayload
		}
		if msg.NightCardChoice != "" && !cards.IsKnownCard(msg.NightCardChoice) {
			return ErrInvalidInputPayload
		}
		if msg.StashAction != "" {
			action := strings.ToLower(strings.TrimSpace(msg.StashAction))
			if action != "deposit" && action != "withdraw" {
				return ErrInvalidInputPayload
			}
			if msg.StashItem == "" || !items.IsKnownItem(msg.StashItem) {
				return ErrInvalidInputPayload
			}
		}
		return nil
	case model.CmdUseAbility:
		var msg model.AbilityUsePayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Ability)) == "" {
			return ErrInvalidInputPayload
		}
		if !abilities.IsKnownAbility(msg.Ability) {
			return ErrInvalidInputPayload
		}
		if !validateAbilityUsePayload(msg) {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdUseCard:
		var msg model.CardUsePayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Card)) == "" {
			return ErrInvalidInputPayload
		}
		if !cards.IsKnownCard(msg.Card) {
			return ErrInvalidInputPayload
		}
		if !validateCardUsePayload(msg) {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdUseItem:
		var msg model.ItemUsePayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Item)) == "" {
			return ErrInvalidInputPayload
		}
		if !items.IsKnownItem(msg.Item) {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdBlackMarketBuy:
		var msg model.BlackMarketPurchasePayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Item)) == "" {
			return ErrInvalidInputPayload
		}
		if _, exists := items.BlackMarketOfferForItem(msg.Item); !exists {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdFireWeapon:
		var msg model.FireWeaponPayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Weapon)) == "" {
			return ErrInvalidInputPayload
		}
		if !combat.IsSupportedWeapon(msg.Weapon) {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdDropItem:
		var msg model.DropItemPayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Item)) == "" {
			return ErrInvalidInputPayload
		}
		if !items.IsKnownItem(msg.Item) {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdCraftItem:
		var msg model.CraftItemPayload
		if err := decode(&msg); err != nil {
			return err
		}
		if strings.TrimSpace(string(msg.Item)) == "" {
			return ErrInvalidInputPayload
		}
		if _, exists := items.RecipeFor(msg.Item); !exists {
			return ErrInvalidInputPayload
		}
		return nil
	case model.CmdReload:
		// Reload supports empty payload.
		return nil
	default:
		return ErrInvalidInputCommand
	}
}

func validateAbilityUsePayload(payload model.AbilityUsePayload) bool {
	return strings.TrimSpace(string(payload.Ability)) != ""
}

func validateCardUsePayload(payload model.CardUsePayload) bool {
	if payload.TargetItem != "" && !items.IsKnownItem(payload.TargetItem) {
		return false
	}

	switch payload.Card {
	case model.CardLockSnap, model.CardDoorStop:
		return payload.TargetDoorID != 0
	case model.CardItemSteal, model.CardItemGrab:
		return strings.TrimSpace(string(payload.TargetPlayerID)) != ""
	default:
		return true
	}
}
