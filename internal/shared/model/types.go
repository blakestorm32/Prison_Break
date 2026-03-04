package model

type (
	MatchID  string
	PlayerID string
	EntityID uint32
	DoorID   uint16
	CellID   uint16
	ZoneID   uint16
	RoomID   string
)

type MatchStatus string

const (
	MatchStatusLobby    MatchStatus = "lobby"
	MatchStatusRunning  MatchStatus = "running"
	MatchStatusGameOver MatchStatus = "game_over"
)

type PhaseType string

const (
	PhaseDay   PhaseType = "day"
	PhaseNight PhaseType = "night"
)

type FactionType string

const (
	FactionAuthority FactionType = "authority"
	FactionPrisoner  FactionType = "prisoner"
	FactionNeutral   FactionType = "neutral"
)

type AlignmentType string

const (
	AlignmentGood    AlignmentType = "good"
	AlignmentEvil    AlignmentType = "evil"
	AlignmentNeutral AlignmentType = "neutral"
)

type RoleType string

const (
	RoleWarden          RoleType = "warden"
	RoleDeputy          RoleType = "deputy"
	RoleGangLeader      RoleType = "gang_leader"
	RoleGangMember      RoleType = "gang_member"
	RoleSnitch          RoleType = "snitch"
	RoleNeutralPrisoner RoleType = "neutral_prisoner"
)

type EntityKind string

const (
	EntityKindPlayer      EntityKind = "player"
	EntityKindNPCGuard    EntityKind = "npc_guard"
	EntityKindNPCPrisoner EntityKind = "npc_prisoner"
	EntityKindProjectile  EntityKind = "projectile"
	EntityKindDroppedItem EntityKind = "dropped_item"
	EntityKindDoorProxy   EntityKind = "door_proxy"
)

type ItemType string

const (
	ItemWood         ItemType = "wood"
	ItemMetalSlab    ItemType = "metal_slab"
	ItemShiv         ItemType = "shiv"
	ItemBullet       ItemType = "bullet"
	ItemPistol       ItemType = "pistol"
	ItemHuntingRifle ItemType = "hunting_rifle"
	ItemLockPick     ItemType = "lock_pick"
	ItemWireCutters  ItemType = "wire_cutters"
	ItemSilencer     ItemType = "silencer"
	ItemSatchel      ItemType = "satchel"
	ItemDoorStop     ItemType = "door_stop"
	ItemGoldenBullet ItemType = "golden_bullet"
	ItemLadder       ItemType = "ladder"
	ItemShovel       ItemType = "shovel"
	ItemBadge        ItemType = "badge"
	ItemKeys         ItemType = "keys"
)

type CardType string

const (
	CardMorphine         CardType = "morphine"
	CardBullet           CardType = "bullet"
	CardMoney            CardType = "money"
	CardSpeed            CardType = "speed"
	CardArmorPlate       CardType = "armor_plate"
	CardLockSnap         CardType = "lock_snap"
	CardItemSteal        CardType = "item_steal"
	CardItemGrab         CardType = "item_grab"
	CardScrapBundle      CardType = "scrap_bundle"
	CardDoorStop         CardType = "door_stop"
	CardGetOutOfJailFree CardType = "get_out_of_jail_free"
)

type AbilityType string

const (
	AbilityAlarm      AbilityType = "alarm"
	AbilitySearch     AbilityType = "search"
	AbilityCameraMan  AbilityType = "camera_man"
	AbilityDetainer   AbilityType = "detainer"
	AbilityTracker    AbilityType = "tracker"
	AbilityPickPocket AbilityType = "pick_pocket"
	AbilityHacker     AbilityType = "hacker"
	AbilityDisguise   AbilityType = "disguise"
	AbilityLocksmith  AbilityType = "locksmith"
	AbilityChameleon  AbilityType = "chameleon"
)

type InputCommandType string

const (
	CmdMoveIntent     InputCommandType = "move_intent"
	CmdAimIntent      InputCommandType = "aim_intent"
	CmdInteract       InputCommandType = "interact"
	CmdUseAbility     InputCommandType = "use_ability"
	CmdUseCard        InputCommandType = "use_card"
	CmdUseItem        InputCommandType = "use_item"
	CmdBlackMarketBuy InputCommandType = "black_market_buy"
	CmdFireWeapon     InputCommandType = "fire_weapon"
	CmdReload         InputCommandType = "reload"
	CmdDropItem       InputCommandType = "drop_item"
	CmdCraftItem      InputCommandType = "craft_item"
)

type SnapshotKind string

const (
	SnapshotKindFull  SnapshotKind = "full"
	SnapshotKindDelta SnapshotKind = "delta"
)

type EffectType string

const (
	EffectStunned    EffectType = "stunned"
	EffectSolitary   EffectType = "solitary"
	EffectSpeedBoost EffectType = "speed_boost"
	EffectArmorPlate EffectType = "armor_plate"
	EffectTracked    EffectType = "tracked"
	EffectDisguised  EffectType = "disguised"
	EffectChameleon  EffectType = "chameleon"
)

type EscapeRouteType string

const (
	EscapeRouteCourtyardDig   EscapeRouteType = "courtyard_dig"
	EscapeRouteBadgeEscape    EscapeRouteType = "badge_escape"
	EscapeRoutePowerOutEscape EscapeRouteType = "power_out_escape"
	EscapeRouteLadderEscape   EscapeRouteType = "ladder_escape"
	EscapeRouteRoofHelicopter EscapeRouteType = "roof_helicopter_escape"
)

type EscapeAttemptStatus string

const (
	EscapeAttemptStatusSuccess EscapeAttemptStatus = "success"
	EscapeAttemptStatusFailed  EscapeAttemptStatus = "failed"
)

type ActionFeedbackKind string

const (
	ActionFeedbackKindCombat   ActionFeedbackKind = "combat"
	ActionFeedbackKindStun     ActionFeedbackKind = "stun"
	ActionFeedbackKindAlarm    ActionFeedbackKind = "alarm"
	ActionFeedbackKindDoor     ActionFeedbackKind = "door"
	ActionFeedbackKindPurchase ActionFeedbackKind = "purchase"
	ActionFeedbackKindEscape   ActionFeedbackKind = "escape"
	ActionFeedbackKindSystem   ActionFeedbackKind = "system"
)

type ActionFeedbackLevel string

const (
	ActionFeedbackLevelInfo    ActionFeedbackLevel = "info"
	ActionFeedbackLevelSuccess ActionFeedbackLevel = "success"
	ActionFeedbackLevelWarning ActionFeedbackLevel = "warning"
	ActionFeedbackLevelError   ActionFeedbackLevel = "error"
)

type WinReason string

const (
	WinReasonMaxCyclesReached        WinReason = "max_cycles_reached"
	WinReasonWardenDied              WinReason = "warden_died"
	WinReasonGangLeaderEscaped       WinReason = "gang_leader_escaped"
	WinReasonAllGangMembersDead      WinReason = "all_gang_members_dead"
	WinReasonNoEscapesAtTimeLimit    WinReason = "no_escapes_at_time_limit"
	WinReasonHitmanTargetEliminated  WinReason = "hitman_target_eliminated"
	WinReasonEscapeArtistFirstEscape WinReason = "escape_artist_first_escape"
)
