# T-021 Ebiten Client Shell (Render + Camera + HUD Skeleton)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-021` in `roadmap_tickets.md`

## Goal

Build a runnable Ebiten client shell that renders authoritative snapshot state:
- map topology
- door/entity/player visuals
- phase and cycle
- local health/ammo
- active timed cooldown-style effects in HUD

## Implemented Files

- `cmd/client/main.go`
- `internal/client/netclient/snapshot_store.go`
- `internal/client/netclient/snapshot_store_test.go`
- `internal/client/render/camera.go`
- `internal/client/render/camera_test.go`
- `internal/client/render/hud.go`
- `internal/client/render/hud_test.go`
- `internal/client/render/shell.go`
- `go.mod`
- `go.sum`
- `roadmap_tickets.md`

## Core Behavior

1. Authoritative snapshot state store:
- Added `SnapshotStore` in `internal/client/netclient` to ingest authoritative snapshots.
- Supports:
  - full snapshot replacement
  - delta snapshot application over existing state
  - deterministic upsert/remove for players/entities/doors/cells/zones
  - global phase/status/power/alarm/market/gameover updates
- Rejects invalid deltas (no baseline state, stale tick IDs).

2. Ebiten render shell:
- Added `render.Shell` implementing `ebiten.Game`.
- Renders:
  - room geometry from `gamecore/map` layout
  - live door state (open/closed)
  - player markers
  - entity markers
- Camera:
  - follows local player when present
  - clamps to map bounds
  - transforms tile/world positions into screen coordinates.

3. HUD skeleton:
- Added `BuildHUDLines` for structured HUD output.
- Displays:
  - match/tick/status
  - phase/cycle/end tick
  - power/alarm/black-market room
  - local player identity/role
  - local hearts, temp hearts, ammo, room
  - timed cooldown list based on stun/solitary/effect end ticks

4. Client entrypoint:
- `cmd/client/main.go` now boots Ebiten shell.
- Seeds a deterministic demo authoritative full snapshot until live net client wiring (`T-022`+) is implemented.

5. Dependency pinning:
- Added Ebiten and compatible supporting modules while preserving module `go` target at `1.22.0`.

## Test Coverage

`internal/client/netclient/snapshot_store_test.go`:
- full snapshot application
- deep-copy safety for state access
- delta merge behavior across players/entities/map/global fields
- stale/invalid delta rejection

`internal/client/render/camera_test.go`:
- focus-to-screen-center transform behavior
- map-bound camera clamping

`internal/client/render/hud_test.go`:
- phase/health/ammo/cooldown string coverage
- missing-local-player fallback rendering
- half-heart formatting edge cases

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/client/...`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
