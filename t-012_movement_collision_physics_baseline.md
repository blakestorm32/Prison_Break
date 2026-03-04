# T-012 Movement/Collision/Physics Baseline

Status: Implemented (code + tests)  
Related roadmap ticket: `T-012` in `roadmap_tickets.md`

## Goal

Implement baseline movement/physics so:
- players move on tile map only through walkable space
- collisions block movement (walls, closed doors, occupied tiles)
- stun prevents motion intents
- knockback motion and stun application are available for combat integration

## Implemented Files

- `internal/engine/physics/motion.go`
- `internal/engine/physics/motion_test.go`
- `internal/gamecore/map/layout.go` (door lookup by tile point)
- `internal/server/game/manager.go` (movement + knockback integration)
- `internal/server/game/manager_test.go`
- `internal/server/game/errors.go` (player lookup error for knockback API)

## Physics Baseline Behavior

1. Movement resolution:
- move intent vector is clamped to max per-tick displacement
- sprint input uses a higher cap than normal move
- resolution is axis-separated (`X` then `Y`) for deterministic blocking behavior

2. Collision blocking:
- out-of-bounds and wall tiles are blocked
- closed room doors are blocked
- destination tile occupied by another player is blocked

3. Stun-compatible motion:
- move intents are ignored while `current_tick <= stunned_until_tick`
- blocked-by-stun move outputs zero velocity

4. Knockback:
- dedicated knockback resolver applies collision-aware displacement
- stun duration extends using max(existing, new)
- manager exposes `ApplyKnockback(...)` for upcoming combat systems

## Server Integration

1. Spawn placement:
- on match start, players spawn on deterministic walkable coordinates in `cell_block_a`
- this avoids invalid origin positions and makes movement/collision deterministic

2. Tick simulation:
- `CmdMoveIntent` now goes through physics resolver instead of raw position addition
- room id is updated from resulting tile location
- deltas include changed player states when movement/velocity/room changes

3. Map support:
- layout now supports `DoorLinkAt(point)` lookup for tile-level door collision checks

## Test Coverage

`internal/engine/physics/motion_test.go`:
- move on walkable tiles
- wall and out-of-bounds blocking
- closed-door blocking and open-door pass
- occupied-tile (player collision) blocking
- stun prevents movement
- knockback displacement + stun extension
- extreme input magnitude clamping

`internal/server/game/manager_test.go`:
- movement blocked by closed door then succeeds when opened
- movement blocked by other player and by stun
- knockback integration (displacement, wall collision behavior, stun persistence)
- deterministic cell assignment/spawn continues to be validated with existing map/access tests

## Verification

Ran repeatedly during implementation:
- `C:\Program Files\Go\bin\go.exe test ./internal/engine/physics ./internal/gamecore/map`
- `C:\Program Files\Go\bin\go.exe test ./internal/server/game ./internal/engine/physics ./internal/server/networking`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
