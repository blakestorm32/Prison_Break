package model

import "encoding/json"

type InputCommand struct {
	PlayerID PlayerID `json:"player_id"`

	ClientSeq  uint64 `json:"client_seq"`
	IngressSeq uint64 `json:"ingress_seq,omitempty"`
	TargetTick uint64 `json:"target_tick,omitempty"`

	Type    InputCommandType `json:"type"`
	Payload json.RawMessage  `json:"payload,omitempty"`
}

type MovementInputPayload struct {
	MoveX  float32 `json:"move_x"`
	MoveY  float32 `json:"move_y"`
	Sprint bool    `json:"sprint,omitempty"`
}

type AimInputPayload struct {
	AimX float32 `json:"aim_x"`
	AimY float32 `json:"aim_y"`
}

type InteractPayload struct {
	TargetEntityID EntityID        `json:"target_entity_id,omitempty"`
	TargetDoorID   DoorID          `json:"target_door_id,omitempty"`
	TargetCellID   CellID          `json:"target_cell_id,omitempty"`
	TargetRoomID   RoomID          `json:"target_room_id,omitempty"`
	EscapeRoute    EscapeRouteType `json:"escape_route,omitempty"`
	MarketRoomID   RoomID          `json:"market_room_id,omitempty"`
	NightCardChoice CardType       `json:"night_card_choice,omitempty"`
	StashAction     string         `json:"stash_action,omitempty"`
	StashItem       ItemType       `json:"stash_item,omitempty"`
	StashAmount     uint8          `json:"stash_amount,omitempty"`
}

type AbilityUsePayload struct {
	Ability        AbilityType `json:"ability"`
	TargetPlayerID PlayerID    `json:"target_player_id,omitempty"`
	TargetEntityID EntityID    `json:"target_entity_id,omitempty"`
	TargetDoorID   DoorID      `json:"target_door_id,omitempty"`
	TargetCellID   CellID      `json:"target_cell_id,omitempty"`
	TargetRoomID   RoomID      `json:"target_room_id,omitempty"`
}

type CardUsePayload struct {
	Card           CardType `json:"card"`
	TargetPlayerID PlayerID `json:"target_player_id,omitempty"`
	TargetEntityID EntityID `json:"target_entity_id,omitempty"`
	TargetDoorID   DoorID   `json:"target_door_id,omitempty"`
	TargetCellID   CellID   `json:"target_cell_id,omitempty"`
	TargetItem     ItemType `json:"target_item,omitempty"`
}

type ItemUsePayload struct {
	Item           ItemType `json:"item"`
	TargetPlayerID PlayerID `json:"target_player_id,omitempty"`
	TargetDoorID   DoorID   `json:"target_door_id,omitempty"`
	TargetRoomID   RoomID   `json:"target_room_id,omitempty"`
	Amount         uint8    `json:"amount,omitempty"`
}

type BlackMarketPurchasePayload struct {
	Item ItemType `json:"item"`
}

type FireWeaponPayload struct {
	Weapon         ItemType `json:"weapon"`
	TargetX        float32  `json:"target_x"`
	TargetY        float32  `json:"target_y"`
	UseGoldenRound bool     `json:"use_golden_round,omitempty"`
}

type CraftItemPayload struct {
	Item ItemType `json:"item"`
}

type DropItemPayload struct {
	Item   ItemType `json:"item"`
	Amount uint8    `json:"amount,omitempty"`
}
