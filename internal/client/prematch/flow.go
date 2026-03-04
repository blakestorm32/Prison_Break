package prematch

import (
	"fmt"
	"sort"
	"strings"

	"prison-break/internal/shared/model"
	"prison-break/internal/shared/protocol"
)

type Stage string

const (
	StageMainMenu    Stage = "main_menu"
	StageLobbyList   Stage = "lobby_list"
	StageTutorial    Stage = "tutorial"
	StageConnecting  Stage = "connecting"
	StageLobbyWait   Stage = "lobby_wait"
	StageInMatch     Stage = "in_match"
	StageErrorNotice Stage = "error_notice"
)

type MenuOption int

const (
	MenuOptionQuickPlay MenuOption = iota
	MenuOptionBrowseLobbies
	MenuOptionSpectateMatch
	MenuOptionTutorialCodex
)

type ConnectIntent struct {
	PreferredMatchID model.MatchID
	Spectator        bool
}

type LobbyStatus struct {
	MatchID     model.MatchID
	Status      model.MatchStatus
	PlayerCount uint8
	MinPlayers  uint8
	MaxPlayers  uint8
}

func (s LobbyStatus) ReadyToStart() bool {
	if s.MinPlayers == 0 {
		return false
	}
	return s.PlayerCount >= s.MinPlayers
}

type Flow struct {
	stage Stage

	menuOptions []MenuOption
	menuIndex   int

	lobbies       []protocol.LobbySummary
	selectedLobby int
	tutorialPage  int

	lobbyStatus LobbyStatus
	lastError   string
}

func NewFlow() *Flow {
	return &Flow{
		stage: StageMainMenu,
		menuOptions: []MenuOption{
			MenuOptionQuickPlay,
			MenuOptionBrowseLobbies,
			MenuOptionSpectateMatch,
			MenuOptionTutorialCodex,
		},
	}
}

func (f *Flow) Stage() Stage {
	if f == nil {
		return StageMainMenu
	}
	return f.stage
}

func (f *Flow) LastError() string {
	if f == nil {
		return ""
	}
	return f.lastError
}

func (f *Flow) MenuOptions() []MenuOption {
	if f == nil {
		return nil
	}
	out := make([]MenuOption, len(f.menuOptions))
	copy(out, f.menuOptions)
	return out
}

func MenuOptionLabel(option MenuOption) string {
	switch option {
	case MenuOptionQuickPlay:
		return "Quick Play (create/join)"
	case MenuOptionBrowseLobbies:
		return "Browse Lobbies"
	case MenuOptionSpectateMatch:
		return "Spectate Match"
	case MenuOptionTutorialCodex:
		return "Tutorial / Rules Codex"
	default:
		return "Unknown"
	}
}

func (f *Flow) MenuIndex() int {
	if f == nil {
		return 0
	}
	return f.menuIndex
}

func (f *Flow) MoveMenuSelection(delta int) {
	if f == nil || f.stage != StageMainMenu || len(f.menuOptions) == 0 || delta == 0 {
		return
	}

	count := len(f.menuOptions)
	next := f.menuIndex + delta
	for next < 0 {
		next += count
	}
	f.menuIndex = next % count
}

func (f *Flow) ActivateMenuSelection() (ConnectIntent, bool) {
	if f == nil || f.stage != StageMainMenu || len(f.menuOptions) == 0 {
		return ConnectIntent{}, false
	}

	selected := f.menuOptions[f.menuIndex]
	switch selected {
	case MenuOptionQuickPlay:
		f.stage = StageConnecting
		f.lastError = ""
		return ConnectIntent{}, true
	case MenuOptionBrowseLobbies:
		f.stage = StageLobbyList
		f.lastError = ""
		f.selectedLobby = 0
		return ConnectIntent{}, false
	case MenuOptionSpectateMatch:
		f.stage = StageConnecting
		f.lastError = ""
		return ConnectIntent{Spectator: true}, true
	case MenuOptionTutorialCodex:
		f.stage = StageTutorial
		f.lastError = ""
		f.tutorialPage = 0
		return ConnectIntent{}, false
	default:
		return ConnectIntent{}, false
	}
}

func (f *Flow) BackToMainMenu() {
	if f == nil {
		return
	}
	f.stage = StageMainMenu
}

func (f *Flow) MoveTutorialPage(delta int, pageCount int) {
	if f == nil || f.stage != StageTutorial || pageCount <= 0 || delta == 0 {
		return
	}

	next := f.tutorialPage + delta
	for next < 0 {
		next += pageCount
	}
	f.tutorialPage = next % pageCount
}

func (f *Flow) TutorialPage() int {
	if f == nil {
		return 0
	}
	return f.tutorialPage
}

func (f *Flow) SetLobbies(lobbies []protocol.LobbySummary) {
	if f == nil {
		return
	}

	f.lobbies = append([]protocol.LobbySummary(nil), lobbies...)
	sort.SliceStable(f.lobbies, func(i int, j int) bool {
		if f.lobbies[i].Joinable != f.lobbies[j].Joinable {
			return f.lobbies[i].Joinable
		}
		if f.lobbies[i].ReadyToStart != f.lobbies[j].ReadyToStart {
			return f.lobbies[i].ReadyToStart
		}
		if f.lobbies[i].PlayerCount != f.lobbies[j].PlayerCount {
			return f.lobbies[i].PlayerCount > f.lobbies[j].PlayerCount
		}
		return f.lobbies[i].MatchID < f.lobbies[j].MatchID
	})

	if len(f.lobbies) == 0 {
		f.selectedLobby = 0
		return
	}
	if f.selectedLobby < 0 {
		f.selectedLobby = 0
	}
	if f.selectedLobby >= len(f.lobbies) {
		f.selectedLobby = len(f.lobbies) - 1
	}
}

func (f *Flow) Lobbies() []protocol.LobbySummary {
	if f == nil {
		return nil
	}
	out := make([]protocol.LobbySummary, len(f.lobbies))
	copy(out, f.lobbies)
	return out
}

func (f *Flow) MoveLobbySelection(delta int) {
	if f == nil || f.stage != StageLobbyList || len(f.lobbies) == 0 || delta == 0 {
		return
	}

	count := len(f.lobbies)
	next := f.selectedLobby + delta
	for next < 0 {
		next += count
	}
	f.selectedLobby = next % count
}

func (f *Flow) SelectedLobby() (protocol.LobbySummary, bool) {
	if f == nil || len(f.lobbies) == 0 || f.selectedLobby < 0 || f.selectedLobby >= len(f.lobbies) {
		return protocol.LobbySummary{}, false
	}
	return f.lobbies[f.selectedLobby], true
}

func (f *Flow) BeginJoinSelectedLobby() (ConnectIntent, error) {
	if f == nil || f.stage != StageLobbyList {
		return ConnectIntent{}, fmt.Errorf("prematch: lobby selection not available")
	}
	selected, ok := f.SelectedLobby()
	if !ok {
		return ConnectIntent{}, fmt.Errorf("prematch: no lobbies available")
	}
	if selected.MatchID == "" {
		return ConnectIntent{}, fmt.Errorf("prematch: selected lobby has empty match id")
	}
	f.stage = StageConnecting
	f.lastError = ""
	return ConnectIntent{
		PreferredMatchID: selected.MatchID,
		Spectator:        false,
	}, nil
}

func (f *Flow) BeginSpectateSelectedLobby() (ConnectIntent, error) {
	if f == nil || f.stage != StageLobbyList {
		return ConnectIntent{}, fmt.Errorf("prematch: lobby selection not available")
	}
	selected, ok := f.SelectedLobby()
	if !ok {
		return ConnectIntent{}, fmt.Errorf("prematch: no lobbies available")
	}
	if selected.MatchID == "" {
		return ConnectIntent{}, fmt.Errorf("prematch: selected lobby has empty match id")
	}
	f.stage = StageConnecting
	f.lastError = ""
	return ConnectIntent{
		PreferredMatchID: selected.MatchID,
		Spectator:        true,
	}, nil
}

func (f *Flow) OnConnectError(err error) {
	if f == nil {
		return
	}

	f.stage = StageErrorNotice
	if err == nil {
		f.lastError = "connection failed"
		return
	}
	message := strings.TrimSpace(err.Error())
	if message == "" {
		message = "connection failed"
	}
	f.lastError = message
}

func (f *Flow) OnJoined(lobby LobbyStatus) {
	if f == nil {
		return
	}

	f.lastError = ""
	f.lobbyStatus = lobby
	if lobby.Status == model.MatchStatusRunning {
		f.stage = StageInMatch
		return
	}
	f.stage = StageLobbyWait
}

func (f *Flow) OnLobbySnapshot(status model.MatchStatus, playerCount uint8) {
	if f == nil {
		return
	}
	f.lobbyStatus.Status = status
	f.lobbyStatus.PlayerCount = playerCount
	if f.stage == StageLobbyWait && status == model.MatchStatusRunning {
		f.stage = StageInMatch
	}
}

func (f *Flow) LobbyStatus() LobbyStatus {
	if f == nil {
		return LobbyStatus{}
	}
	return f.lobbyStatus
}
