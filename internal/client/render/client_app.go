package render

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"log"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"

	"prison-break/internal/client/netclient"
	"prison-break/internal/client/onboarding"
	"prison-break/internal/client/prematch"
	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

type ClientAppConfig struct {
	ScreenWidth  int
	ScreenHeight int

	SessionConfig netclient.SessionConfig
}

type connectResult struct {
	attemptID uint64
	session   *netclient.Session
	err       error
}

type lobbyFetchResult struct {
	attemptID uint64
	lobbies   []protocol.LobbySummary
	err       error
}

type ClientApp struct {
	screenWidth  int
	screenHeight int

	sessionConfig netclient.SessionConfig
	flow          *prematch.Flow
	lastStage     prematch.Stage

	session *netclient.Session
	shell   *Shell

	connectAttemptID uint64
	connectCancel    context.CancelFunc
	connectResults   chan connectResult

	lobbyFetchAttemptID uint64
	lobbyFetchResults   chan lobbyFetchResult

	codexPages []onboarding.CodexPage

	lastSendWarnAt time.Time
}

func NewClientApp(config ClientAppConfig) *ClientApp {
	screenWidth := config.ScreenWidth
	if screenWidth <= 0 {
		screenWidth = defaultScreenWidth
	}
	screenHeight := config.ScreenHeight
	if screenHeight <= 0 {
		screenHeight = defaultScreenHeight
	}

	flow := prematch.NewFlow()
	sessionCfg := config.SessionConfig
	defaults := netclient.DefaultSessionConfig()
	if strings.TrimSpace(sessionCfg.ServerURL) == "" {
		sessionCfg.ServerURL = defaults.ServerURL
	}
	if strings.TrimSpace(sessionCfg.PlayerName) == "" {
		sessionCfg.PlayerName = defaults.PlayerName
	}
	if sessionCfg.HandshakeTimeout <= 0 {
		sessionCfg.HandshakeTimeout = defaults.HandshakeTimeout
	}
	if sessionCfg.WriteTimeout <= 0 {
		sessionCfg.WriteTimeout = defaults.WriteTimeout
	}
	if sessionCfg.SendQueueDepth <= 0 {
		sessionCfg.SendQueueDepth = defaults.SendQueueDepth
	}

	return &ClientApp{
		screenWidth:         screenWidth,
		screenHeight:        screenHeight,
		sessionConfig:       sessionCfg,
		flow:                flow,
		lastStage:           flow.Stage(),
		connectResults:      make(chan connectResult, 4),
		lobbyFetchResults:   make(chan lobbyFetchResult, 4),
		codexPages:          onboarding.Pages(),
		lastSendWarnAt:      time.Time{},
		connectAttemptID:    0,
		lobbyFetchAttemptID: 0,
	}
}

func (a *ClientApp) Update() error {
	if a == nil || a.flow == nil {
		return nil
	}

	a.processAsyncResults()
	a.syncStageFromSession()
	a.handleStageTransition()

	switch a.flow.Stage() {
	case prematch.StageMainMenu:
		a.updateMainMenu()
	case prematch.StageLobbyList:
		a.updateLobbyList()
	case prematch.StageTutorial:
		a.updateTutorial()
	case prematch.StageConnecting:
		a.updateConnecting()
	case prematch.StageLobbyWait:
		a.updateLobbyWait()
	case prematch.StageInMatch:
		return a.updateInMatch()
	case prematch.StageErrorNotice:
		a.updateErrorNotice()
	}

	return nil
}

func (a *ClientApp) Draw(screen *ebiten.Image) {
	if a == nil || a.flow == nil {
		screen.Fill(color.RGBA{R: 10, G: 12, B: 15, A: 255})
		return
	}

	if a.flow.Stage() == prematch.StageInMatch && a.shell != nil {
		a.shell.Draw(screen)
		return
	}

	screen.Fill(color.RGBA{R: 9, G: 14, B: 19, A: 255})

	lines := a.buildPreMatchLines()
	x := 28
	y := 36
	for _, line := range lines {
		text.Draw(screen, line, basicfont.Face7x13, x, y, color.RGBA{R: 232, G: 239, B: 246, A: 255})
		y += 18
	}
}

func (a *ClientApp) Layout(_, _ int) (int, int) {
	if a != nil && a.flow != nil && a.flow.Stage() == prematch.StageInMatch && a.shell != nil {
		return a.shell.Layout(0, 0)
	}
	return a.screenWidth, a.screenHeight
}

func (a *ClientApp) updateMainMenu() {
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		a.flow.MoveMenuSelection(-1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		a.flow.MoveMenuSelection(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		intent, shouldConnect := a.flow.ActivateMenuSelection()
		if shouldConnect {
			a.startConnect(intent)
		}
	}
}

func (a *ClientApp) updateTutorial() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		a.flow.BackToMainMenu()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		a.flow.MoveTutorialPage(-1, len(a.codexPages))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		a.flow.MoveTutorialPage(1, len(a.codexPages))
	}
}

func (a *ClientApp) updateLobbyList() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		a.flow.BackToMainMenu()
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		a.refreshLobbies()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		a.flow.MoveLobbySelection(-1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		a.flow.MoveLobbySelection(1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		intent, err := a.flow.BeginJoinSelectedLobby()
		if err != nil {
			a.flow.OnConnectError(err)
			return
		}
		a.startConnect(intent)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF) {
		intent, err := a.flow.BeginSpectateSelectedLobby()
		if err != nil {
			a.flow.OnConnectError(err)
			return
		}
		a.startConnect(intent)
	}
}

func (a *ClientApp) updateConnecting() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.cancelActiveConnect()
		a.flow.BackToMainMenu()
	}
}

func (a *ClientApp) updateLobbyWait() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.leaveSessionToMenu()
	}
}

func (a *ClientApp) updateErrorNotice() {
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		a.flow.BackToMainMenu()
	}
}

func (a *ClientApp) updateInMatch() error {
	if a.shell == nil {
		return nil
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyP) {
		a.shell.TogglePauseMenu()
		return nil
	}
	if a.shell.IsPauseMenuOpen() {
		if inpututil.IsKeyJustPressed(ebiten.KeyQ) {
			a.leaveSessionToMenu()
		}
		return nil
	}
	return a.shell.Update()
}

func (a *ClientApp) handleStageTransition() {
	stage := a.flow.Stage()
	if stage == a.lastStage {
		return
	}

	if stage == prematch.StageLobbyList {
		a.refreshLobbies()
	}

	if stage == prematch.StageMainMenu {
		a.cancelActiveConnect()
	}

	a.lastStage = stage
}

func (a *ClientApp) processAsyncResults() {
	for {
		select {
		case result := <-a.lobbyFetchResults:
			if result.attemptID != a.lobbyFetchAttemptID {
				continue
			}
			if result.err != nil {
				a.flow.OnConnectError(result.err)
				continue
			}
			a.flow.SetLobbies(result.lobbies)
		case result := <-a.connectResults:
			if result.attemptID != a.connectAttemptID {
				if result.session != nil {
					_ = result.session.Close()
				}
				continue
			}
			a.connectCancel = nil

			if result.err != nil {
				a.flow.OnConnectError(result.err)
				continue
			}
			a.onConnectSuccess(result.session)
		default:
			return
		}
	}
}

func (a *ClientApp) onConnectSuccess(session *netclient.Session) {
	if session == nil {
		a.flow.OnConnectError(errors.New("connection succeeded without session"))
		return
	}

	if a.session != nil {
		_ = a.session.Close()
	}
	a.session = session
	a.shell = nil
	if session.LocalPlayerID() != "" {
		a.sessionConfig.PlayerID = session.LocalPlayerID()
	}

	state, ok := session.Store().CurrentState()
	if !ok {
		a.flow.OnConnectError(errors.New("session did not provide initial snapshot"))
		return
	}

	lobbyState := prematch.LobbyStatus{
		MatchID:     session.MatchID(),
		Status:      state.Status,
		PlayerCount: uint8(len(state.Players)),
		MinPlayers:  session.MinPlayers(),
		MaxPlayers:  session.MaxPlayers(),
	}
	a.flow.OnJoined(lobbyState)

	if a.flow.Stage() == prematch.StageInMatch {
		a.ensureShell()
	}
}

func (a *ClientApp) syncStageFromSession() {
	if a.session == nil {
		return
	}

	if asyncErr := a.session.LastAsyncError(); asyncErr != nil {
		a.leaveSessionOnly()
		a.flow.OnConnectError(asyncErr)
		return
	}

	state, ok := a.session.Store().CurrentState()
	if !ok {
		return
	}

	a.flow.OnLobbySnapshot(state.Status, uint8(len(state.Players)))
	if a.flow.Stage() == prematch.StageInMatch {
		a.ensureShell()
	}
}

func (a *ClientApp) ensureShell() {
	if a.session == nil || a.shell != nil {
		return
	}

	a.shell = NewShell(ShellConfig{
		ScreenWidth:             a.screenWidth,
		ScreenHeight:            a.screenHeight,
		LocalPlayerID:           a.session.LocalPlayerID(),
		SpectatorFollowPlayerID: a.session.SpectatorFollowPlayerID(),
		SpectatorFollowSlot:     a.session.SpectatorFollowSlot(),
		SpectatorSlotCount:      a.session.SpectatorSlotCount(),
		Store:                   a.session.Store(),
		OnInputCommand: func(command model.InputCommand) {
			if err := a.session.SendInputCommand(command); err != nil {
				if errors.Is(err, netclient.ErrReadOnlySession) {
					return
				}

				now := time.Now()
				if a.lastSendWarnAt.IsZero() || now.Sub(a.lastSendWarnAt) >= 750*time.Millisecond {
					log.Printf("input command dispatch warning: %v", err)
					a.lastSendWarnAt = now
				}
			}
		},
	})
}

func (a *ClientApp) startConnect(intent prematch.ConnectIntent) {
	a.cancelActiveConnect()
	a.leaveSessionOnly()

	a.connectAttemptID++
	attemptID := a.connectAttemptID
	cfg := a.sessionConfig
	if intent.PreferredMatchID != "" {
		cfg.PreferredMatchID = intent.PreferredMatchID
	}
	cfg.Spectator = intent.Spectator
	if cfg.Spectator && cfg.PreferredMatchID == "" {
		a.flow.OnConnectError(errors.New("spectator join requires PRISON_PREFERRED_MATCH_ID or selected lobby"))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	a.connectCancel = cancel

	go func() {
		session, err := netclient.DialAndJoin(ctx, cfg)
		result := connectResult{
			attemptID: attemptID,
			session:   session,
			err:       err,
		}
		select {
		case a.connectResults <- result:
		default:
			if session != nil {
				_ = session.Close()
			}
		}
	}()
}

func (a *ClientApp) refreshLobbies() {
	a.lobbyFetchAttemptID++
	attemptID := a.lobbyFetchAttemptID
	serverURL := a.sessionConfig.ServerURL
	authToken := a.sessionConfig.AuthToken
	preferredRegion := a.sessionConfig.PreferredRegion
	regionLatencyMS := a.sessionConfig.RegionLatencyMS

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	go func() {
		defer cancel()
		lobbies, err := netclient.FetchLobbiesWithPreferencesAndAuth(
			ctx,
			serverURL,
			false,
			authToken,
			preferredRegion,
			regionLatencyMS,
		)
		result := lobbyFetchResult{
			attemptID: attemptID,
			lobbies:   lobbies,
			err:       err,
		}
		select {
		case a.lobbyFetchResults <- result:
		default:
		}
	}()
}

func (a *ClientApp) cancelActiveConnect() {
	if a.connectCancel != nil {
		a.connectCancel()
		a.connectCancel = nil
	}
}

func (a *ClientApp) leaveSessionOnly() {
	if a.session != nil {
		_ = a.session.Close()
	}
	a.session = nil
	a.shell = nil
	a.lastSendWarnAt = time.Time{}
}

func (a *ClientApp) leaveSessionToMenu() {
	a.cancelActiveConnect()
	a.leaveSessionOnly()
	a.flow.BackToMainMenu()
}

func (a *ClientApp) buildPreMatchLines() []string {
	stage := a.flow.Stage()
	switch stage {
	case prematch.StageMainMenu:
		return a.mainMenuLines()
	case prematch.StageLobbyList:
		return a.lobbyListLines()
	case prematch.StageTutorial:
		return a.tutorialLines()
	case prematch.StageConnecting:
		return []string{
			"Prison Break - Connecting",
			"",
			"Connecting to server and joining lobby...",
			"Press Esc to cancel.",
		}
	case prematch.StageLobbyWait:
		return a.lobbyWaitLines()
	case prematch.StageErrorNotice:
		return []string{
			"Prison Break - Error",
			"",
			"Connection or lobby operation failed:",
			a.flow.LastError(),
			"",
			"Press Enter or Esc to return to menu.",
		}
	case prematch.StageInMatch:
		return []string{"Launching match..."}
	default:
		return []string{"Prison Break"}
	}
}

func (a *ClientApp) mainMenuLines() []string {
	lines := []string{
		"Prison Break",
		"",
		fmt.Sprintf("Player: %s", nonEmptyOrFallback(strings.TrimSpace(a.sessionConfig.PlayerName), "Player")),
		fmt.Sprintf("Server: %s", strings.TrimSpace(a.sessionConfig.ServerURL)),
		fmt.Sprintf("Preferred Region: %s", nonEmptyOrFallback(strings.TrimSpace(a.sessionConfig.PreferredRegion), "auto")),
		"",
		"Main Menu",
	}
	for index, option := range a.flow.MenuOptions() {
		prefix := "  "
		if index == a.flow.MenuIndex() {
			prefix = "> "
		}
		lines = append(lines, prefix+prematch.MenuOptionLabel(option))
	}
	lines = append(lines, "", "Controls: Up/Down to select, Enter to continue")
	return lines
}

func (a *ClientApp) lobbyListLines() []string {
	lines := []string{
		"Lobby Browser",
		"",
		"Select a lobby to join.",
		"Controls: Up/Down move, Enter join player, F spectate, R refresh, Esc back",
		"",
	}

	lobbies := a.flow.Lobbies()
	if len(lobbies) == 0 {
		lines = append(lines, "No lobbies available yet. Press R to refresh or Esc to go back.")
		return lines
	}

	selected, _ := a.flow.SelectedLobby()
	for _, lobby := range lobbies {
		prefix := "  "
		if lobby.MatchID == selected.MatchID {
			prefix = "> "
		}
		readyLabel := "waiting"
		if lobby.ReadyToStart {
			readyLabel = "ready"
		}
		lines = append(
			lines,
			fmt.Sprintf(
				"%s%s | %s | players %d/%d | %s",
				prefix,
				lobby.MatchID,
				nonEmptyOrFallback(strings.TrimSpace(lobby.Region), "global"),
				lobby.PlayerCount,
				lobby.MaxPlayers,
				readyLabel,
			),
		)
	}
	return lines
}

func (a *ClientApp) lobbyWaitLines() []string {
	status := a.flow.LobbyStatus()
	readyLabel := "waiting for more players"
	if status.ReadyToStart() {
		readyLabel = "ready - match will start automatically"
	}

	lines := []string{
		"Lobby",
		"",
		fmt.Sprintf("Match: %s", status.MatchID),
		fmt.Sprintf("Players: %d/%d (min %d)", status.PlayerCount, status.MaxPlayers, status.MinPlayers),
		fmt.Sprintf("Status: %s", nonEmptyOrFallback(string(status.Status), "unknown")),
		fmt.Sprintf("Start: %s", readyLabel),
		"",
		"Waiting for game_start...",
		"Press Esc to leave lobby and return to menu.",
	}
	return lines
}

func (a *ClientApp) tutorialLines() []string {
	if len(a.codexPages) == 0 {
		return []string{
			"Tutorial / Rules Codex",
			"",
			"No codex pages available.",
			"Press Esc to return to menu.",
		}
	}

	index := a.flow.TutorialPage()
	if index < 0 || index >= len(a.codexPages) {
		index = 0
	}
	page := a.codexPages[index]
	lines := []string{
		"Tutorial / Rules Codex",
		fmt.Sprintf("Page %d/%d: %s", index+1, len(a.codexPages), page.Title),
		"",
	}
	lines = append(lines, page.Lines...)
	lines = append(lines, "", "Controls: Left/Right move page, Esc back to menu")
	return lines
}

func nonEmptyOrFallback(value string, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
