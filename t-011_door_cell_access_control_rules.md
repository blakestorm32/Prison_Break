# T-011 Door/Cell Access-Control Rules

Status: Implemented (code + tests)  
Related roadmap ticket: `T-011` in `roadmap_tickets.md`

## Goal

Enforce map access-control rules for:
- cell ownership and authority privileges on cell doors
- Warden HQ room restriction
- camera room authority + power dependency
- black market prisoner-only access

## Implemented Files

- `internal/gamecore/map/access_control.go`
- `internal/gamecore/map/access_control_test.go`
- `internal/gamecore/map/layout.go`
- `internal/gamecore/map/layout_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`

## Rules Enforced

1. Cell ownership + authority privilege:
- only cell owner or authority can operate that cell door
- non-owner prisoners cannot bypass this by targeting the raw door id

2. Warden HQ restriction:
- only `RoleWarden` may enter `warden_hq`

3. Camera room restriction:
- authority only
- additionally denied for everyone when power is off

4. Black market restriction:
- only prisoners may enter the configured `black_market_room_id`

## Server Integration

1. Deterministic cell assignment on match start:
- each player is assigned a unique cell deterministically by sorted player id
- corresponding `CellState.owner_player_id` is set
- players start in `cell_block_a` with `assigned_cell_id` populated

2. Interaction command enforcement:
- `CmdInteract` now applies room entry and door/cell operations through access checks
- unauthorized interactions are ignored (no state mutation)
- authorized operations mutate authoritative map/player state

3. Door state support for cells:
- map state now includes explicit cell door entries (12 additional doors)
- total seeded doors in default map state: 22 (10 room doors + 12 cell doors)

## Test Coverage

`internal/gamecore/map/access_control_test.go`:
- Warden HQ role gate
- camera room authority + power-off behavior
- black market prisoner-only behavior
- cell door owner/authority checks
- role-based authority/prisoner classification fallbacks

`internal/server/game/manager_test.go`:
- deterministic cell ownership assignment at match start
- owner/authority vs non-owner prisoner cell-door interaction behavior
- room interaction enforcement for Warden HQ, camera room, and black market

Additional updates:
- `internal/gamecore/map/layout_test.go` and map seeding tests updated for expanded door set

## Verification

Ran:
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing across map, game, networking, and existing packages.
