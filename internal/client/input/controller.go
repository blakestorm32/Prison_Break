package input

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"prison-break/internal/gamecore/abilities"
	"prison-break/internal/gamecore/cards"
	"prison-break/internal/gamecore/combat"
	"prison-break/internal/gamecore/escape"
	gameitems "prison-break/internal/gamecore/items"
	"prison-break/internal/shared/model"
)

type TouchPoint struct {
	ID int64
	X  float64
	Y  float64
}

type InputSnapshot struct {
	MoveUp    bool
	MoveDown  bool
	MoveLeft  bool
	MoveRight bool
	Sprint    bool

	InteractPressed bool
	ReloadPressed   bool
	FirePressed     bool

	HasAim    bool
	AimWorldX float32
	AimWorldY float32

	PanelInventoryPressed bool
	PanelCardsPressed     bool
	PanelAbilitiesPressed bool
	PanelMarketPressed    bool
	PanelEscapePressed    bool
	PanelPrevPressed      bool
	PanelNextPressed      bool
	PanelUsePressed       bool

	SpectatorPrevPressed bool
	SpectatorNextPressed bool

	Touches []TouchPoint
}

type Rect struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func (r Rect) Contains(x float64, y float64) bool {
	return x >= r.MinX && x <= r.MaxX && y >= r.MinY && y <= r.MaxY
}

type MobileLayout struct {
	Enabled bool

	JoystickCenterX float64
	JoystickCenterY float64
	JoystickRadius  float64

	FireButton     Rect
	InteractButton Rect
	ReloadButton   Rect
}

func DefaultMobileLayout(screenWidth int, screenHeight int) MobileLayout {
	if screenWidth <= 0 {
		screenWidth = 1280
	}
	if screenHeight <= 0 {
		screenHeight = 720
	}

	screenW := float64(screenWidth)
	screenH := float64(screenHeight)
	buttonSize := math.Max(64, screenH*0.11)
	margin := math.Max(18, screenW*0.018)

	joystickRadius := math.Max(56, screenH*0.12)
	joystickCenterX := margin + joystickRadius
	joystickCenterY := screenH - margin - joystickRadius

	rightBaseX := screenW - margin - buttonSize
	baseY := screenH - margin - buttonSize

	return MobileLayout{
		Enabled:         true,
		JoystickCenterX: joystickCenterX,
		JoystickCenterY: joystickCenterY,
		JoystickRadius:  joystickRadius,
		FireButton: Rect{
			MinX: rightBaseX,
			MinY: baseY - buttonSize,
			MaxX: rightBaseX + buttonSize,
			MaxY: baseY,
		},
		InteractButton: Rect{
			MinX: rightBaseX - buttonSize - margin,
			MinY: baseY - (buttonSize * 0.45),
			MaxX: rightBaseX - margin,
			MaxY: baseY + (buttonSize * 0.55),
		},
		ReloadButton: Rect{
			MinX: rightBaseX,
			MinY: baseY + (buttonSize * 0.25),
			MaxX: rightBaseX + buttonSize,
			MaxY: baseY + (buttonSize * 1.25),
		},
	}
}

type ControllerConfig struct {
	PlayerID model.PlayerID

	ScreenWidth  int
	ScreenHeight int

	FireWeapon model.ItemType

	MobileLayout MobileLayout
}

type Controller struct {
	playerID model.PlayerID

	nextClientSeq uint64
	fireWeapon    model.ItemType

	mobile MobileLayout

	prevFirePressed     bool
	prevInteractPressed bool
	prevReloadPressed   bool
	lastMoveTargetTick  uint64
	lastAimTargetTick   uint64
}

func NewController(config ControllerConfig) *Controller {
	trimmedID := strings.TrimSpace(string(config.PlayerID))

	fireWeapon := config.FireWeapon
	if fireWeapon == "" {
		fireWeapon = model.ItemPistol
	}
	if !combat.IsSupportedWeapon(fireWeapon) {
		fireWeapon = model.ItemPistol
	}

	mobile := config.MobileLayout
	if mobile.JoystickRadius <= 0 {
		mobile = DefaultMobileLayout(config.ScreenWidth, config.ScreenHeight)
	}

	return &Controller{
		playerID:   model.PlayerID(trimmedID),
		fireWeapon: fireWeapon,
		mobile:     mobile,
	}
}

func (c *Controller) MobileLayout() MobileLayout {
	if c == nil {
		return MobileLayout{}
	}
	return c.mobile
}

func (c *Controller) BuildUseAbilityCommand(
	payload model.AbilityUsePayload,
	targetTick uint64,
) (model.InputCommand, bool) {
	if c == nil || c.playerID == "" || !abilities.IsKnownAbility(payload.Ability) {
		return model.InputCommand{}, false
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return model.InputCommand{}, false
	}
	return c.newCommand(model.CmdUseAbility, rawPayload, targetTick), true
}

func (c *Controller) BuildUseCardCommand(
	payload model.CardUsePayload,
	targetTick uint64,
) (model.InputCommand, bool) {
	if c == nil || c.playerID == "" || !cards.IsKnownCard(payload.Card) {
		return model.InputCommand{}, false
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return model.InputCommand{}, false
	}
	return c.newCommand(model.CmdUseCard, rawPayload, targetTick), true
}

func (c *Controller) BuildUseItemCommand(
	payload model.ItemUsePayload,
	targetTick uint64,
) (model.InputCommand, bool) {
	if c == nil || c.playerID == "" || strings.TrimSpace(string(payload.Item)) == "" {
		return model.InputCommand{}, false
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return model.InputCommand{}, false
	}
	return c.newCommand(model.CmdUseItem, rawPayload, targetTick), true
}

func (c *Controller) BuildBlackMarketBuyCommand(
	payload model.BlackMarketPurchasePayload,
	targetTick uint64,
) (model.InputCommand, bool) {
	if c == nil || c.playerID == "" {
		return model.InputCommand{}, false
	}
	if _, exists := gameitems.BlackMarketOfferForItem(payload.Item); !exists {
		return model.InputCommand{}, false
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return model.InputCommand{}, false
	}
	return c.newCommand(model.CmdBlackMarketBuy, rawPayload, targetTick), true
}

func (c *Controller) BuildInteractCommand(
	payload model.InteractPayload,
	targetTick uint64,
) (model.InputCommand, bool) {
	if c == nil || c.playerID == "" {
		return model.InputCommand{}, false
	}
	if payload.EscapeRoute != "" && !escape.IsKnownRoute(payload.EscapeRoute) {
		return model.InputCommand{}, false
	}
	if payload.MarketRoomID != "" && strings.TrimSpace(string(payload.MarketRoomID)) == "" {
		return model.InputCommand{}, false
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return model.InputCommand{}, false
	}
	return c.newCommand(model.CmdInteract, rawPayload, targetTick), true
}

func (c *Controller) BuildCommands(
	snapshot InputSnapshot,
	targetTick uint64,
	localPlayer *model.PlayerState,
) []model.InputCommand {
	if c == nil || c.playerID == "" {
		return nil
	}

	moveX, moveY, sprint := c.resolveMove(snapshot)

	aimWorldX := snapshot.AimWorldX
	aimWorldY := snapshot.AimWorldY
	hasAim := snapshot.HasAim
	if !hasAim && localPlayer != nil {
		aimWorldX = localPlayer.Position.X
		aimWorldY = localPlayer.Position.Y
		hasAim = true
	}

	firePressed := snapshot.FirePressed || c.touchInsideButton(snapshot.Touches, c.mobile.FireButton)
	interactPressed := snapshot.InteractPressed || c.touchInsideButton(snapshot.Touches, c.mobile.InteractButton)
	reloadPressed := snapshot.ReloadPressed || c.touchInsideButton(snapshot.Touches, c.mobile.ReloadButton)

	commands := make([]model.InputCommand, 0, 5)

	if (moveX != 0 || moveY != 0) && targetTick != 0 && targetTick != c.lastMoveTargetTick {
		payload, err := json.Marshal(model.MovementInputPayload{
			MoveX:  moveX,
			MoveY:  moveY,
			Sprint: sprint,
		})
		if err == nil {
			commands = append(commands, c.newCommand(model.CmdMoveIntent, payload, targetTick))
			c.lastMoveTargetTick = targetTick
		}
	}

	if hasAim && targetTick != 0 && targetTick != c.lastAimTargetTick {
		aimX, aimY := resolveAimVector(aimWorldX, aimWorldY, localPlayer)
		payload, err := json.Marshal(model.AimInputPayload{
			AimX: aimX,
			AimY: aimY,
		})
		if err == nil {
			commands = append(commands, c.newCommand(model.CmdAimIntent, payload, targetTick))
			c.lastAimTargetTick = targetTick
		}
	}

	if edgePressed(c.prevFirePressed, firePressed) {
		payload, err := json.Marshal(model.FireWeaponPayload{
			Weapon:  c.fireWeapon,
			TargetX: aimWorldX,
			TargetY: aimWorldY,
		})
		if err == nil {
			commands = append(commands, c.newCommand(model.CmdFireWeapon, payload, targetTick))
		}
	}

	if edgePressed(c.prevInteractPressed, interactPressed) {
		payload, err := json.Marshal(model.InteractPayload{})
		if err == nil {
			commands = append(commands, c.newCommand(model.CmdInteract, payload, targetTick))
		}
	}

	if edgePressed(c.prevReloadPressed, reloadPressed) {
		commands = append(commands, c.newCommand(model.CmdReload, nil, targetTick))
	}

	c.prevFirePressed = firePressed
	c.prevInteractPressed = interactPressed
	c.prevReloadPressed = reloadPressed

	return commands
}

func (c *Controller) newCommand(
	commandType model.InputCommandType,
	payload json.RawMessage,
	targetTick uint64,
) model.InputCommand {
	c.nextClientSeq++
	return model.InputCommand{
		PlayerID:   c.playerID,
		ClientSeq:  c.nextClientSeq,
		TargetTick: targetTick,
		Type:       commandType,
		Payload:    payload,
	}
}

func (c *Controller) resolveMove(snapshot InputSnapshot) (float32, float32, bool) {
	var moveX float32
	var moveY float32

	if snapshot.MoveRight {
		moveX += 1
	}
	if snapshot.MoveLeft {
		moveX -= 1
	}
	if snapshot.MoveDown {
		moveY += 1
	}
	if snapshot.MoveUp {
		moveY -= 1
	}

	sprint := snapshot.Sprint

	if moveX == 0 && moveY == 0 && c.mobile.Enabled {
		jx, jy, active := c.joystickVector(snapshot.Touches)
		if active {
			moveX = jx
			moveY = jy
			if math.Hypot(float64(jx), float64(jy)) > 0.82 {
				sprint = true
			}
		}
	}

	moveX, moveY = normalizeVector(moveX, moveY)
	if moveX == 0 && moveY == 0 {
		sprint = false
	}

	return moveX, moveY, sprint
}

func (c *Controller) joystickVector(touches []TouchPoint) (float32, float32, bool) {
	if !c.mobile.Enabled || c.mobile.JoystickRadius <= 0 || len(touches) == 0 {
		return 0, 0, false
	}

	ordered := append([]TouchPoint(nil), touches...)
	sort.Slice(ordered, func(i int, j int) bool {
		return ordered[i].ID < ordered[j].ID
	})

	for _, touch := range ordered {
		dx := touch.X - c.mobile.JoystickCenterX
		dy := touch.Y - c.mobile.JoystickCenterY
		distance := math.Hypot(dx, dy)
		if distance > c.mobile.JoystickRadius*1.35 {
			continue
		}
		if distance == 0 {
			return 0, 0, true
		}

		magnitude := math.Min(1, distance/c.mobile.JoystickRadius)
		unitX := dx / distance
		unitY := dy / distance
		return float32(unitX * magnitude), float32(unitY * magnitude), true
	}

	return 0, 0, false
}

func (c *Controller) touchInsideButton(touches []TouchPoint, button Rect) bool {
	if !c.mobile.Enabled || len(touches) == 0 {
		return false
	}
	for _, touch := range touches {
		if button.Contains(touch.X, touch.Y) {
			return true
		}
	}
	return false
}

func resolveAimVector(aimWorldX float32, aimWorldY float32, localPlayer *model.PlayerState) (float32, float32) {
	if localPlayer == nil {
		return aimWorldX, aimWorldY
	}
	return aimWorldX - localPlayer.Position.X, aimWorldY - localPlayer.Position.Y
}

func edgePressed(previous bool, current bool) bool {
	return !previous && current
}

func normalizeVector(x float32, y float32) (float32, float32) {
	magnitude := math.Hypot(float64(x), float64(y))
	if magnitude <= 1 {
		return x, y
	}
	return float32(float64(x) / magnitude), float32(float64(y) / magnitude)
}
