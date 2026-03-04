package render

import (
	"fmt"
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font/basicfont"

	"prison-break/internal/client/input"
	"prison-break/internal/shared/model"
)

func (s *Shell) resolveCameraFocusPlayer(state model.GameState) (model.PlayerState, bool) {
	if s == nil {
		return model.PlayerState{}, false
	}

	if s.localPlayerID != "" {
		if local, ok := playerByID(state.Players, s.localPlayerID); ok {
			return local, true
		}
	}

	players := orderedSpectatorPlayers(state.Players)
	if len(players) == 0 {
		return model.PlayerState{}, false
	}

	index := s.resolveSpectatorFollowIndex(players)
	return players[index], true
}

func (s *Shell) updateSpectatorFollowSelection(state model.GameState, snapshot input.InputSnapshot) {
	if s == nil || s.localPlayerID != "" {
		return
	}

	current := spectatorInputEdgeState{
		prev: snapshot.SpectatorPrevPressed,
		next: snapshot.SpectatorNextPressed,
	}
	previous := s.spectatorInputPrev
	s.spectatorInputPrev = current

	players := orderedSpectatorPlayers(state.Players)
	if len(players) == 0 {
		s.spectatorFollowPlayerID = ""
		s.spectatorFollowSlot = 0
		s.spectatorSlotCount = 0
		return
	}

	index := s.resolveSpectatorFollowIndex(players)
	if current.next && !previous.next {
		index = (index + 1) % len(players)
	}
	if current.prev && !previous.prev {
		index = (index - 1 + len(players)) % len(players)
	}

	s.spectatorFollowPlayerID = players[index].ID
	s.spectatorFollowSlot = index + 1
	s.spectatorSlotCount = len(players)
}

func (s *Shell) resolveSpectatorFollowIndex(players []model.PlayerState) int {
	if len(players) == 0 {
		s.spectatorFollowPlayerID = ""
		s.spectatorFollowSlot = 0
		s.spectatorSlotCount = 0
		return 0
	}

	if s.spectatorFollowPlayerID != "" {
		for index, player := range players {
			if player.ID == s.spectatorFollowPlayerID {
				s.spectatorFollowSlot = index + 1
				s.spectatorSlotCount = len(players)
				return index
			}
		}
	}

	if s.spectatorFollowSlot > 0 && s.spectatorFollowSlot <= len(players) {
		index := s.spectatorFollowSlot - 1
		s.spectatorFollowPlayerID = players[index].ID
		s.spectatorSlotCount = len(players)
		return index
	}

	s.spectatorFollowPlayerID = players[0].ID
	s.spectatorFollowSlot = 1
	s.spectatorSlotCount = len(players)
	return 0
}

func orderedSpectatorPlayers(players []model.PlayerState) []model.PlayerState {
	if len(players) == 0 {
		return nil
	}
	out := append([]model.PlayerState(nil), players...)
	sort.Slice(out, func(i int, j int) bool {
		if out[i].Alive != out[j].Alive {
			return out[i].Alive
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (s *Shell) drawSpectatorOverlay(screen *ebiten.Image, state model.GameState) {
	if s == nil || s.localPlayerID != "" {
		return
	}

	players := orderedSpectatorPlayers(state.Players)
	if len(players) == 0 {
		return
	}

	selectedIndex := s.resolveSpectatorFollowIndex(players)
	lines := make([]string, 0, len(players)+2)
	lines = append(lines, fmt.Sprintf("Spectator Follow %d/%d", selectedIndex+1, len(players)))
	lines = append(lines, "Q/E or Left/Right: switch player")
	for index, player := range players {
		prefix := " "
		if index == selectedIndex {
			prefix = ">"
		}
		status := "down"
		if player.Alive {
			status = "alive"
		}
		lines = append(
			lines,
			fmt.Sprintf(
				"%s %d. %s  hp:%s  room:%s  %s",
				prefix,
				index+1,
				player.ID,
				formatHearts(player.HeartsHalf),
				roomOrUnknown(player.CurrentRoomID),
				status,
			),
		)
	}

	panelWidth := clampFloat32(estimatePanelWidth(lines), 420, float32(s.screenWidth)-24)
	panelX := float32(s.screenWidth) - panelWidth - 12
	panelY := float32(12)
	lineHeight := float32(16)
	panelHeight := float32(16 + (len(lines) * int(lineHeight)))
	if panelHeight > float32(s.screenHeight)-24 {
		panelHeight = float32(s.screenHeight) - 24
	}

	vector.DrawFilledRect(screen, panelX, panelY, panelWidth, panelHeight, color.RGBA{R: 8, G: 12, B: 18, A: 216}, false)
	vector.StrokeRect(screen, panelX, panelY, panelWidth, panelHeight, 1, color.RGBA{R: 71, G: 90, B: 110, A: 255}, false)

	maxLines := int((panelHeight - 12) / lineHeight)
	if maxLines > len(lines) {
		maxLines = len(lines)
	}
	for i := 0; i < maxLines; i++ {
		textColor := color.RGBA{R: 211, G: 221, B: 233, A: 255}
		if i >= 2 && (i-2) == selectedIndex {
			textColor = color.RGBA{R: 250, G: 253, B: 255, A: 255}
		}
		text.Draw(screen, lines[i], basicfont.Face7x13, int(panelX)+10, int(panelY)+20+(i*int(lineHeight)), textColor)
	}
}
