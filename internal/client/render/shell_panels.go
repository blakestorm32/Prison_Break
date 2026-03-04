package render

import (
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"

	"prison-break/internal/client/input"
	"prison-break/internal/gamecore/abilities"
	"prison-break/internal/gamecore/cards"
	"prison-break/internal/gamecore/escape"
	gameitems "prison-break/internal/gamecore/items"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

type actionPanelMode uint8

const (
	actionPanelNone actionPanelMode = iota
	actionPanelInventory
	actionPanelCards
	actionPanelAbilities
	actionPanelMarket
	actionPanelEscape
	actionPanelNightCards
	actionPanelStash
)

type panelInputEdgeState struct {
	inventory bool
	cards     bool
	abilities bool
	market    bool
	escape    bool
	stash     bool
	interact  bool
	prev      bool
	next      bool
	use       bool
}

type actionPanelLayout struct {
	inventoryTab input.Rect
	cardsTab     input.Rect
	abilitiesTab input.Rect
	marketTab    input.Rect
	escapeTab    input.Rect
	stashTab     input.Rect
	prevButton   input.Rect
	nextButton   input.Rect
	useButton    input.Rect
}

type actionPanelAvailability struct {
	inventory bool
	cards     bool
	abilities bool
	market    bool
	escape    bool
	stash     bool
	nightCards bool
}

func (s *Shell) augmentSnapshotWithPanelTouches(snapshot input.InputSnapshot) input.InputSnapshot {
	if s == nil || len(snapshot.Touches) == 0 {
		return snapshot
	}

	layout := s.actionPanelLayout()
	if touchInsideRect(snapshot.Touches, layout.inventoryTab) {
		snapshot.PanelInventoryPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.cardsTab) {
		snapshot.PanelCardsPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.abilitiesTab) {
		snapshot.PanelAbilitiesPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.marketTab) {
		snapshot.PanelMarketPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.escapeTab) {
		snapshot.PanelEscapePressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.stashTab) {
		snapshot.PanelStashPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.prevButton) {
		snapshot.PanelPrevPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.nextButton) {
		snapshot.PanelNextPressed = true
	}
	if touchInsideRect(snapshot.Touches, layout.useButton) {
		snapshot.PanelUsePressed = true
	}
	return snapshot
}

func touchInsideRect(touches []input.TouchPoint, rect input.Rect) bool {
	if len(touches) == 0 {
		return false
	}
	for _, touch := range touches {
		if rect.Contains(touch.X, touch.Y) {
			return true
		}
	}
	return false
}

func (s *Shell) updateActionPanelCommands(
	snapshot input.InputSnapshot,
	state model.GameState,
	localPlayer *model.PlayerState,
) []model.InputCommand {
	if s == nil || s.inputController == nil {
		return nil
	}

	interactPressed := snapshot.InteractPressed
	mobileLayout := s.inputController.MobileLayout()
	if mobileLayout.Enabled && touchInsideRect(snapshot.Touches, mobileLayout.InteractButton) {
		interactPressed = true
	}

	current := panelInputEdgeState{
		inventory: snapshot.PanelInventoryPressed,
		cards:     snapshot.PanelCardsPressed,
		abilities: snapshot.PanelAbilitiesPressed,
		market:    snapshot.PanelMarketPressed,
		escape:    snapshot.PanelEscapePressed,
		stash:     snapshot.PanelStashPressed,
		interact:  interactPressed,
		prev:      snapshot.PanelPrevPressed,
		next:      snapshot.PanelNextPressed,
		use:       snapshot.PanelUsePressed,
	}
	previous := s.panelInputPrev
	s.panelInputPrev = current

	inventoryEdge := current.inventory && !previous.inventory
	cardsEdge := current.cards && !previous.cards
	abilitiesEdge := current.abilities && !previous.abilities
	marketEdge := current.market && !previous.market
	escapeEdge := current.escape && !previous.escape
	stashEdge := current.stash && !previous.stash
	interactEdge := current.interact && !previous.interact
	prevEdge := current.prev && !previous.prev
	nextEdge := current.next && !previous.next
	useEdge := current.use && !previous.use

	if localPlayer == nil {
		s.panelMode = actionPanelNone
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
		return nil
	}

	availability := computeActionPanelAvailability(*localPlayer, state)

	if inventoryEdge && availability.inventory {
		s.toggleActionPanel(actionPanelInventory)
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
	}
	if cardsEdge && availability.cards {
		s.toggleActionPanel(actionPanelCards)
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
	}
	if abilitiesEdge && availability.abilities {
		s.toggleActionPanel(actionPanelAbilities)
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
	}
	if stashEdge {
		if availability.stash {
			s.toggleActionPanel(actionPanelStash)
			s.panelLocalHint = ""
			s.panelLocalHintWarning = false
		} else {
			s.panelLocalHint = "Go to your cell block to access stash."
			s.panelLocalHintWarning = true
		}
	}
	if marketEdge {
		s.panelLocalHint = marketAccessInstruction(state.Map.BlackMarketRoomID)
		s.panelLocalHintWarning = true
	}
	if escapeEdge {
		if s.panelMode == actionPanelMarket {
			s.panelMode = actionPanelNone
		} else if availability.escape {
			s.toggleActionPanel(actionPanelEscape)
			s.panelLocalHint = ""
			s.panelLocalHintWarning = false
		}
	}
	if interactEdge && shouldHandleMarketInteract(*localPlayer, state) {
		s.panelSuppressInteract = true
		if canOpen, reason := canOpenMarketPanel(*localPlayer, state); canOpen {
			s.panelMode = actionPanelMarket
			s.panelLocalHint = ""
			s.panelLocalHintWarning = false
		} else {
			s.panelLocalHint = reason
			s.panelLocalHintWarning = true
		}
	}
	if availability.nightCards {
		s.panelMode = actionPanelNightCards
	}

	if !isPanelModeAvailable(s.panelMode, availability) {
		s.panelMode = actionPanelNone
		return nil
	}
	if s.panelMode == actionPanelNone {
		return nil
	}
	if actionPanelUsesCenteredModal(s.panelMode) {
		s.panelSuppressGameplay = true
	}

	targetTick := state.TickID + 1
	switch s.panelMode {
	case actionPanelInventory:
		entries := inventoryPanelEntries(*localPlayer)
		if len(entries) == 0 {
			s.panelInventoryIdx = 0
			return nil
		}
		s.panelInventoryIdx = clampPanelIndex(s.panelInventoryIdx, len(entries))
		if prevEdge {
			s.panelInventoryIdx = wrapPanelIndex(s.panelInventoryIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelInventoryIdx = wrapPanelIndex(s.panelInventoryIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}

		command, ok := s.inputController.BuildUseItemCommand(model.ItemUsePayload{
			Item:   entries[s.panelInventoryIdx].Item,
			Amount: 1,
		}, targetTick)
		if !ok {
			return nil
		}
		return []model.InputCommand{command}

	case actionPanelCards:
		entries := cardPanelEntries(*localPlayer)
		if len(entries) == 0 {
			s.panelCardsIdx = 0
			return nil
		}
		s.panelCardsIdx = clampPanelIndex(s.panelCardsIdx, len(entries))
		if prevEdge {
			s.panelCardsIdx = wrapPanelIndex(s.panelCardsIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelCardsIdx = wrapPanelIndex(s.panelCardsIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}

		payload := cardPayloadForPanel(entries[s.panelCardsIdx], *localPlayer, state)
		command, ok := s.inputController.BuildUseCardCommand(payload, targetTick)
		if !ok {
			return nil
		}
		return []model.InputCommand{command}

	case actionPanelAbilities:
		entries := abilityPanelEntries(*localPlayer)
		if len(entries) == 0 {
			s.panelAbilitiesIdx = 0
			return nil
		}
		s.panelAbilitiesIdx = clampPanelIndex(s.panelAbilitiesIdx, len(entries))
		if prevEdge {
			s.panelAbilitiesIdx = wrapPanelIndex(s.panelAbilitiesIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelAbilitiesIdx = wrapPanelIndex(s.panelAbilitiesIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}

		payload := abilityPayloadForPanel(entries[s.panelAbilitiesIdx], *localPlayer, state)
		command, ok := s.inputController.BuildUseAbilityCommand(payload, targetTick)
		if !ok {
			return nil
		}
		return []model.InputCommand{command}

	case actionPanelMarket:
		entries := marketPanelEntries()
		if len(entries) == 0 {
			s.panelMarketIdx = 0
			return nil
		}
		s.panelMarketIdx = clampPanelIndex(s.panelMarketIdx, len(entries))
		if prevEdge {
			s.panelMarketIdx = wrapPanelIndex(s.panelMarketIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelMarketIdx = wrapPanelIndex(s.panelMarketIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}

		offer := entries[s.panelMarketIdx]
		if canBuy, reason := marketOfferUsability(*localPlayer, state, offer); !canBuy {
			s.panelLocalHint = reason
			s.panelLocalHintWarning = true
			return nil
		}
		command, ok := s.inputController.BuildBlackMarketBuyCommand(model.BlackMarketPurchasePayload{
			Item: offer.Item,
		}, targetTick)
		if !ok {
			s.panelLocalHint = "Unable to build market purchase command."
			s.panelLocalHintWarning = true
			return nil
		}
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
		return []model.InputCommand{command}

	case actionPanelEscape:
		entries := escapePanelEntries(*localPlayer, state.Map)
		if len(entries) == 0 {
			s.panelEscapeIdx = 0
			return nil
		}
		s.panelEscapeIdx = clampPanelIndex(s.panelEscapeIdx, len(entries))
		if prevEdge {
			s.panelEscapeIdx = wrapPanelIndex(s.panelEscapeIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelEscapeIdx = wrapPanelIndex(s.panelEscapeIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}
		selected := entries[s.panelEscapeIdx]
		if !selected.CanAttempt {
			reason := selected.FailureReason
			if reason == "" {
				reason = fmt.Sprintf("%s is not ready yet.", selected.RouteLabel)
			}
			s.panelLocalHint = reason
			s.panelLocalHintWarning = true
			return nil
		}
		command, ok := s.inputController.BuildInteractCommand(model.InteractPayload{
			EscapeRoute: selected.Route,
		}, targetTick)
		if !ok {
			s.panelLocalHint = "Unable to build escape command."
			s.panelLocalHintWarning = true
			return nil
		}
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
		return []model.InputCommand{command}

	case actionPanelNightCards:
		entries := nightCardPanelEntries(*localPlayer)
		if len(entries) == 0 {
			s.panelNightCardsIdx = 0
			return nil
		}
		s.panelNightCardsIdx = clampPanelIndex(s.panelNightCardsIdx, len(entries))
		if prevEdge {
			s.panelNightCardsIdx = wrapPanelIndex(s.panelNightCardsIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelNightCardsIdx = wrapPanelIndex(s.panelNightCardsIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}
		command, ok := s.inputController.BuildInteractCommand(model.InteractPayload{
			NightCardChoice: entries[s.panelNightCardsIdx],
		}, targetTick)
		if !ok {
			s.panelLocalHint = "Unable to select night card."
			s.panelLocalHintWarning = true
			return nil
		}
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
		return []model.InputCommand{command}

	case actionPanelStash:
		entries := stashPanelEntries(*localPlayer, state.Map)
		if len(entries) == 0 {
			s.panelStashIdx = 0
			return nil
		}
		s.panelStashIdx = clampPanelIndex(s.panelStashIdx, len(entries))
		if prevEdge {
			s.panelStashIdx = wrapPanelIndex(s.panelStashIdx, len(entries), -1)
		}
		if nextEdge {
			s.panelStashIdx = wrapPanelIndex(s.panelStashIdx, len(entries), 1)
		}
		if !useEdge {
			return nil
		}

		selected := entries[s.panelStashIdx]
		command, ok := s.inputController.BuildInteractCommand(model.InteractPayload{
			StashAction: selected.Action,
			StashItem:   selected.Item,
			StashAmount: 1,
		}, targetTick)
		if !ok {
			s.panelLocalHint = "Unable to submit stash action."
			s.panelLocalHintWarning = true
			return nil
		}
		s.panelLocalHint = ""
		s.panelLocalHintWarning = false
		return []model.InputCommand{command}
	}

	return nil
}

func (s *Shell) toggleActionPanel(next actionPanelMode) {
	if s.panelMode == next {
		s.panelMode = actionPanelNone
		return
	}
	s.panelMode = next
}

func actionPanelUsesCenteredModal(mode actionPanelMode) bool {
	return mode == actionPanelMarket || mode == actionPanelEscape || mode == actionPanelNightCards || mode == actionPanelStash
}

func shouldHandleMarketInteract(local model.PlayerState, state model.GameState) bool {
	if !gamemap.IsPrisonerPlayer(local) {
		return false
	}
	if state.Map.BlackMarketRoomID == "" {
		return false
	}
	return local.CurrentRoomID != "" && local.CurrentRoomID == state.Map.BlackMarketRoomID
}

func canOpenMarketPanel(local model.PlayerState, state model.GameState) (bool, string) {
	if !local.Alive {
		return false, "You are down."
	}
	if !gamemap.IsPrisonerPlayer(local) {
		return false, "Only prisoners can access the black market."
	}
	if state.Map.BlackMarketRoomID == "" {
		return false, "Black-market location is not set this cycle."
	}
	if local.CurrentRoomID != state.Map.BlackMarketRoomID {
		return false, fmt.Sprintf("Go to %s and press Interact (E/F).", roomDisplayLabel(state.Map.BlackMarketRoomID))
	}
	if state.Phase.Current != model.PhaseNight {
		return false, "Black market opens at night."
	}
	return true, ""
}

func marketAccessInstruction(marketRoomID model.RoomID) string {
	if marketRoomID == "" {
		return "Black-market location is not set."
	}
	return fmt.Sprintf("Go to %s at night and press Interact (E/F) to buy.", roomDisplayLabel(marketRoomID))
}

func computeActionPanelAvailability(local model.PlayerState, state model.GameState) actionPanelAvailability {
	prisonerAccess := gamemap.IsPrisonerPlayer(local)
	cellOwner := local.AssignedCell != 0 && local.CurrentRoomID == gamemap.RoomCellBlockA
	return actionPanelAvailability{
		inventory: len(inventoryPanelEntries(local)) > 0,
		cards:     len(cardPanelEntries(local)) > 0,
		abilities: len(abilityPanelEntries(local)) > 0,
		market:    prisonerAccess,
		escape:    prisonerAccess,
		stash:     cellOwner,
		nightCards: len(local.NightCardChoices) > 0 && state.Phase.Current == model.PhaseNight,
	}
}

func isPanelModeAvailable(mode actionPanelMode, availability actionPanelAvailability) bool {
	switch mode {
	case actionPanelNone:
		return true
	case actionPanelInventory:
		return availability.inventory
	case actionPanelCards:
		return availability.cards
	case actionPanelAbilities:
		return availability.abilities
	case actionPanelMarket:
		return availability.market
	case actionPanelEscape:
		return availability.escape
	case actionPanelNightCards:
		return availability.nightCards
	case actionPanelStash:
		return availability.stash
	default:
		return false
	}
}

func clampPanelIndex(index int, count int) int {
	if count <= 0 {
		return 0
	}
	if index < 0 {
		return 0
	}
	if index >= count {
		return count - 1
	}
	return index
}

func wrapPanelIndex(index int, count int, delta int) int {
	if count <= 0 || delta == 0 {
		return clampPanelIndex(index, count)
	}
	next := index + delta
	for next < 0 {
		next += count
	}
	return next % count
}

func inventoryPanelEntries(player model.PlayerState) []model.ItemStack {
	if len(player.Inventory) == 0 {
		return nil
	}
	out := make([]model.ItemStack, 0, len(player.Inventory))
	for _, stack := range player.Inventory {
		if stack.Item == "" || stack.Quantity == 0 {
			continue
		}
		out = append(out, stack)
	}
	return out
}

func cardPanelEntries(player model.PlayerState) []model.CardType {
	if len(player.Cards) == 0 {
		return nil
	}
	out := make([]model.CardType, 0, len(player.Cards))
	for _, card := range player.Cards {
		if card == "" {
			continue
		}
		out = append(out, card)
	}
	return out
}

func abilityPanelEntries(player model.PlayerState) []model.AbilityType {
	if player.AssignedAbility != "" {
		if abilities.IsKnownAbility(player.AssignedAbility) && abilities.CanPlayerUse(player, player.AssignedAbility) {
			return []model.AbilityType{player.AssignedAbility}
		}
		return nil
	}

	catalog := []model.AbilityType{
		model.AbilityAlarm,
		model.AbilitySearch,
		model.AbilityCameraMan,
		model.AbilityDetainer,
		model.AbilityTracker,
		model.AbilityPickPocket,
		model.AbilityHacker,
		model.AbilityDisguise,
		model.AbilityLocksmith,
		model.AbilityChameleon,
	}

	out := make([]model.AbilityType, 0, len(catalog))
	for _, ability := range catalog {
		if !abilities.CanPlayerUse(player, ability) {
			continue
		}
		out = append(out, ability)
	}
	return out
}

func marketPanelEntries() []gameitems.BlackMarketOffer {
	return gameitems.BlackMarketCatalog()
}

func escapePanelEntries(local model.PlayerState, mapState model.MapState) []escape.RouteEvaluation {
	return escape.EvaluateAllRoutes(local, mapState)
}

func nightCardPanelEntries(player model.PlayerState) []model.CardType {
	if len(player.NightCardChoices) == 0 {
		return nil
	}
	out := append([]model.CardType(nil), player.NightCardChoices...)
	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})
	return out
}

type stashPanelEntry struct {
	Action string
	Item   model.ItemType
	Count  uint8
}

func stashPanelEntries(local model.PlayerState, mapState model.MapState) []stashPanelEntry {
	if local.AssignedCell == 0 {
		return nil
	}
	entries := make([]stashPanelEntry, 0, len(local.Inventory)+6)
	for _, stack := range local.Inventory {
		if stack.Item == "" || stack.Quantity == 0 {
			continue
		}
		entries = append(entries, stashPanelEntry{
			Action: "deposit",
			Item:   stack.Item,
			Count:  stack.Quantity,
		})
	}

	stash := stashForAssignedCell(local, mapState)
	for _, stack := range stash {
		if stack.Item == "" || stack.Quantity == 0 {
			continue
		}
		entries = append(entries, stashPanelEntry{
			Action: "withdraw",
			Item:   stack.Item,
			Count:  stack.Quantity,
		})
	}

	sort.Slice(entries, func(i int, j int) bool {
		if entries[i].Action != entries[j].Action {
			return entries[i].Action < entries[j].Action
		}
		if entries[i].Item != entries[j].Item {
			return entries[i].Item < entries[j].Item
		}
		return entries[i].Count < entries[j].Count
	})
	return entries
}

func stashForAssignedCell(local model.PlayerState, mapState model.MapState) []model.ItemStack {
	if local.AssignedCell == 0 {
		return nil
	}
	for _, cell := range mapState.Cells {
		if cell.ID != local.AssignedCell {
			continue
		}
		return append([]model.ItemStack(nil), cell.Stash...)
	}
	return nil
}

func abilityPayloadForPanel(ability model.AbilityType, local model.PlayerState, state model.GameState) model.AbilityUsePayload {
	payload := model.AbilityUsePayload{
		Ability: ability,
	}

	switch ability {
	case model.AbilitySearch, model.AbilityDetainer, model.AbilityTracker, model.AbilityPickPocket:
		payload.TargetPlayerID = targetPlayerForPanel(local, state.Players)
	case model.AbilityLocksmith:
		payload.TargetDoorID = targetDoorForPanel(local, state.Map)
	}

	return payload
}

func cardPayloadForPanel(card model.CardType, local model.PlayerState, state model.GameState) model.CardUsePayload {
	payload := model.CardUsePayload{
		Card: card,
	}

	switch card {
	case model.CardLockSnap, model.CardDoorStop:
		payload.TargetDoorID = targetDoorForPanel(local, state.Map)
	case model.CardItemSteal, model.CardItemGrab:
		payload.TargetPlayerID = targetPlayerForPanel(local, state.Players)
	case model.CardMoney:
		payload.TargetEntityID = targetNPCPrisonerForPanel(local, state.Entities)
	}

	return payload
}

func targetNPCPrisonerForPanel(local model.PlayerState, entities []model.EntityState) model.EntityID {
	var (
		bestID   model.EntityID
		bestDist float32
	)

	for _, candidate := range entities {
		if candidate.ID == 0 || candidate.Kind != model.EntityKindNPCPrisoner || !candidate.Active {
			continue
		}
		if local.CurrentRoomID != "" && candidate.RoomID != "" && candidate.RoomID != local.CurrentRoomID {
			continue
		}

		dx := candidate.Position.X - local.Position.X
		dy := candidate.Position.Y - local.Position.Y
		distanceSq := (dx * dx) + (dy * dy)
		if bestID == 0 || distanceSq < bestDist {
			bestID = candidate.ID
			bestDist = distanceSq
		}
	}
	return bestID
}

func targetPlayerForPanel(local model.PlayerState, players []model.PlayerState) model.PlayerID {
	var (
		bestID   model.PlayerID
		bestDist float32
	)

	for _, candidate := range players {
		if candidate.ID == "" || candidate.ID == local.ID || !candidate.Alive {
			continue
		}

		if local.CurrentRoomID != "" && candidate.CurrentRoomID != "" && candidate.CurrentRoomID != local.CurrentRoomID {
			continue
		}

		dx := candidate.Position.X - local.Position.X
		dy := candidate.Position.Y - local.Position.Y
		distanceSq := (dx * dx) + (dy * dy)
		if bestID == "" || distanceSq < bestDist {
			bestID = candidate.ID
			bestDist = distanceSq
		}
	}
	return bestID
}

func targetDoorForPanel(local model.PlayerState, mapState model.MapState) model.DoorID {
	if len(mapState.Doors) == 0 {
		return 0
	}

	for _, door := range mapState.Doors {
		if door.ID == 0 {
			continue
		}
		if canViewerTraverseDoor(local, door, mapState) {
			return door.ID
		}
	}
	return 0
}

func marketOfferUsability(local model.PlayerState, state model.GameState, offer gameitems.BlackMarketOffer) (bool, string) {
	if !local.Alive {
		return false, "You are down."
	}
	if !gamemap.IsPrisonerPlayer(local) {
		return false, "Only prisoners can buy."
	}
	if state.Phase.Current != model.PhaseNight {
		return false, "Market opens at night."
	}
	if state.Map.BlackMarketRoomID == "" {
		return false, "Market room not set."
	}
	if local.CurrentRoomID != state.Map.BlackMarketRoomID {
		return false, fmt.Sprintf("Go to %s.", state.Map.BlackMarketRoomID)
	}
	if offer.Item == model.ItemGoldenBullet && hasInventoryItem(local.Inventory, model.ItemGoldenBullet) {
		return false, "You already carry the golden bullet."
	}

	moneyCards := countCards(local.Cards, model.CardMoney)
	if moneyCards < int(offer.MoneyCardCost) {
		return false, fmt.Sprintf("Need %d money cards.", offer.MoneyCardCost)
	}
	return true, "Purchase ready."
}

func countCards(cards []model.CardType, target model.CardType) int {
	if len(cards) == 0 || target == "" {
		return 0
	}

	count := 0
	for _, card := range cards {
		if card == target {
			count++
		}
	}
	return count
}

func hasInventoryItem(inventory []model.ItemStack, target model.ItemType) bool {
	for _, stack := range inventory {
		if stack.Item == target && stack.Quantity > 0 {
			return true
		}
	}
	return false
}

func requirementSummary(requirements []escape.RouteRequirement) string {
	if len(requirements) == 0 {
		return "--"
	}
	met := 0
	for _, requirement := range requirements {
		if requirement.Met {
			met++
		}
	}
	return fmt.Sprintf("%d/%d met", met, len(requirements))
}

func formatEscapeAttemptFeedback(feedback model.EscapeAttemptFeedback) string {
	if feedback.Route == "" || feedback.Status == "" || feedback.TickID == 0 {
		return "no attempts yet"
	}
	status := "FAIL"
	if feedback.Status == model.EscapeAttemptStatusSuccess {
		status = "SUCCESS"
	}
	if feedback.Reason == "" {
		return fmt.Sprintf("%s %s @%d", status, feedback.Route, feedback.TickID)
	}
	return fmt.Sprintf("%s %s @%d (%s)", status, feedback.Route, feedback.TickID, feedback.Reason)
}

func (s *Shell) actionPanelLayout() actionPanelLayout {
	width := float64(s.screenWidth)
	tabWidth := 94.0
	tabHeight := 24.0
	gap := 8.0

	if s.screenWidth <= 900 {
		tabWidth = 76
		tabHeight = 28
	}

	totalTabsWidth := (tabWidth * 6) + (gap * 5)
	startX := width - totalTabsWidth - 16
	if startX < 12 {
		startX = 12
	}
	y := 14.0

	inventory := input.Rect{MinX: startX, MinY: y, MaxX: startX + tabWidth, MaxY: y + tabHeight}
	cards := input.Rect{MinX: inventory.MaxX + gap, MinY: y, MaxX: inventory.MaxX + gap + tabWidth, MaxY: y + tabHeight}
	abilities := input.Rect{MinX: cards.MaxX + gap, MinY: y, MaxX: cards.MaxX + gap + tabWidth, MaxY: y + tabHeight}
	market := input.Rect{MinX: abilities.MaxX + gap, MinY: y, MaxX: abilities.MaxX + gap + tabWidth, MaxY: y + tabHeight}
	stashTab := input.Rect{MinX: market.MaxX + gap, MinY: y, MaxX: market.MaxX + gap + tabWidth, MaxY: y + tabHeight}
	escapeTab := input.Rect{MinX: stashTab.MaxX + gap, MinY: y, MaxX: stashTab.MaxX + gap + tabWidth, MaxY: y + tabHeight}

	panelY := y + tabHeight + 12
	panelWidth := tabWidth*6 + gap*5
	panelHeight := 208.0
	if s.screenHeight <= 760 {
		panelHeight = 184
	}

	prevButton := input.Rect{
		MinX: startX + 8,
		MinY: panelY + panelHeight - 34,
		MaxX: startX + 88,
		MaxY: panelY + panelHeight - 8,
	}
	nextButton := input.Rect{
		MinX: prevButton.MaxX + 10,
		MinY: prevButton.MinY,
		MaxX: prevButton.MaxX + 90,
		MaxY: prevButton.MaxY,
	}
	useButton := input.Rect{
		MinX: startX + panelWidth - 120,
		MinY: prevButton.MinY,
		MaxX: startX + panelWidth - 10,
		MaxY: prevButton.MaxY,
	}

	return actionPanelLayout{
		inventoryTab: inventory,
		cardsTab:     cards,
		abilitiesTab: abilities,
		marketTab:    market,
		stashTab:     stashTab,
		escapeTab:    escapeTab,
		prevButton:   prevButton,
		nextButton:   nextButton,
		useButton:    useButton,
	}
}

func (s *Shell) drawActionPanels(screen *ebiten.Image, state model.GameState) {
	if s == nil || s.localPlayerID == "" {
		return
	}

	local, found := playerByID(state.Players, s.localPlayerID)
	if !found {
		return
	}

	layout := s.actionPanelLayout()
	availability := computeActionPanelAvailability(local, state)

	if availability.inventory {
		s.drawActionPanelTab(screen, layout.inventoryTab, "Inventory", "Tab", s.panelMode == actionPanelInventory)
	}
	if availability.cards {
		s.drawActionPanelTab(screen, layout.cardsTab, "Cards", "C", s.panelMode == actionPanelCards)
	}
	if availability.abilities {
		s.drawActionPanelTab(screen, layout.abilitiesTab, "Abilities", "Click", s.panelMode == actionPanelAbilities)
	}
	if availability.escape {
		s.drawActionPanelTab(screen, layout.escapeTab, "Escape", "X", s.panelMode == actionPanelEscape)
	}
	if availability.stash {
		s.drawActionPanelTab(screen, layout.stashTab, "Stash", "H", s.panelMode == actionPanelStash)
	}
	if availability.market {
		instruction := marketAccessInstruction(state.Map.BlackMarketRoomID)
		text.Draw(screen, instruction, basicfont.Face7x13, int(layout.inventoryTab.MinX), int(layout.inventoryTab.MaxY)+18, color.RGBA{R: 180, G: 195, B: 210, A: 255})
	}

	if !isPanelModeAvailable(s.panelMode, availability) {
		s.panelMode = actionPanelNone
	}

	if s.panelMode == actionPanelNone {
		return
	}
	if actionPanelUsesCenteredModal(s.panelMode) {
		s.drawCenteredActionPanelModal(screen, state, local)
		return
	}

	panelX := float32(layout.inventoryTab.MinX)
	panelY := float32(layout.inventoryTab.MaxY + 10)
	panelWidth := float32(layout.abilitiesTab.MaxX - layout.inventoryTab.MinX)
	panelHeight := float32(layout.useButton.MaxY - layout.inventoryTab.MaxY - 2)

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 8, G: 12, B: 18, A: 224}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 1, color.RGBA{R: 71, G: 90, B: 110, A: 255}, false)

	title := actionPanelTitle(s.panelMode)
	text.Draw(screen, title, basicfont.Face7x13, int(panelX)+10, int(panelY)+18, color.RGBA{R: 243, G: 248, B: 255, A: 255})

	entries, selected := s.panelEntriesForDraw(state, local)
	lineY := int(panelY) + 38
	if len(entries) == 0 {
		text.Draw(screen, "No entries available.", basicfont.Face7x13, int(panelX)+10, lineY, color.RGBA{R: 194, G: 204, B: 216, A: 255})
	} else {
		maxItems := 7
		if len(entries) < maxItems {
			maxItems = len(entries)
		}
		start := 0
		if selected >= maxItems {
			start = selected - maxItems + 1
		}
		for index := start; index < start+maxItems; index++ {
			line := entries[index]
			entryColor := color.RGBA{R: 211, G: 221, B: 233, A: 255}
			if index == selected {
				entryColor = color.RGBA{R: 250, G: 253, B: 255, A: 255}
				vector.DrawFilledRect(screen, panelX+8, float32(lineY-12), panelWidth-16, 16, color.RGBA{R: 35, G: 61, B: 86, A: 210}, false)
			}
			text.Draw(screen, line, basicfont.Face7x13, int(panelX)+12, lineY, entryColor)
			lineY += 20
		}
	}

	if local.LastActionFeedback.Kind != "" && local.LastActionFeedback.TickID > 0 {
		text.Draw(
			screen,
			fmt.Sprintf("Event: %s", formatActionFeedback(local.LastActionFeedback)),
			basicfont.Face7x13,
			int(panelX)+10,
			int(layout.prevButton.MinY)-10,
			color.RGBA{R: 188, G: 202, B: 218, A: 255},
		)
	}

	s.drawActionPanelButton(screen, layout.prevButton, "Prev")
	s.drawActionPanelButton(screen, layout.nextButton, "Next")
	s.drawActionPanelButton(screen, layout.useButton, "Use")
}

func (s *Shell) drawActionPanelTab(screen *ebiten.Image, rect input.Rect, title string, shortcut string, selected bool) {
	fill := color.RGBA{R: 22, G: 30, B: 39, A: 212}
	border := color.RGBA{R: 68, G: 85, B: 104, A: 255}
	if selected {
		fill = color.RGBA{R: 44, G: 67, B: 92, A: 224}
		border = color.RGBA{R: 160, G: 197, B: 232, A: 255}
	}

	x := float32(rect.MinX)
	y := float32(rect.MinY)
	w := float32(rect.MaxX - rect.MinX)
	h := float32(rect.MaxY - rect.MinY)
	vector.DrawFilledRect(screen, x, y, w, h, fill, false)
	vector.StrokeRect(screen, x, y, w, h, 1, border, false)

	text.Draw(screen, title, basicfont.Face7x13, int(rect.MinX)+8, int(rect.MinY)+15, color.RGBA{R: 240, G: 246, B: 252, A: 255})
	text.Draw(screen, shortcut, basicfont.Face7x13, int(rect.MaxX)-20, int(rect.MinY)+15, color.RGBA{R: 191, G: 207, B: 222, A: 255})
}

func (s *Shell) drawActionPanelButton(screen *ebiten.Image, rect input.Rect, label string) {
	x := float32(rect.MinX)
	y := float32(rect.MinY)
	w := float32(rect.MaxX - rect.MinX)
	h := float32(rect.MaxY - rect.MinY)
	vector.DrawFilledRect(screen, x, y, w, h, color.RGBA{R: 27, G: 39, B: 53, A: 220}, false)
	vector.StrokeRect(screen, x, y, w, h, 1, color.RGBA{R: 91, G: 116, B: 138, A: 255}, false)

	labelX := int(rect.MinX + ((rect.MaxX - rect.MinX) / 2) - float64(len(label)*3))
	labelY := int(rect.MinY + ((rect.MaxY - rect.MinY) / 2) + 4)
	text.Draw(screen, label, basicfont.Face7x13, labelX, labelY, color.RGBA{R: 236, G: 244, B: 251, A: 255})
}

func (s *Shell) drawCenteredActionPanelModal(screen *ebiten.Image, state model.GameState, local model.PlayerState) {
	if s == nil {
		return
	}

	type modalLine struct {
		text      string
		color     color.RGBA
		highlight bool
	}

	lines := make([]modalLine, 0, 24)
	lines = append(lines,
		modalLine{text: actionPanelTitle(s.panelMode), color: color.RGBA{R: 244, G: 249, B: 255, A: 255}},
		modalLine{text: "Arrows: select | Enter: confirm | X: close", color: color.RGBA{R: 181, G: 199, B: 217, A: 255}},
		modalLine{text: "", color: color.RGBA{R: 194, G: 205, B: 218, A: 255}},
	)

	detailColor := color.RGBA{R: 197, G: 209, B: 223, A: 255}
	detailLines := make([]string, 0, 8)
	switch s.panelMode {
	case actionPanelMarket:
		offers := marketPanelEntries()
		if len(offers) == 0 {
			detailLines = append(detailLines, "No market offers available.")
			break
		}
		s.panelMarketIdx = clampPanelIndex(s.panelMarketIdx, len(offers))
		for index, offer := range offers {
			canBuy, _ := marketOfferUsability(local, state, offer)
			status := "LOCKED"
			if canBuy {
				status = "READY"
			}
			lines = append(lines, modalLine{
				text:      fmt.Sprintf("%d. %s x%d  M%d  %s", index+1, offer.Item, offer.Quantity, offer.MoneyCardCost, status),
				color:     color.RGBA{R: 214, G: 224, B: 236, A: 255},
				highlight: index == s.panelMarketIdx,
			})
		}

		selectedOffer := offers[s.panelMarketIdx]
		moneyCards := countCards(local.Cards, model.CardMoney)
		canBuy, reason := marketOfferUsability(local, state, selectedOffer)
		detailLines = append(detailLines, fmt.Sprintf("Money cards: %d | Cost: %d", moneyCards, selectedOffer.MoneyCardCost))
		detailLines = append(detailLines, reason)
		if !canBuy {
			detailColor = color.RGBA{R: 242, G: 185, B: 131, A: 255}
		}

	case actionPanelEscape:
		evaluations := escapePanelEntries(local, state.Map)
		if len(evaluations) == 0 {
			detailLines = append(detailLines, "No escape routes available.")
			break
		}
		s.panelEscapeIdx = clampPanelIndex(s.panelEscapeIdx, len(evaluations))
		for index, entry := range evaluations {
			stateText := "BLOCKED"
			if entry.CanAttempt {
				stateText = "READY"
			}
			lines = append(lines, modalLine{
				text:      fmt.Sprintf("%d. %s  %s", index+1, entry.RouteLabel, stateText),
				color:     color.RGBA{R: 214, G: 224, B: 236, A: 255},
				highlight: index == s.panelEscapeIdx,
			})
		}

		selected := evaluations[s.panelEscapeIdx]
		if selected.CanAttempt {
			detailLines = append(detailLines, fmt.Sprintf("%s is ready. Press Enter to attempt.", selected.RouteLabel))
		} else {
			detailColor = color.RGBA{R: 242, G: 185, B: 131, A: 255}
			if selected.FailureReason != "" {
				detailLines = append(detailLines, selected.FailureReason)
			}
		}
		for _, requirement := range selected.Requirements {
			prefix := "[ ]"
			if requirement.Met {
				prefix = "[x]"
			}
			detailLines = append(detailLines, fmt.Sprintf("%s %s", prefix, requirement.Label))
		}
		detailLines = append(detailLines, "Last: "+formatEscapeAttemptFeedback(local.LastEscapeAttempt))

	case actionPanelNightCards:
		choices := nightCardPanelEntries(local)
		if len(choices) == 0 {
			detailLines = append(detailLines, "No night cards pending.")
			break
		}
		s.panelNightCardsIdx = clampPanelIndex(s.panelNightCardsIdx, len(choices))
		for index, card := range choices {
			lines = append(lines, modalLine{
				text:      fmt.Sprintf("%d. %s", index+1, card),
				color:     color.RGBA{R: 214, G: 224, B: 236, A: 255},
				highlight: index == s.panelNightCardsIdx,
			})
		}
		detailLines = append(detailLines, fmt.Sprintf("Choose 1 card for the night. Slots: %d/%d", len(local.Cards), cards.MaxCardsHeld))

	case actionPanelStash:
		entries := stashPanelEntries(local, state.Map)
		if len(entries) == 0 {
			detailLines = append(detailLines, "No stash actions available.")
			break
		}
		s.panelStashIdx = clampPanelIndex(s.panelStashIdx, len(entries))
		for index, entry := range entries {
			verb := "Store"
			if entry.Action == "withdraw" {
				verb = "Take"
			}
			lines = append(lines, modalLine{
				text:      fmt.Sprintf("%d. %s %s x%d", index+1, verb, entry.Item, entry.Count),
				color:     color.RGBA{R: 214, G: 224, B: 236, A: 255},
				highlight: index == s.panelStashIdx,
			})
		}
		detailLines = append(detailLines, fmt.Sprintf("Cell stash entries: %d", len(stashForAssignedCell(local, state.Map))))
	}

	lines = append(lines, modalLine{text: "", color: color.RGBA{R: 194, G: 205, B: 218, A: 255}})
	for _, detail := range detailLines {
		lines = append(lines, modalLine{text: detail, color: detailColor})
	}
	if strings.TrimSpace(s.panelLocalHint) != "" {
		lines = append(lines, modalLine{text: "", color: color.RGBA{R: 194, G: 205, B: 218, A: 255}})
		hintColor := color.RGBA{R: 208, G: 219, B: 232, A: 255}
		if s.panelLocalHintWarning {
			hintColor = color.RGBA{R: 242, G: 185, B: 131, A: 255}
		}
		lines = append(lines, modalLine{text: "Hint: " + s.panelLocalHint, color: hintColor})
	}

	maxLines := len(lines)
	if maxLines > 20 {
		maxLines = 20
		lines = lines[:maxLines]
	}

	panelText := make([]string, 0, len(lines))
	for _, line := range lines {
		panelText = append(panelText, line.text)
	}

	panelWidth := clampFloat32(estimatePanelWidth(panelText), 460, float32(s.screenWidth)-36)
	panelHeight := float32(28 + (len(lines) * 16))
	maxHeight := float32(s.screenHeight) - 28
	if panelHeight > maxHeight {
		panelHeight = maxHeight
	}
	panelX := (float32(s.screenWidth) - panelWidth) / 2
	panelY := (float32(s.screenHeight) - panelHeight) / 2

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 7, G: 13, B: 19, A: 236}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 2, color.RGBA{R: 99, G: 124, B: 147, A: 255}, false)

	lineY := int(panelY) + 22
	for _, line := range lines {
		if line.highlight {
			vector.DrawFilledRect(screen, panelX+10, float32(lineY-11), panelWidth-20, 15, color.RGBA{R: 35, G: 61, B: 86, A: 214}, false)
		}
		text.Draw(screen, line.text, basicfont.Face7x13, int(panelX)+14, lineY, line.color)
		lineY += 16
	}
}

func actionPanelTitle(mode actionPanelMode) string {
	switch mode {
	case actionPanelInventory:
		return "Inventory (use selected item)"
	case actionPanelCards:
		return "Cards (consume selected card)"
	case actionPanelAbilities:
		return "Abilities (activate selected ability)"
	case actionPanelMarket:
		return "Black Market"
	case actionPanelEscape:
		return "Escape Routes"
	case actionPanelNightCards:
		return "Night Card Draft"
	case actionPanelStash:
		return "Cell Stash"
	default:
		return "Action Panel"
	}
}

func (s *Shell) panelEntriesForDraw(state model.GameState, local model.PlayerState) ([]string, int) {
	switch s.panelMode {
	case actionPanelInventory:
		entries := inventoryPanelEntries(local)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			lines = append(lines, fmt.Sprintf("%d. %s x%d", index+1, entry.Item, entry.Quantity))
		}
		return lines, clampPanelIndex(s.panelInventoryIdx, len(lines))

	case actionPanelCards:
		entries := cardPanelEntries(local)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			lines = append(lines, fmt.Sprintf("%d. %s", index+1, entry))
		}
		return lines, clampPanelIndex(s.panelCardsIdx, len(lines))

	case actionPanelAbilities:
		entries := abilityPanelEntries(local)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			lines = append(lines, fmt.Sprintf("%d. %s", index+1, entry))
		}
		return lines, clampPanelIndex(s.panelAbilitiesIdx, len(lines))

	case actionPanelMarket:
		entries := marketPanelEntries()
		lines := make([]string, 0, len(entries))
		for index, offer := range entries {
			canBuy, _ := marketOfferUsability(local, state, offer)
			status := "LOCKED"
			if canBuy {
				status = "READY"
			}
			lines = append(lines, fmt.Sprintf("%d. %s x%d  M%d  %s", index+1, offer.Item, offer.Quantity, offer.MoneyCardCost, status))
		}
		return lines, clampPanelIndex(s.panelMarketIdx, len(lines))

	case actionPanelEscape:
		entries := escapePanelEntries(local, state.Map)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			stateText := "BLOCKED"
			if entry.CanAttempt {
				stateText = "READY"
			}
			lines = append(lines, fmt.Sprintf("%d. %s  %s  %s", index+1, entry.RouteLabel, stateText, requirementSummary(entry.Requirements)))
		}
		return lines, clampPanelIndex(s.panelEscapeIdx, len(entries))

	case actionPanelNightCards:
		entries := nightCardPanelEntries(local)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			lines = append(lines, fmt.Sprintf("%d. %s", index+1, entry))
		}
		return lines, clampPanelIndex(s.panelNightCardsIdx, len(entries))

	case actionPanelStash:
		entries := stashPanelEntries(local, state.Map)
		lines := make([]string, 0, len(entries))
		for index, entry := range entries {
			verb := "Store"
			if entry.Action == "withdraw" {
				verb = "Take"
			}
			lines = append(lines, fmt.Sprintf("%d. %s %s x%d", index+1, verb, entry.Item, entry.Count))
		}
		return lines, clampPanelIndex(s.panelStashIdx, len(entries))
	}

	return nil, 0
}
