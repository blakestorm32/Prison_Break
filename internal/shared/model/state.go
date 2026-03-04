package model

// Slices in this schema should be serialized in deterministic ID order.
type Vector2 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

type PhaseState struct {
	Current     PhaseType `json:"current"`
	StartedTick uint64    `json:"started_tick"`
	EndsTick    uint64    `json:"ends_tick"`
}

type AlarmState struct {
	Active      bool     `json:"active"`
	EndsTick    uint64   `json:"ends_tick,omitempty"`
	TriggeredBy PlayerID `json:"triggered_by,omitempty"`
}

type ItemStack struct {
	Item     ItemType `json:"item"`
	Quantity uint8    `json:"quantity"`
}

type EffectState struct {
	Effect    EffectType `json:"effect"`
	EndsTick  uint64     `json:"ends_tick,omitempty"`
	Stacks    uint8      `json:"stacks,omitempty"`
	SourceID  EntityID   `json:"source_id,omitempty"`
	SourcePID PlayerID   `json:"source_player_id,omitempty"`
}

type EscapeAttemptFeedback struct {
	Route  EscapeRouteType     `json:"route"`
	Status EscapeAttemptStatus `json:"status"`
	Reason string              `json:"reason,omitempty"`
	TickID uint64              `json:"tick_id"`
}

type ActionFeedback struct {
	Kind    ActionFeedbackKind  `json:"kind"`
	Level   ActionFeedbackLevel `json:"level"`
	Message string              `json:"message"`
	TickID  uint64              `json:"tick_id"`
}

type PlayerState struct {
	ID        PlayerID `json:"id"`
	Name      string   `json:"name"`
	Connected bool     `json:"connected"`
	Alive     bool     `json:"alive"`

	Faction   FactionType   `json:"faction"`
	Alignment AlignmentType `json:"alignment"`
	Role      RoleType      `json:"role"`

	HeartsHalf     uint8 `json:"hearts_half"`
	TempHeartsHalf uint8 `json:"temp_hearts_half,omitempty"`
	Bullets        uint8 `json:"bullets"`

	Position Vector2 `json:"position"`
	Velocity Vector2 `json:"velocity"`
	Facing   Vector2 `json:"facing"`

	CurrentRoomID   RoomID      `json:"current_room_id,omitempty"`
	AssignedCell    CellID      `json:"assigned_cell_id,omitempty"`
	LockedInCell    CellID      `json:"locked_in_cell_id,omitempty"`
	AssignedAbility AbilityType `json:"assigned_ability,omitempty"`

	StunnedUntilTick  uint64 `json:"stunned_until_tick,omitempty"`
	SolitaryUntilTick uint64 `json:"solitary_until_tick,omitempty"`

	Inventory []ItemStack   `json:"inventory,omitempty"`
	Cards     []CardType    `json:"cards,omitempty"`
	NightCardChoices []CardType `json:"night_card_choices,omitempty"`
	Effects   []EffectState `json:"effects,omitempty"`

	LastEscapeAttempt  EscapeAttemptFeedback `json:"last_escape_attempt,omitempty"`
	LastActionFeedback ActionFeedback        `json:"last_action_feedback,omitempty"`
}

type DoorState struct {
	ID DoorID `json:"id"`

	RoomA RoomID `json:"room_a"`
	RoomB RoomID `json:"room_b"`

	Open             bool     `json:"open"`
	Locked           bool     `json:"locked"`
	CanClose         bool     `json:"can_close"`
	BlockedUntilTick uint64   `json:"blocked_until_tick,omitempty"`
	LockedByPlayerID PlayerID `json:"locked_by_player_id,omitempty"`
}

type CellState struct {
	ID                CellID     `json:"id"`
	OwnerPlayerID     PlayerID   `json:"owner_player_id,omitempty"`
	DoorID            DoorID     `json:"door_id,omitempty"`
	OccupantPlayerIDs []PlayerID `json:"occupant_player_ids,omitempty"`
	Stash             []ItemStack `json:"stash,omitempty"`
}

type ZoneState struct {
	ID         ZoneID `json:"id"`
	RoomID     RoomID `json:"room_id,omitempty"`
	Restricted bool   `json:"restricted"`
	Name       string `json:"name,omitempty"`
}

type MapState struct {
	PowerOn bool       `json:"power_on"`
	Alarm   AlarmState `json:"alarm"`

	BlackMarketRoomID RoomID `json:"black_market_room_id,omitempty"`

	Doors           []DoorState `json:"doors,omitempty"`
	Cells           []CellState `json:"cells,omitempty"`
	RestrictedZones []ZoneState `json:"restricted_zones,omitempty"`
}

type EntityState struct {
	ID   EntityID   `json:"id"`
	Kind EntityKind `json:"kind"`

	OwnerPlayerID PlayerID `json:"owner_player_id,omitempty"`
	Position      Vector2  `json:"position"`
	Velocity      Vector2  `json:"velocity"`

	HeartsHalf int16    `json:"hearts_half,omitempty"`
	Active     bool     `json:"active"`
	RoomID     RoomID   `json:"room_id,omitempty"`
	Tags       []string `json:"tags,omitempty"`
}

type GameOverState struct {
	Reason          WinReason  `json:"reason"`
	EndedTick       uint64     `json:"ended_tick"`
	WinnerPlayerIDs []PlayerID `json:"winner_player_ids,omitempty"`
	Notes           string     `json:"notes,omitempty"`
}

type GameState struct {
	MatchID MatchID     `json:"match_id"`
	TickID  uint64      `json:"tick_id"`
	Status  MatchStatus `json:"status"`

	CycleCount uint8      `json:"cycle_count"`
	Phase      PhaseState `json:"phase"`

	Map      MapState      `json:"map"`
	Players  []PlayerState `json:"players"`
	Entities []EntityState `json:"entities,omitempty"`

	GameOver *GameOverState `json:"game_over,omitempty"`
}

type PlayerAck struct {
	PlayerID               PlayerID `json:"player_id"`
	LastProcessedClientSeq uint64   `json:"last_processed_client_seq"`
}

type Snapshot struct {
	Kind SnapshotKind `json:"kind"`

	TickID     uint64 `json:"tick_id"`
	BaseTickID uint64 `json:"base_tick_id,omitempty"`

	State *GameState `json:"state,omitempty"`
	Delta *GameDelta `json:"delta,omitempty"`

	PlayerAcks []PlayerAck `json:"player_acks,omitempty"`
}

type GameDelta struct {
	ChangedPlayers   []PlayerState `json:"changed_players,omitempty"`
	RemovedPlayerIDs []PlayerID    `json:"removed_player_ids,omitempty"`

	ChangedEntities  []EntityState `json:"changed_entities,omitempty"`
	RemovedEntityIDs []EntityID    `json:"removed_entity_ids,omitempty"`

	ChangedDoors []DoorState `json:"changed_doors,omitempty"`
	ChangedCells []CellState `json:"changed_cells,omitempty"`
	ChangedZones []ZoneState `json:"changed_zones,omitempty"`

	Phase             *PhaseState    `json:"phase,omitempty"`
	Status            *MatchStatus   `json:"status,omitempty"`
	CycleCount        *uint8         `json:"cycle_count,omitempty"`
	PowerOn           *bool          `json:"power_on,omitempty"`
	Alarm             *AlarmState    `json:"alarm,omitempty"`
	BlackMarketRoomID *RoomID        `json:"black_market_room_id,omitempty"`
	GameOver          *GameOverState `json:"game_over,omitempty"`
}
