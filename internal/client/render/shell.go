package render

import (
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
	panelInputPrev          panelInputEdgeState
	spectatorInputPrev      spectatorInputEdgeState
	spectatorFollowPlayerID model.PlayerID
	spectatorFollowSlot     int
	spectatorSlotCount      int
	pauseMenuOpen           bool
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
	camera.TilePixels = 28
	camera.Zoom = 1.15

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
	if hasLocal {
		localCopy := local
		localPtr = &localCopy
	}

	commands := s.inputController.BuildCommands(snapshot, state.TickID+1, localPtr)
	panelCommands := s.updateActionPanelCommands(snapshot, state, localPtr)
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

func (s *Shell) drawRooms(screen *ebiten.Image, state model.GameState) {
	for _, room := range s.layout.Rooms() {
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
	doorByID := make(map[model.DoorID]model.DoorState, len(state.Map.Doors))
	for _, door := range state.Map.Doors {
		doorByID[door.ID] = door
	}

	for _, doorLink := range s.layout.DoorLinks() {
		doorState, exists := doorByID[doorLink.ID]
		open := !exists || doorState.Open

		center := model.Vector2{X: float32(doorLink.Position.X) + 0.5, Y: float32(doorLink.Position.Y) + 0.5}
		x, y, w, h := s.camera.TileRectToScreen(float64(center.X)-0.18, float64(center.Y)-0.18, 0.36, 0.36)

		fill := color.RGBA{R: 210, G: 66, B: 66, A: 255}
		if open {
			fill = color.RGBA{R: 88, G: 198, B: 121, A: 255}
		}
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), fill, false)
	}
}

func (s *Shell) drawPlayers(screen *ebiten.Image, state model.GameState) {
	for _, player := range state.Players {
		size := 0.72
		x, y, w, h := s.camera.TileRectToScreen(float64(player.Position.X)-(size/2), float64(player.Position.Y)-(size/2), size, size)

		fill := playerFillColor(player)
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
		text.Draw(screen, string(player.ID), basicfont.Face7x13, labelX, labelY, color.RGBA{R: 232, G: 236, B: 243, A: 255})
	}
}

func (s *Shell) drawEntities(screen *ebiten.Image, state model.GameState) {
	for _, entity := range state.Entities {
		if !entity.Active {
			continue
		}
		size := 0.45
		x, y, w, h := s.camera.TileRectToScreen(float64(entity.Position.X)-(size/2), float64(entity.Position.Y)-(size/2), size, size)
		vector.DrawFilledRect(screen, float32(x), float32(y), float32(w), float32(h), entityFillColor(entity.Kind), false)
	}
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

	if showMobileHints {
		s.drawMobileActionSurfaces(screen, state)
	}
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

	canFire, canInteract, canReload := s.computeActionButtonStates(state)
	s.drawMobileActionButton(screen, layout.FireButton, "FIRE", color.RGBA{R: 196, G: 81, B: 72, A: 196}, canFire)
	s.drawMobileActionButton(screen, layout.InteractButton, "USE", color.RGBA{R: 78, G: 153, B: 102, A: 196}, canInteract)
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

func (s *Shell) computeActionButtonStates(state model.GameState) (canFire bool, canInteract bool, canReload bool) {
	if s == nil || s.localPlayerID == "" {
		return false, false, false
	}
	local, found := playerByID(state.Players, s.localPlayerID)
	if !found || !local.Alive || combat.IsActionBlocked(local, state.TickID) {
		return false, false, false
	}

	canFire = local.Bullets > 0
	canReload = local.Bullets < 255
	canInteract = s.hasNearbyInteractable(local, state)
	return canFire, canInteract, canReload
}

func (s *Shell) hasNearbyInteractable(local model.PlayerState, state model.GameState) bool {
	if local.CurrentRoomID != "" {
		for _, door := range state.Map.Doors {
			if door.ID == 0 {
				continue
			}
			if door.RoomA == local.CurrentRoomID || door.RoomB == local.CurrentRoomID {
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

func (s *Shell) drawPauseMenu(screen *ebiten.Image) {
	lines := []string{
		"Pause Menu",
		"",
		"Controls",
		"Move: WASD/Arrows | Sprint: Shift",
		"Aim/Fire: Mouse + Space/LMB",
		"Interact: E/F | Reload: R",
		"Panels: Tab/C/V/B/X + [ ] + Enter",
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

		InteractPressed: ebiten.IsKeyPressed(ebiten.KeyE) || ebiten.IsKeyPressed(ebiten.KeyF),
		ReloadPressed:   ebiten.IsKeyPressed(ebiten.KeyR),
		FirePressed:     ebiten.IsKeyPressed(ebiten.KeySpace) || ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),

		HasAim:    true,
		AimWorldX: aimWorldX,
		AimWorldY: aimWorldY,

		PanelInventoryPressed: ebiten.IsKeyPressed(ebiten.KeyTab),
		PanelCardsPressed:     ebiten.IsKeyPressed(ebiten.KeyC),
		PanelAbilitiesPressed: ebiten.IsKeyPressed(ebiten.KeyV),
		PanelMarketPressed:    ebiten.IsKeyPressed(ebiten.KeyB) || ebiten.IsKeyPressed(ebiten.KeyM),
		PanelEscapePressed:    ebiten.IsKeyPressed(ebiten.KeyX),
		PanelPrevPressed:      ebiten.IsKeyPressed(ebiten.KeyBracketLeft) || ebiten.IsKeyPressed(ebiten.KeyPageUp),
		PanelNextPressed:      ebiten.IsKeyPressed(ebiten.KeyBracketRight) || ebiten.IsKeyPressed(ebiten.KeyPageDown),
		PanelUsePressed:       ebiten.IsKeyPressed(ebiten.KeyEnter) || ebiten.IsKeyPressed(ebiten.KeyKPEnter),
		SpectatorPrevPressed:  ebiten.IsKeyPressed(ebiten.KeyQ) || ebiten.IsKeyPressed(ebiten.KeyArrowLeft),
		SpectatorNextPressed:  ebiten.IsKeyPressed(ebiten.KeyE) || ebiten.IsKeyPressed(ebiten.KeyArrowRight),

		Touches: touches,
	}
	return s.augmentSnapshotWithPanelTouches(snapshot)
}
