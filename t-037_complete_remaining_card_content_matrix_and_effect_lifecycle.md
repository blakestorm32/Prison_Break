# T-037 Complete Remaining Card Content Matrix and Effect Lifecycle

Status: Implemented (code + tests)  
Related roadmap ticket: `T-037` in `roadmap_tickets.md`

## Goal

Complete card matrix behavior and edge-case lifecycle handling for:
- `morphine`
- `bullet`
- `money`
- `speed`
- `armor_plate`
- `lock_snap`
- `item_steal`
- `item_grab`
- `scrap_bundle`
- `door_stop`
- `get_out_of_jail_free`

## Implemented Files

- `internal/shared/model/input.go`
- `internal/gamecore/cards/framework.go`
- `internal/gamecore/cards/framework_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_abilities_cards_test.go`
- `internal/client/render/shell_panels.go`
- `roadmap_tickets.md`

## Core Changes

1. Card payload schema and validation hardening:
- Added optional `target_item` to `CardUsePayload` for targeted `item_steal` usage.
- Added card-specific payload checks in input validation:
  - `lock_snap` and `door_stop` require `target_door_id`
  - `item_steal` and `item_grab` require `target_player_id`
  - unknown `target_item` values are rejected

2. Card effect behavior completion and balancing:
- `armor_plate` now avoids stacking temporary hearts; grants/refreshes only when beneficial.
- Redundant armor use no longer wastes the card if it does not improve state.
- `item_steal` now supports explicit target item stealing (`target_item`) while keeping deterministic fallback behavior.
- `item_grab` now takes a target player and transfers a deterministic random item from that target inventory (instead of generating free items).
- Added deterministic inventory-based random selection helper in cards framework.

3. Lock Snap lifecycle implementation:
- `lock_snap` now requires a locked door target.
- On use, it temporarily breaks the lock and opens the door.
- Original door lock/open state is restored after round end (phase end lifecycle), with deterministic tick-based restore handling.

4. UX integration:
- Client action panel now auto-selects a local-room target player for `item_grab`, matching new server validation and reducing failed actions.

## Test Coverage Added

`internal/gamecore/cards/framework_test.go`:
- `TestDeterministicGrabFromInventory`

`internal/server/game/input_queue_test.go`:
- `TestSubmitInputRejectsCardPayloadWithoutRequiredTargets`

`internal/server/game/manager_abilities_cards_test.go`:
- `TestCardBulletAddsAmmoAndPreservesCardAtCap`
- `TestCardArmorPlateNoStackAndPhaseExpiryLifecycle`
- `TestCardLockSnapRepairsDoorStateAfterRound`
- `TestCardItemStealSupportsTargetItemAndRoomGate`
- `TestCardItemGrabStealsDeterministicItemFromTarget`
- `TestCardScrapBundleGrantsMaterialsAndPreservesOnCapacityFailure`
- `TestCardGetOutOfJailFreeClearsAuthorityPenaltyLock`

## Verification

Executed:
- `go test ./internal/gamecore/cards`
- `go test ./internal/client/render`
- `go test ./internal/server/game`
- `go test ./...`

Result:
- all tests passing.
