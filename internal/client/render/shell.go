package render

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"

	"prison-break/internal/client/input"
	"prison-break/internal/client/netclient"
	"prison-break/internal/client/prediction"
	"prison-break/internal/gamecore/abilities"
	"prison-break/internal/gamecore/combat"
	"prison-break/internal/gamecore/escape"
	gamemap "prison-break/internal/gamecore/map"
	"prison-break/internal/shared/model"
)

const (
	defaultScreenWidth  = 1280
	defaultScreenHeight = 720
)

type ShellConfig struct {
	ScreenWidth             int
	ScreenHeight            int
	LocalPlayerID           model.PlayerID
	SpectatorFollowPlayerID model.PlayerID
	SpectatorFollowSlot     uint8
	SpectatorSlotCount      uint8
	Store                   *netclient.SnapshotStore
	Layout                  gamemap.Layout

	InputController       *input.Controller
	OnInputCommand        func(model.InputCommand)
	InputSnapshotProvider func() input.InputSnapshot

	PredictionEngine *prediction.Engine
	Now              func() time.Time
}

type Shell struct {
	screenWidth   int
	screenHeight  int
	localPlayerID model.PlayerID

	store  *netclient.SnapshotStore
	layout gamemap.Layout
	camera Camera

	inputController       *input.Controller
	onInputCommand        func(model.InputCommand)
	outgoingCommands      []model.InputCommand
	inputSnapshotProvider func() input.InputSnapshot

	predictionEngine *prediction.Engine
	nowFn            func() time.Time

	lastReconciledTick      uint64
	panelMode               actionPanelMode
	panelInventoryIdx       int
	panelCardsIdx           int
	panelAbilitiesIdx       int
	panelMarketIdx          int
	panelEscapeIdx          int
	panelNightCardsIdx      int
	panelStashIdx           int
	panelInputPrev          panelInputEdgeState
	panelSuppressGameplay   bool
	panelSuppressInteract   bool
	panelLocalHint          string
	panelLocalHintWarning   bool
	cameraCyclePrevPressed  bool
	cameraCycleNextPressed  bool
	cameraViewRoomIndex     int
	footstepHistory         map[model.PlayerID][]footstepSample
	spectatorInputPrev      spectatorInputEdgeState
	spectatorFollowPlayerID model.PlayerID
	spectatorFollowSlot     int
	spectatorSlotCount      int
	pauseMenuOpen           bool
	abilityInfoOpen         bool
	abilityInfoPrevPressed  bool
}

type footstepSample struct {
	Position model.Vector2
	TickID   uint64
}

type spectatorInputEdgeState struct {
	prev bool
	next bool
}

func NewShell(config ShellConfig) *Shell {
	screenWidth := config.ScreenWidth
	if screenWidth <= 0 {
		screenWidth = defaultScreenWidth
	}
	screenHeight := config.ScreenHeight
	if screenHeight <= 0 {
		screenHeight = defaultScreenHeight
	}

	store := config.Store
	if store == nil {
		store = netclient.NewSnapshotStore()
	}

	layout := config.Layout
	if layout.Width() == 0 || layout.Height() == 0 {
		layout = gamemap.DefaultPrisonLayout()
	}

	camera := NewCamera(screenWidth, screenHeight)
	camera.TilePixels = 40
	camera.Zoom = 1.00

	inputController := config.InputController
	if inputController == nil {
		inputController = input.NewController(input.ControllerConfig{
			PlayerID:     config.LocalPlayerID,
			ScreenWidth:  screenWidth,
			ScreenHeight: screenHeight,
		})
	}

	shell := &Shell{
		screenWidth:             screenWidth,
		screenHeight:            screenHeight,
		localPlayerID:           config.LocalPlayerID,
		spectatorFollowPlayerID: model.PlayerID(strings.TrimSpace(string(config.SpectatorFollowPlayerID))),
		spectatorFollowSlot:     int(config.SpectatorFollowSlot),
		spectatorSlotCount:      int(config.SpectatorSlotCount),
		store:                   store,
		layout:                  layout,
		camera:                  camera,
		inputController:         inputController,
		onInputCommand:          config.OnInputCommand,
		outgoingCommands:        make([]model.InputCommand, 0, 64),
		footstepHistory:         make(map[model.PlayerID][]footstepSample),
	}
	if config.InputSnapshotProvider != nil {
		shell.inputSnapshotProvider = config.InputSnapshotProvider
	} else {
		shell.inputSnapshotProvider = shell.captureInputSnapshot
	}
	if config.Now != nil {
		shell.nowFn = config.Now
	} else {
		shell.nowFn = time.Now
	}

	if config.PredictionEngine != nil {
		shell.predictionEngine = config.PredictionEngine
	} else {
		shell.predictionEngine = prediction.NewEngine(config.LocalPlayerID, prediction.DefaultConfig())
	}

	if shell.predictionEngine != nil {
		if initial, ok := shell.store.CurrentState(); ok {
			shell.predictionEngine.SeedAuthoritativeState(initial, shell.nowFn().UTC())
			shell.lastReconciledTick = initial.TickID
		}
	}

	return shell
}

func (s *Shell) ApplyAuthoritativeSnapshot(snapshot model.Snapshot) bool {
	applied := s.store.ApplySnapshot(snapshot)
	if !applied {
		return false
	}

	if s.predictionEngine != nil {
		if current, ok := s.store.CurrentState(); ok {
			s.predictionEngine.AcceptAuthoritativeSnapshot(snapshot, current, s.nowFn().UTC())
			s.lastReconciledTick = snapshot.TickID
		}
	}

	return true
}

func (s *Shell) Update() error {
	s.reconcilePredictionFromStore(s.nowFn().UTC())

	if s.store == nil {
		return nil
	}

	state, hasState := s.store.CurrentState()
	if !hasState {
		return nil
	}

	snapshot := input.InputSnapshot{}
	if s.inputSnapshotProvider != nil {
		snapshot = s.inputSnapshotProvider()
	}
	if snapshot.AbilityInfoPressed && !s.abilityInfoPrevPressed {
		s.abilityInfoOpen = !s.abilityInfoOpen
	}
	s.abilityInfoPrevPressed = snapshot.AbilityInfoPressed

	if s.localPlayerID == "" {
		s.updateSpectatorFollowSelection(state, snapshot)
	}
	if focus, hasFocus := s.resolveCameraFocusPlayer(state); hasFocus {
		s.camera.FocusOn(focus.Position)
		s.camera.ClampToLayout(s.layout)
	}

	if s.inputController == nil {
		return nil
	}
	if s.pauseMenuOpen {
		return nil
	}

	local, hasLocal := playerByID(state.Players, s.localPlayerID)
	var localPtr *model.PlayerState
	var localState model.PlayerState
	if hasLocal {
		localState = local
		localPtr = &localState
	}
	s.recordFootstepHistory(state)
	s.updateCameraViewSelection(snapshot, state, localPtr)

	s.panelSuppressGameplay = false
	s.panelSuppressInteract = false
	panelCommands := s.updateActionPanelCommands(snapshot, state, localPtr)
	gameplaySnapshot := s.filterSnapshotForActionPanels(snapshot)
	commands := s.inputController.BuildCommands(gameplaySnapshot, state.TickID+1, localPtr)
	if len(panelCommands) > 0 {
		commands = append(commands, panelCommands...)
	}
	if len(commands) == 0 {
		return nil
	}

	if s.predictionEngine != nil {
		s.predictionEngine.RecordLocalCommands(commands)
	}

	s.outgoingCommands = append(s.outgoingCommands, commands...)
	if s.onInputCommand != nil {
		for _, command := range commands {
			s.onInputCommand(command)
		}
	}

	return nil
}

func (s *Shell) DrainOutgoingCommands() []model.InputCommand {
	if len(s.outgoingCommands) == 0 {
		return nil
	}

	out := make([]model.InputCommand, len(s.outgoingCommands))
	copy(out, s.outgoingCommands)
	s.outgoingCommands = s.outgoingCommands[:0]
	return out
}

func (s *Shell) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{R: 18, G: 23, B: 30, A: 255})

	state, hasState := s.resolveRenderState()
	if !hasState {
		text.Draw(screen, "Waiting for authoritative snapshot...", basicfont.Face7x13, 24, 32, color.White)
		return
	}

	if focus, hasFocus := s.resolveCameraFocusPlayer(state); hasFocus {
		s.camera.FocusOn(focus.Position)
	}
	s.camera.ClampToLayout(s.layout)

	s.drawRooms(screen, state)
	s.drawDoors(screen, state)
	s.drawEntities(screen, state)
	s.drawPlayers(screen, state)
	s.drawHUD(screen, state)
	s.drawSpectatorOverlay(screen, state)
	s.drawActionPanels(screen, state)
	s.drawAbilityInfoPanel(screen, state)
	if s.pauseMenuOpen {
		s.drawPauseMenu(screen)
	}
}

func (s *Shell) resolveRenderState() (model.GameState, bool) {
	s.reconcilePredictionFromStore(s.nowFn().UTC())

	state, hasState := s.store.CurrentState()
	if !hasState {
		return model.GameState{}, false
	}

	if s.predictionEngine == nil {
		return state, true
	}

	predicted, ok := s.predictionEngine.RenderState(s.nowFn().UTC())
	if !ok {
		return state, true
	}

	return predicted, true
}

func (s *Shell) reconcilePredictionFromStore(now time.Time) {
	if s.predictionEngine == nil || s.store == nil {
		return
	}

	meta, hasMeta := s.store.LatestSnapshotMeta()
	if !hasMeta || meta.TickID == 0 || meta.TickID <= s.lastReconciledTick {
		return
	}

	state, hasState := s.store.CurrentState()
	if !hasState {
		return
	}

	snapshot := model.Snapshot{
		Kind:       model.SnapshotKindDelta,
		TickID:     meta.TickID,
		PlayerAcks: meta.PlayerAcks,
	}
	s.predictionEngine.AcceptAuthoritativeSnapshot(snapshot, state, now)
	s.lastReconciledTick = meta.TickID
}

func (s *Shell) Layout(_, _ int) (int, int) {
	return s.screenWidth, s.screenHeight
}

func (s *Shell) recordFootstepHistory(state model.GameState) {
	if s == nil {
		return
	}
	if s.footstepHistory == nil {
		s.footstepHistory = make(map[model.PlayerID][]footstepSample)
	}

	const maxSamplesPerPlayer = 720
	for _, player := range state.Players {
		if player.ID == "" || !player.Alive {
			continue
		}
		history := s.footstepHistory[player.ID]
		if len(history) > 0 && history[len(history)-1].TickID == state.TickID {
			history[len(history)-1].Position = player.Position
		} else {
			history = append(history, footstepSample{
				Position: player.Position,
				TickID:   state.TickID,
			})
		}
		if len(history) > maxSamplesPerPlayer {
			history = append([]footstepSample(nil), history[len(history)-maxSamplesPerPlayer:]...)
		}
		s.footstepHistory[player.ID] = history
	}
}

func (s *Shell) updateCameraViewSelection(
	snapshot input.InputSnapshot,
	state model.GameState,
	localPlayer *model.PlayerState,
) {
	if s == nil {
		return
	}

	currentPrev := snapshot.PanelPrevPressed
	currentNext := snapshot.PanelNextPressed
	prevEdge := currentPrev && !s.cameraCyclePrevPressed
	nextEdge := currentNext && !s.cameraCycleNextPressed
	s.cameraCyclePrevPressed = currentPrev
	s.cameraCycleNextPressed = currentNext

	if localPlayer == nil || localPlayer.ID == "" {
		s.cameraViewRoomIndex = 0
		return
	}
	if !effectActiveForPlayer(*localPlayer, model.EffectCameraView, state.TickID) || !state.Map.PowerOn {
		s.cameraViewRoomIndex = 0
		return
	}
	if actionPanelUsesCenteredModal(s.panelMode) {
		return
	}

	rooms := s.cameraViewRooms()
	if len(rooms) == 0 {
		s.cameraViewRoomIndex = 0
		return
	}
	s.cameraViewRoomIndex = clampPanelIndex(s.cameraViewRoomIndex, len(rooms))
	if prevEdge {
		s.cameraViewRoomIndex = wrapPanelIndex(s.cameraViewRoomIndex, len(rooms), -1)
	}
	if nextEdge {
		s.cameraViewRoomIndex = wrapPanelIndex(s.cameraViewRoomIndex, len(rooms), 1)
	}
}

func (s *Shell) cameraViewRooms() []model.RoomID {
	if s == nil {
		return nil
	}
	rooms := s.layout.Rooms()
	if len(rooms) == 0 {
		return nil
	}
	out := make([]model.RoomID, 0, len(rooms))
	for _, room := range rooms {
		if room.ID == "" {
			continue
		}
		out = append(out, room.ID)
	}
	sort.Slice(out, func(i int, j int) bool {
		return out[i] < out[j]
	})
	return out
}

func (s *Shell) drawRooms(screen *ebiten.Image, state model.GameState) {
	_, hasLocal, localRoomID, limitToLocalRoom := s.resolveVisionScope(state)
	if !hasLocal {
		limitToLocalRoom = false
	}

	for _, room := range s.layout.Rooms() {
		if limitToLocalRoom && room.ID != localRoomID {
			continue
		}

		widthTiles := float64((room.Max.X - room.Min.X) + 1)
		heightTiles := float64((room.Max.Y - room.Min.Y) + 1)
		x, y, w, h := s.camera.TileRectToScreen(float64(room.Min.X), float64(room.Min.Y), widthTiles, heightTiles)

		fill := roomFillColor(room.ID, state.Map.BlackMarketRoomID)
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 1, color.RGBA{R: 42, G: 53, B: 67, A: 255}, false)

		label := roomDisplayLabel(room.ID)
		if label != "" {
			labelX := int(x + (w / 2) - float64(len(label)*3))
			labelY := int(y + (h / 2))
			text.Draw(screen, label, basicfont.Face7x13, labelX, labelY, color.RGBA{R: 224, G: 231, B: 238, A: 220})
		}
	}
}

func (s *Shell) drawDoors(screen *ebiten.Image, state model.GameState) {
	localPlayer, hasLocal, localRoomID, limitToLocalRoom := s.resolveVisionScope(state)

	doorByID := make(map[model.DoorID]model.DoorState, len(state.Map.Doors))
	for _, door := range state.Map.Doors {
		doorByID[door.ID] = door
	}

	for _, doorLink := range s.layout.DoorLinks() {
		if limitToLocalRoom && doorLink.RoomA != localRoomID && doorLink.RoomB != localRoomID {
			continue
		}

		doorState, exists := doorByID[doorLink.ID]
		if !exists {
			doorState = model.DoorState{
				ID:    doorLink.ID,
				RoomA: doorLink.RoomA,
				RoomB: doorLink.RoomB,
				Open:  true,
			}
		}
		open := doorState.Open
		if hasLocal {
			open = projectDoorOpenForViewer(open, doorState, localPlayer, state.Map)
		}

		center := model.Vector2{X: float32(doorLink.Position.X) + 0.5, Y: float32(doorLink.Position.Y) + 0.5}
		x, y, w, h := s.camera.TileRectToScreen(float64(center.X)-0.50, float64(center.Y)-0.50, 1.00, 1.00)

		fill := color.RGBA{R: 210, G: 66, B: 66, A: 255}
		if open {
			fill = color.RGBA{R: 88, G: 198, B: 121, A: 255}
		}
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
		vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), 2.5, color.RGBA{R: 232, G: 239, B: 246, A: 255}, false)
		label := "C"
		if open {
			label = "O"
		}
		text.Draw(screen, label, basicfont.Face7x13, int(x+w/2)-3, int(y+h/2)+5, color.RGBA{R: 250, G: 252, B: 255, A: 255})

		destinationLabel := doorLeadLabelForViewer(doorState, hasLocal, localRoomID)
		if destinationLabel != "" {
			labelX := int(x + (w / 2) - float64(len(destinationLabel)*3))
			labelY := int(y) - 4
			if labelY < 12 {
				labelY = int(y + h + 12)
			}
			text.Draw(screen, destinationLabel, basicfont.Face7x13, labelX, labelY, color.RGBA{R: 216, G: 228, B: 241, A: 248})
		}
	}
}

func (s *Shell) drawPlayers(screen *ebiten.Image, state model.GameState) {
	localViewer, hasLocal, localRoomID, limitToLocalRoom := s.resolveVisionScope(state)
	if !hasLocal {
		limitToLocalRoom = false
	}

	s.drawTrackerFootsteps(screen, state, localViewer, hasLocal)

	for _, player := range state.Players {
		if limitToLocalRoom && player.ID != s.localPlayerID && player.CurrentRoomID != localRoomID {
			continue
		}
		if hasLocal && player.ID != s.localPlayerID && playerIsInvisibleForViewer(player, localViewer, state.TickID) {
			continue
		}

		size := 0.72
		x, y, w, h := s.camera.TileRectToScreen(float64(player.Position.X)-(size/2), float64(player.Position.Y)-(size/2), size, size)

		fill := playerFillColor(player)
		label := string(player.ID)
		if player.ID != s.localPlayerID && effectActiveForPlayer(player, model.EffectDisguised, state.TickID) {
			fill = entityFillColor(model.EntityKindNPCPrisoner)
			label = "NPC"
		}
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)

		borderColor := color.RGBA{R: 16, G: 19, B: 24, A: 255}
		border := float32(1)
		if player.ID == s.localPlayerID {
			borderColor = color.RGBA{R: 251, G: 252, B: 255, A: 255}
			border = 2
		}
		vector.StrokeRect(screen, float32(x), float32(y), float32(w), float32(h), border, borderColor, false)

		labelX := int(x)
		labelY := int(y) - 3
		if labelY < 10 {
			labelY = int(y + h + 11)
		}
		labelColor := toRGBA(fill)
		labelColor.A = 255
		text.Draw(screen, label, basicfont.Face7x13, labelX, labelY, labelColor)
	}
}

func (s *Shell) drawTrackerFootsteps(
	screen *ebiten.Image,
	state model.GameState,
	localViewer model.PlayerState,
	hasLocal bool,
) {
	if s == nil || !hasLocal || !effectActiveForPlayer(localViewer, model.EffectTrackerView, state.TickID) {
		return
	}

	_, _, visibleRoomID, limitToVisibleRoom := s.resolveVisionScope(state)
	const trailWindowTicks = 900
	const maxTrailSamplesPerPlayer = 120
	cutoffTick := uint64(0)
	if state.TickID > trailWindowTicks {
		cutoffTick = state.TickID - trailWindowTicks
	}

	for _, player := range state.Players {
		if player.ID == "" || !player.Alive {
			continue
		}

		history := s.footstepHistory[player.ID]
		if len(history) == 0 {
			continue
		}

		baseColor := toRGBA(playerFillColor(player))
		drawn := 0
		for index := len(history) - 1; index >= 0; index-- {
			sample := history[index]
			if sample.TickID < cutoffTick {
				break
			}
			if limitToVisibleRoom && visibleRoomID != "" {
				point := gamemap.Point{X: int(sample.Position.X), Y: int(sample.Position.Y)}
				roomID, inRoom := s.layout.RoomAt(point)
				if !inRoom || roomID != visibleRoomID {
					continue
				}
			}

			ageTicks := state.TickID - sample.TickID
			fadeRatio := 1.0 - (float64(ageTicks) / float64(trailWindowTicks))
			if fadeRatio < 0.18 {
				fadeRatio = 0.18
			}

			dotColor := baseColor
			dotColor.A = uint8(32 + (fadeRatio * 140))
			size := 0.16
			if player.ID == s.localPlayerID {
				size = 0.14
			}

			x, y, w, h := s.camera.TileRectToScreen(
				float64(sample.Position.X)-(size/2),
				float64(sample.Position.Y)-(size/2),
				size,
				size,
			)
			vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), dotColor, false)
			drawn++
			if drawn >= maxTrailSamplesPerPlayer {
				break
			}
		}
	}
}

func (s *Shell) drawEntities(screen *ebiten.Image, state model.GameState) {
	_, hasLocal, localRoomID, limitToLocalRoom := s.resolveVisionScope(state)
	if !hasLocal {
		limitToLocalRoom = false
	}

	for _, entity := range state.Entities {
		if !entity.Active {
			continue
		}
		if limitToLocalRoom && entity.RoomID != localRoomID {
			continue
		}
		size := 0.45
		x, y, w, h := s.camera.TileRectToScreen(float64(entity.Position.X)-(size/2), float64(entity.Position.Y)-(size/2), size, size)
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), entityFillColor(entity.Kind), false)
	}
}

func resolveLocalVisionScope(
	state model.GameState,
	localPlayerID model.PlayerID,
) (localPlayer model.PlayerState, hasLocalPlayer bool, localRoomID model.RoomID, limitToLocalRoom bool) {
	if localPlayerID == "" {
		return model.PlayerState{}, false, "", false
	}

	local, found := playerByID(state.Players, localPlayerID)
	if !found {
		return model.PlayerState{}, false, "", false
	}

	localRoomID = local.CurrentRoomID
	if localRoomID == "" {
		return local, true, "", false
	}

	return local, true, localRoomID, true
}

func (s *Shell) resolveVisionScope(
	state model.GameState,
) (localPlayer model.PlayerState, hasLocalPlayer bool, localRoomID model.RoomID, limitToLocalRoom bool) {
	localPlayer, hasLocalPlayer, localRoomID, limitToLocalRoom = resolveLocalVisionScope(state, s.localPlayerID)
	if !hasLocalPlayer {
		return model.PlayerState{}, false, "", false
	}

	// Camera-man view overrides room-limited fog while the feed effect is active.
	if effectActiveForPlayer(localPlayer, model.EffectCameraView, state.TickID) && state.Map.PowerOn {
		rooms := s.cameraViewRooms()
		if len(rooms) > 0 {
			s.cameraViewRoomIndex = clampPanelIndex(s.cameraViewRoomIndex, len(rooms))
			selectedRoom := rooms[s.cameraViewRoomIndex]
			if selectedRoom != "" {
				localRoomID = selectedRoom
				limitToLocalRoom = true
			}
		}
	}

	return localPlayer, hasLocalPlayer, localRoomID, limitToLocalRoom
}

func projectDoorOpenForViewer(
	isDoorOpen bool,
	door model.DoorState,
	viewer model.PlayerState,
	mapState model.MapState,
) bool {
	if !isDoorOpen {
		return false
	}
	if !canViewerTraverseDoor(viewer, door, mapState) {
		return false
	}
	return true
}

func canViewerTraverseDoor(
	viewer model.PlayerState,
	door model.DoorState,
	mapState model.MapState,
) bool {
	if door.ID == 0 {
		return false
	}

	if viewer.CurrentRoomID != "" {
		targetRoomID, adjacent := doorAdjacentTargetRoom(viewer.CurrentRoomID, door)
		if !adjacent {
			return false
		}
		if targetRoomID != "" && !gamemap.CanEnterRoom(viewer, targetRoomID, mapState) {
			return false
		}
	} else {
		if !gamemap.CanEnterRoom(viewer, door.RoomA, mapState) &&
			!gamemap.CanEnterRoom(viewer, door.RoomB, mapState) {
			return false
		}
	}

	if cell, exists := cellStateForDoor(mapState.Cells, door.ID); exists && !gamemap.CanOperateCellDoor(viewer, cell) {
		return false
	}

	return true
}

func effectActiveForPlayer(player model.PlayerState, effect model.EffectType, tickID uint64) bool {
	if effect == "" {
		return false
	}
	for _, active := range player.Effects {
		if active.Effect != effect {
			continue
		}
		if active.EndsTick != 0 && tickID > active.EndsTick {
			continue
		}
		return true
	}
	return false
}

func playerIsInvisibleForViewer(target model.PlayerState, viewer model.PlayerState, tickID uint64) bool {
	if target.ID == "" || target.ID == viewer.ID {
		return false
	}
	return effectActiveForPlayer(target, model.EffectChameleon, tickID)
}

func doorAdjacentTargetRoom(currentRoomID model.RoomID, door model.DoorState) (model.RoomID, bool) {
	switch currentRoomID {
	case door.RoomA:
		return door.RoomB, true
	case door.RoomB:
		return door.RoomA, true
	default:
		return "", false
	}
}

func cellStateForDoor(cells []model.CellState, doorID model.DoorID) (model.CellState, bool) {
	if doorID == 0 || len(cells) == 0 {
		return model.CellState{}, false
	}
	for _, cell := range cells {
		if cell.DoorID == doorID {
			return cell, true
		}
	}
	return model.CellState{}, false
}

func doorLeadLabelForViewer(
	door model.DoorState,
	hasLocal bool,
	localRoomID model.RoomID,
) string {
	if hasLocal && localRoomID != "" {
		targetRoomID, adjacent := doorAdjacentTargetRoom(localRoomID, door)
		if adjacent && targetRoomID != "" {
			return roomDisplayLabel(targetRoomID)
		}
	}

	if door.RoomA != "" && door.RoomB != "" {
		return fmt.Sprintf("%s/%s", roomDisplayLabel(door.RoomA), roomDisplayLabel(door.RoomB))
	}
	return ""
}

func (s *Shell) drawHUD(screen *ebiten.Image, state model.GameState) {
	showMobileHints := s.shouldShowMobileActionSurfaces()
	lines := BuildHUDLinesWithOptions(state, s.localPlayerID, HUDOptions{
		ShowDesktopActionHints:  false,
		ShowMobileActionHints:   false,
		SpectatorFollowPlayerID: s.spectatorFollowPlayerID,
		SpectatorFollowSlot:     s.spectatorFollowSlot,
		SpectatorSlotCount:      s.spectatorSlotCount,
		ShowVerboseDetails:      false,
		PingMS:                  -1,
	})

	panelX := float32(12)
	panelY := float32(12)
	lineHeight := float32(16)
	panelWidth := clampFloat32(estimatePanelWidth(lines), 220, float32(s.screenWidth)-24)
	panelHeight := float32(16 + (len(lines) * int(lineHeight)))
	if panelHeight > float32(s.screenHeight)-24 {
		panelHeight = float32(s.screenHeight) - 24
	}

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 5, G: 8, B: 12, A: 210}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 1, color.RGBA{R: 53, G: 72, B: 89, A: 255}, false)

	maxLines := int((panelHeight - 12) / lineHeight)
	if maxLines > len(lines) {
		maxLines = len(lines)
	}
	for i := 0; i < maxLines; i++ {
		text.Draw(screen, lines[i], basicfont.Face7x13, int(panelX)+10, int(panelY)+20+(i*int(lineHeight)), color.RGBA{R: 231, G: 237, B: 245, A: 255})
	}

	if !showMobileHints {
		s.drawDesktopActionIndicators(screen, state)
	}

	if showMobileHints {
		s.drawMobileActionSurfaces(screen, state)
	}

	s.drawLatestActionFeedback(screen, state)
}

func (s *Shell) shouldShowMobileActionSurfaces() bool {
	if s == nil || s.inputController == nil {
		return false
	}
	if s.screenWidth <= 960 {
		return true
	}
	return len(ebiten.AppendTouchIDs(nil)) > 0
}

func (s *Shell) drawMobileActionSurfaces(screen *ebiten.Image, state model.GameState) {
	if s == nil || s.inputController == nil {
		return
	}

	layout := s.inputController.MobileLayout()
	if !layout.Enabled || layout.JoystickRadius <= 0 {
		return
	}

	outer := color.RGBA{R: 200, G: 213, B: 224, A: 74}
	inner := color.RGBA{R: 214, G: 230, B: 247, A: 130}
	vector.DrawFilledCircle(screen, float32(layout.JoystickCenterX), float32(layout.JoystickCenterY), float32(layout.JoystickRadius), outer, false)
	vector.StrokeCircle(screen, float32(layout.JoystickCenterX), float32(layout.JoystickCenterY), float32(layout.JoystickRadius), 2, color.RGBA{R: 223, G: 236, B: 248, A: 150}, false)
	vector.DrawFilledCircle(screen, float32(layout.JoystickCenterX), float32(layout.JoystickCenterY), float32(layout.JoystickRadius*0.38), inner, false)
	text.Draw(screen, "MOVE", basicfont.Face7x13, int(layout.JoystickCenterX)-16, int(layout.JoystickCenterY)+4, color.RGBA{R: 248, G: 251, B: 255, A: 235})

	canFire, canInteract, canReload, canAbility, _ := s.computeActionButtonStates(state)
	s.drawMobileActionButton(screen, layout.FireButton, "FIRE", color.RGBA{R: 196, G: 81, B: 72, A: 196}, canFire)
	s.drawMobileActionButton(screen, layout.InteractButton, "USE", color.RGBA{R: 78, G: 153, B: 102, A: 196}, canInteract)
	s.drawMobileActionButton(screen, layout.AbilityButton, "ABILITY", color.RGBA{R: 133, G: 98, B: 183, A: 196}, canAbility)
	s.drawMobileActionButton(screen, layout.ReloadButton, "RELOAD", color.RGBA{R: 71, G: 111, B: 171, A: 196}, canReload)
}

func (s *Shell) drawMobileActionButton(screen *ebiten.Image, rect input.Rect, label string, fill color.Color, enabled bool) {
	x := float32(rect.MinX)
	y := float32(rect.MinY)
	w := float32(rect.MaxX - rect.MinX)
	h := float32(rect.MaxY - rect.MinY)
	if w <= 0 || h <= 0 {
		return
	}

	buttonFill := color.RGBA{R: 66, G: 74, B: 84, A: 132}
	border := color.RGBA{R: 144, G: 153, B: 162, A: 156}
	labelColor := color.RGBA{R: 193, G: 203, B: 214, A: 190}
	if enabled {
		buttonFill = toRGBA(fill)
		border = color.RGBA{R: 231, G: 239, B: 247, A: 186}
		labelColor = color.RGBA{R: 248, G: 252, B: 255, A: 245}
	}
	vector.DrawFilledRect(screen, x, y, w, h, buttonFill, false)
	vector.StrokeRect(screen, x, y, w, h, 2, border, false)
	labelX := int(rect.MinX + ((rect.MaxX - rect.MinX) / 2) - float64(len(label)*3))
	labelY := int(rect.MinY + ((rect.MaxY - rect.MinY) / 2) + 5)
	text.Draw(screen, label, basicfont.Face7x13, labelX, labelY, labelColor)
}

func (s *Shell) drawDesktopActionIndicators(screen *ebiten.Image, state model.GameState) {
	if s == nil || s.localPlayerID == "" {
		return
	}

	canFire, canInteract, _, canAbility, abilityLabel := s.computeActionButtonStates(state)
	panelWidth := float32(112)
	panelHeight := float32(28)
	gap := float32(10)
	startX := float32(s.screenWidth) - ((panelWidth * 3) + (gap * 2) + 14)
	if startX < 12 {
		startX = 12
	}
	y := float32(s.screenHeight) - panelHeight - 14
	if y < 12 {
		y = 12
	}

	s.drawDesktopActionIndicator(screen, startX, y, panelWidth, panelHeight, "SHOOT", canFire, color.RGBA{R: 188, G: 76, B: 67, A: 224})
	s.drawDesktopActionIndicator(screen, startX+panelWidth+gap, y, panelWidth, panelHeight, "INTERACT", canInteract, color.RGBA{R: 68, G: 142, B: 95, A: 224})
	s.drawDesktopActionIndicator(screen, startX+((panelWidth+gap)*2), y, panelWidth, panelHeight, abilityLabel, canAbility, color.RGBA{R: 133, G: 98, B: 183, A: 224})
}

func (s *Shell) drawDesktopActionIndicator(
	screen *ebiten.Image,
	x float32,
	y float32,
	w float32,
	h float32,
	label string,
	enabled bool,
	active color.RGBA,
) {
	fill := color.RGBA{R: 57, G: 64, B: 73, A: 166}
	border := color.RGBA{R: 118, G: 130, B: 143, A: 186}
	textColor := color.RGBA{R: 189, G: 201, B: 214, A: 198}
	if enabled {
		fill = active
		border = color.RGBA{R: 236, G: 241, B: 247, A: 210}
		textColor = color.RGBA{R: 249, G: 252, B: 255, A: 255}
	}

	vector.DrawFilledRect(screen, x, y, w, h, fill, false)
	vector.StrokeRect(screen, x, y, w, h, 1.5, border, false)
	labelX := int(x + (w / 2) - float32(len(label)*3))
	labelY := int(y + (h / 2) + 5)
	text.Draw(screen, label, basicfont.Face7x13, labelX, labelY, textColor)
}

func toRGBA(in color.Color) color.RGBA {
	if typed, ok := in.(color.RGBA); ok {
		return typed
	}
	r, g, b, a := in.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func (s *Shell) computeActionButtonStates(state model.GameState) (canFire bool, canInteract bool, canReload bool, canAbility bool, abilityLabel string) {
	abilityLabel = "ABILITY"
	if s == nil || s.localPlayerID == "" {
		return false, false, false, false, abilityLabel
	}
	local, found := playerByID(state.Players, s.localPlayerID)
	if !found || !local.Alive || combat.IsActionBlocked(local, state.TickID) {
		return false, false, false, false, abilityLabel
	}

	canFire = local.Bullets > 0
	canReload = local.Bullets < 255
	canInteract = s.hasNearbyInteractable(local, state)
	assigned := local.AssignedAbility
	if assigned == "" {
		for _, fallback := range abilities.AbilitiesForPlayer(local) {
			assigned = fallback
			break
		}
	}
	if assigned != "" {
		abilityLabel = "ABILITY"
		canAbility = s.canUseAssignedAbility(local, state, assigned)
	}
	return canFire, canInteract, canReload, canAbility, abilityLabel
}

func (s *Shell) canUseAssignedAbility(local model.PlayerState, state model.GameState, ability model.AbilityType) bool {
	if !abilities.IsKnownAbility(ability) || !abilities.CanPlayerUse(local, ability) {
		return false
	}

	switch ability {
	case model.AbilityAlarm:
		return state.Phase.Current == model.PhaseDay && !state.Map.Alarm.Active
	case model.AbilitySearch, model.AbilityDetainer, model.AbilityPickPocket:
		return targetPlayerForPanel(local, state.Players) != ""
	case model.AbilityCameraMan:
		return local.CurrentRoomID == gamemap.RoomCameraRoom && state.Map.PowerOn
	case model.AbilityTracker:
		for _, player := range state.Players {
			if player.ID != local.ID && player.Alive {
				return true
			}
		}
		return false
	case model.AbilityLocksmith:
		return state.Map.PowerOn && targetDoorForPanel(local, state.Map) != 0
	default:
		return true
	}
}

func (s *Shell) drawLatestActionFeedback(screen *ebiten.Image, state model.GameState) {
	if s == nil || s.localPlayerID == "" {
		return
	}

	lineY := 74
	if strings.TrimSpace(s.panelLocalHint) != "" {
		hintColor := color.RGBA{R: 208, G: 219, B: 232, A: 242}
		if s.panelLocalHintWarning {
			hintColor = color.RGBA{R: 244, G: 189, B: 133, A: 242}
		}
		text.Draw(
			screen,
			"Hint: "+s.panelLocalHint,
			basicfont.Face7x13,
			16,
			lineY,
			hintColor,
		)
		lineY += 16
	}

	local, found := playerByID(state.Players, s.localPlayerID)
	if !found || local.LastActionFeedback.Kind == "" || local.LastActionFeedback.TickID == 0 {
		return
	}

	message := formatActionFeedback(local.LastActionFeedback)
	if message == "none" {
		return
	}
	text.Draw(
		screen,
		"Event: "+message,
		basicfont.Face7x13,
		16,
		lineY,
		color.RGBA{R: 226, G: 234, B: 243, A: 242},
	)
}

func (s *Shell) drawAbilityInfoPanel(screen *ebiten.Image, state model.GameState) {
	if s == nil || !s.abilityInfoOpen || s.localPlayerID == "" {
		return
	}

	local, found := playerByID(state.Players, s.localPlayerID)
	if !found {
		return
	}

	assigned := local.AssignedAbility
	if assigned == "" {
		for _, fallback := range abilities.AbilitiesForPlayer(local) {
			assigned = fallback
			break
		}
	}

	canUse := false
	if assigned != "" {
		canUse = s.canUseAssignedAbility(local, state, assigned)
	}

	lines := []string{
		"Role and Ability Info",
		fmt.Sprintf("Faction: %s", local.Faction),
		fmt.Sprintf("Role: %s", local.Role),
	}
	if assigned == "" {
		lines = append(lines, "Assigned ability: none")
	} else {
		lines = append(lines, fmt.Sprintf("Assigned ability: %s", assigned))
		lines = append(lines, fmt.Sprintf("Ability ready: %s", boolLabel(canUse)))
		lines = append(lines, abilityUsageHint(assigned))
	}
	if local.LastActionFeedback.Kind != "" && strings.Contains(strings.ToLower(local.LastActionFeedback.Message), "can't use that here") {
		lines = append(lines, "Last attempt: "+local.LastActionFeedback.Message)
	}
	lines = append(lines, "Press V to use ability. Press I to close.")

	panelWidth := clampFloat32(estimatePanelWidth(lines), 360, float32(s.screenWidth)-24)
	panelHeight := float32(20 + (len(lines) * 16))
	panelX := float32(12)
	panelY := float32(96)
	if panelY+panelHeight > float32(s.screenHeight)-12 {
		panelY = float32(s.screenHeight) - panelHeight - 12
	}

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 7, G: 13, B: 19, A: 232}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 1, color.RGBA{R: 90, G: 113, B: 137, A: 255}, false)

	for index, line := range lines {
		text.Draw(
			screen,
			line,
			basicfont.Face7x13,
			int(panelX)+10,
			int(panelY)+20+(index*16),
			color.RGBA{R: 234, G: 241, B: 248, A: 255},
		)
	}
}

func abilityUsageHint(ability model.AbilityType) string {
	switch ability {
	case model.AbilityAlarm:
		return "Use in day phase to trigger alarm lockdown."
	case model.AbilitySearch:
		return "Use near a prisoner in your room to confiscate contraband."
	case model.AbilityCameraMan:
		return "Use in camera room while power is on."
	case model.AbilityDetainer:
		return "Use near a target in your room to force detention."
	case model.AbilityTracker:
		return "Use to mark a living target for tracking."
	case model.AbilityPickPocket:
		return "Use near a target in your room to steal one item."
	case model.AbilityHacker:
		return "Use to toggle power state across the prison."
	case model.AbilityDisguise:
		return "Use to temporarily disguise yourself."
	case model.AbilityLocksmith:
		return "Use near an accessible door while power is on."
	case model.AbilityChameleon:
		return "Use to enter temporary chameleon state."
	default:
		return "Use when your tactical window is open."
	}
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func (s *Shell) hasNearbyInteractable(local model.PlayerState, state model.GameState) bool {
	if local.CurrentRoomID != "" {
		for _, door := range state.Map.Doors {
			if door.ID == 0 {
				continue
			}
			if canViewerTraverseDoor(local, door, state.Map) {
				return true
			}
		}
	}

	for _, entity := range state.Entities {
		if !entity.Active || entity.ID == 0 {
			continue
		}
		if local.CurrentRoomID != "" && entity.RoomID != "" && entity.RoomID != local.CurrentRoomID {
			continue
		}
		dx := entity.Position.X - local.Position.X
		dy := entity.Position.Y - local.Position.Y
		if (dx*dx)+(dy*dy) <= 2.25 {
			return true
		}
	}

	if gamemap.IsPrisonerPlayer(local) {
		for _, route := range escape.EvaluateAllRoutes(local, state.Map) {
			if route.CanAttempt {
				return true
			}
		}
	}

	return false
}

func (s *Shell) TogglePauseMenu() {
	if s == nil {
		return
	}
	s.pauseMenuOpen = !s.pauseMenuOpen
}

func (s *Shell) IsPauseMenuOpen() bool {
	if s == nil {
		return false
	}
	return s.pauseMenuOpen
}

func (s *Shell) filterSnapshotForActionPanels(snapshot input.InputSnapshot) input.InputSnapshot {
	if s == nil {
		return snapshot
	}

	if s.panelSuppressInteract {
		snapshot.InteractPressed = false
	}

	if s.panelSuppressGameplay || actionPanelUsesCenteredModal(s.panelMode) {
		snapshot.MoveUp = false
		snapshot.MoveDown = false
		snapshot.MoveLeft = false
		snapshot.MoveRight = false
		snapshot.Sprint = false
		snapshot.InteractPressed = false
		snapshot.AbilityPressed = false
		snapshot.ReloadPressed = false
		snapshot.FirePressed = false
		snapshot.Touches = nil
	}

	return snapshot
}

func (s *Shell) drawPauseMenu(screen *ebiten.Image) {
	lines := []string{
		"Pause Menu",
		"",
		"Controls",
		"Move: WASD/Arrows | Sprint: Shift",
		"Aim/Fire: Mouse + Space/LMB",
		"Interact: E/F | Ability: V | Ability Info: I | Reload: R",
		"Panels: Tab/C | Escape: X | Stash: H in cell block",
		"Night cards: popup at night (Arrows + Enter)",
		"Modal select: Arrow keys + Enter (Esc/X close)",
		"",
		"Esc or P: Resume",
		"Q: Exit match to menu",
	}

	panelWidth := clampFloat32(estimatePanelWidth(lines), 360, float32(s.screenWidth)-60)
	panelHeight := float32(42 + (len(lines) * 18))
	panelX := (float32(s.screenWidth) - panelWidth) / 2
	panelY := (float32(s.screenHeight) - panelHeight) / 2

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 6, G: 10, B: 14, A: 238}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 2, color.RGBA{R: 99, G: 124, B: 147, A: 255}, false)

	for index, line := range lines {
		text.Draw(
			screen,
			line,
			basicfont.Face7x13,
			int(panelX)+14,
			int(panelY)+24+(index*18),
			color.RGBA{R: 236, G: 243, B: 250, A: 255},
		)
	}
}

func estimatePanelWidth(lines []string) float32 {
	width := 560.0
	for _, line := range lines {
		candidate := float64((len(line) * 7) + 20)
		if candidate > width {
			width = candidate
		}
	}
	return float32(width)
}

func clampFloat32(value float32, min float32, max float32) float32 {
	if max < min {
		return min
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func roomFillColor(roomID model.RoomID, marketRoomID model.RoomID) color.Color {
	if roomID == marketRoomID && roomID != "" {
		return color.RGBA{R: 79, G: 132, B: 55, A: 255}
	}

	switch roomID {
	case gamemap.RoomCorridorMain:
		return color.RGBA{R: 52, G: 60, B: 66, A: 255}
	case gamemap.RoomCellBlockA:
		return color.RGBA{R: 66, G: 68, B: 74, A: 255}
	case gamemap.RoomWardenHQ, gamemap.RoomCameraRoom, gamemap.RoomPowerRoom, gamemap.RoomAmmoRoom:
		return color.RGBA{R: 56, G: 45, B: 47, A: 255}
	case gamemap.RoomCourtyard:
		return color.RGBA{R: 44, G: 68, B: 52, A: 255}
	case gamemap.RoomRoofLookout:
		return color.RGBA{R: 58, G: 62, B: 80, A: 255}
	default:
		return color.RGBA{R: 48, G: 54, B: 62, A: 255}
	}
}

func roomDisplayLabel(roomID model.RoomID) string {
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

func playerFillColor(player model.PlayerState) color.Color {
	if !player.Alive {
		return color.RGBA{R: 83, G: 83, B: 86, A: 255}
	}

	switch player.Faction {
	case model.FactionAuthority:
		return color.RGBA{R: 66, G: 149, B: 214, A: 255}
	case model.FactionPrisoner:
		return color.RGBA{R: 218, G: 126, B: 62, A: 255}
	default:
		return color.RGBA{R: 154, G: 161, B: 170, A: 255}
	}
}

func entityFillColor(kind model.EntityKind) color.Color {
	switch kind {
	case model.EntityKindNPCGuard:
		return color.RGBA{R: 220, G: 75, B: 75, A: 255}
	case model.EntityKindNPCPrisoner:
		return color.RGBA{R: 200, G: 150, B: 70, A: 255}
	case model.EntityKindDroppedItem:
		return color.RGBA{R: 230, G: 196, B: 88, A: 255}
	case model.EntityKindProjectile:
		return color.RGBA{R: 245, G: 245, B: 245, A: 255}
	default:
		return color.RGBA{R: 176, G: 176, B: 176, A: 255}
	}
}

func (s *Shell) captureInputSnapshot() input.InputSnapshot {
	mouseX, mouseY := ebiten.CursorPosition()
	aimWorldX, aimWorldY := s.camera.ScreenToWorld(mouseX, mouseY)
	modalOpen := actionPanelUsesCenteredModal(s.panelMode)
	panelPrevPressed := ebiten.IsKeyPressed(ebiten.KeyBracketLeft) || ebiten.IsKeyPressed(ebiten.KeyPageUp)
	panelNextPressed := ebiten.IsKeyPressed(ebiten.KeyBracketRight) || ebiten.IsKeyPressed(ebiten.KeyPageDown)
	if modalOpen {
		panelPrevPressed = panelPrevPressed || ebiten.IsKeyPressed(ebiten.KeyArrowUp) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft)
		panelNextPressed = panelNextPressed || ebiten.IsKeyPressed(ebiten.KeyArrowDown) || ebiten.IsKeyPressed(ebiten.KeyArrowRight)
	}

	touchIDs := ebiten.AppendTouchIDs(nil)
	sort.Slice(touchIDs, func(i int, j int) bool {
		return touchIDs[i] < touchIDs[j]
	})
	touches := make([]input.TouchPoint, 0, len(touchIDs))
	for _, touchID := range touchIDs {
		x, y := ebiten.TouchPosition(touchID)
		touches = append(touches, input.TouchPoint{
			ID: int64(touchID),
			X:  float64(x),
			Y:  float64(y),
		})
	}

	snapshot := input.InputSnapshot{
		MoveUp:    ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyArrowUp),
		MoveDown:  ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyArrowDown),
		MoveLeft:  ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft),
		MoveRight: ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyArrowRight),
		Sprint:    ebiten.IsKeyPressed(ebiten.KeyShift) || ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight),

		InteractPressed:    ebiten.IsKeyPressed(ebiten.KeyE) || ebiten.IsKeyPressed(ebiten.KeyF),
		AbilityPressed:     ebiten.IsKeyPressed(ebiten.KeyV) || ebiten.IsKeyPressed(ebiten.KeyG),
		AbilityInfoPressed: ebiten.IsKeyPressed(ebiten.KeyI),
		ReloadPressed:      ebiten.IsKeyPressed(ebiten.KeyR),
		FirePressed:        ebiten.IsKeyPressed(ebiten.KeySpace) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),

		HasAim:    true,
		AimWorldX: aimWorldX,
		AimWorldY: aimWorldY,

		PanelInventoryPressed: ebiten.IsKeyPressed(ebiten.KeyTab),
		PanelCardsPressed:     ebiten.IsKeyPressed(ebiten.KeyC),
		PanelAbilitiesPressed: false,
		PanelMarketPressed:    ebiten.IsKeyPressed(ebiten.KeyB) || ebiten.IsKeyPressed(ebiten.KeyM),
		PanelEscapePressed:    ebiten.IsKeyPressed(ebiten.KeyX),
		PanelStashPressed:     ebiten.IsKeyPressed(ebiten.KeyH),
		PanelPrevPressed:      panelPrevPressed,
		PanelNextPressed:      panelNextPressed,
		PanelUsePressed:       ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeyKPEnter),
		SpectatorPrevPressed:  ebiten.IsKeyPressed(ebiten.KeyQ) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft),
		SpectatorNextPressed:  ebiten.IsKeyPressed(ebiten.KeyE) || ebiten.IsKeyPressed(ebiten.KeyArrowRight),

		Touches: touches,
	}
	return s.augmentSnapshotWithPanelTouches(snapshot)
}
