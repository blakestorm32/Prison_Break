# T-010 Map Model (Tiles, Rooms, Corridors, Restricted Zones)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-010` in `roadmap_tickets.md`

## Goal

Implement a deterministic prison map model that provides:
- tile-level topology (walkable vs blocked)
- room/corridor definitions
- restricted zone tagging
- room access checks and path connectivity utilities

## Implemented Files

- `internal/gamecore/map/layout.go`
- `internal/gamecore/map/layout_test.go`
- `internal/server/game/manager.go` (default map seeding)
- `internal/server/game/manager_test.go` (integration validation)

## Map Model Features

1. Default deterministic prison layout:
- one canonical map with named rooms:
  - `warden_hq`, `camera_room`, `power_room`, `ammunition_room`, `mail_room`
  - `cell_block_a`, `cafeteria`, `courtyard`, `black_market`, `roof_lookout`
  - `corridor_main`
- explicit door links between each room and corridor.

2. Tile model:
- `TileWall`, `TileFloor`, `TileDoor`
- bounds checks and walkability checks.

3. Room/corridor and restriction queries:
- room existence and metadata lookups
- corridor room detection
- restricted room checks.

4. Connectivity and access checks:
- deterministic BFS room pathing (`FindRoomPath`, `AreRoomsConnected`)
- deterministic BFS tile pathfinding (`FindPath`)
- room access evaluation (`CheckRoomAccess`) with reachability + restricted-target flag.

5. Shared model conversion:
- map layout converts to `model.MapState` via `ToMapState()`
- deterministic ordering for doors, cells, and restricted zones.

## Server Integration

- `internal/server/game/manager.go` now seeds initial `GameState.Map` from `gamemap.DefaultPrisonLayout().ToMapState()`.
- This ensures snapshots already contain a consistent topology for upcoming access-control and movement tickets.

## Test Coverage

`internal/gamecore/map/layout_test.go` includes:
- expected room/restricted-zone topology validation
- room-access checks with restricted target and missing-room edge case
- deterministic room path checks
- tile path validity checks (bounds, walkability, adjacency continuity)
- pathfinding edge cases (out-of-bounds, wall target, same start/end)
- deterministic ordering checks for map state collections

`internal/server/game/manager_test.go` integration:
- create-match snapshot seeds default map topology (doors/cells/restricted zones/black market id/power+alarm defaults)

## Verification

Ran:
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing across map, game, networking, and existing packages.
