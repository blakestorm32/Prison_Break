# T-016 Inventory / Items / Contraband Tagging

Status: Implemented (code + tests)  
Related roadmap ticket: `T-016` in `roadmap_tickets.md`

## Goal

Implement deterministic item systems so these are functional:
- item ownership in per-player inventory
- item pickups from world drops
- item transfers between players
- contraband tagging/checks
- inventory capacity constraints

## Implemented Files

- `internal/gamecore/items/inventory.go`
- `internal/gamecore/items/inventory_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_items_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Inventory and capacity:
- deterministic, normalized inventory stacks (merged + sorted by item id)
- base stack capacity is `6`
- each `satchel` adds `+3` stack capacity
- capacity checks enforce atomic add/remove/transfer/craft operations

2. Item operations:
- `AddItem`, `RemoveItem`, `TransferItem`, `Craft`, `HasItem`
- transfer requires same-room source/target and both alive
- craft uses deterministic recipes and is atomic

3. Contraband:
- contraband items are tagged via `items.IsContraband(...)`
- player-level checks available via `ContrabandStacks(...)` and `HasContraband(...)`

4. Dropped items and pickups:
- drops create `EntityKindDroppedItem` entities with deterministic tags:
  - `item:<item_type>`
  - `qty:<n>`
  - `contraband` (when applicable)
- pickup is handled through `CmdInteract` with `target_entity_id`
- pickup requires same room and sufficient capacity
- successful pickup removes the dropped entity and updates player inventory

5. Tick + snapshot integration:
- `CmdDropItem`, `CmdUseItem`, and `CmdCraftItem` now mutate game state
- delta snapshots now include `changed_entities` and `removed_entity_ids` when relevant

6. Payload validation hardening:
- `use_item`, `drop_item`, and `fire_weapon` require known item ids
- `craft_item` requires a known craft recipe output

## Test Coverage

`internal/gamecore/items/inventory_test.go`:
- stack normalization/merging/sorting
- capacity constraints with satchel expansion
- atomic transfer failure on destination-capacity edge case
- craft atomicity on output overflow
- contraband detection
- dropped-item tag parse/validation edge cases

`internal/server/game/manager_items_test.go`:
- drop command creates dropped entity and entity delta
- pickup path checks room + capacity constraints and removed-entity delta
- transfer command validation (same room + destination capacity)
- craft command integration and contraband output detection

`internal/server/game/input_queue_test.go`:
- unknown item ids are rejected for item-related command payloads

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/items ./internal/server/game`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
