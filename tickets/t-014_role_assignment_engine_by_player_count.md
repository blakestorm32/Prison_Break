# T-014 Role Assignment Engine by Player Count

Status: Implemented (code + tests)  
Related roadmap ticket: `T-014` in `roadmap_tickets.md`

## Goal

Implement deterministic role assignment for player counts `6-12` with hidden-role visibility support.

## Implemented Files

- `internal/gamecore/roles/assignment.go`
- `internal/gamecore/roles/assignment_test.go`
- `internal/gamecore/roles/visibility.go`
- `internal/gamecore/roles/visibility_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`
- `internal/server/networking/server.go`
- `internal/server/networking/server_test.go`

## Assignment Engine

1. Deterministic assignment:
- sorts player IDs deterministically
- chooses a valid role template by player count and deterministic match-based hash
- shuffles role profiles deterministically
- maps profiles to sorted player IDs

2. Supported templates:
- explicit valid role mixes for player counts `6` through `12`
- includes authority/prisoner/neutral distributions from design variants
- fallback path for low-count development tests (`<6`) to keep existing harnesses usable

3. State application:
- `roles.ApplyAssignments(...)` updates each player’s `role`, `faction`, and `alignment`
- invoked on match start in server manager

## Hidden-Role Visibility Rules

Applied visibility baseline:
- local player sees full role/faction/alignment
- warden identity remains public (alignment hidden to others)
- all other non-self players have role/faction/alignment redacted

Projection applied to:
- join full snapshot send path
- game start initial snapshot
- per-tick snapshot streaming path

## Tests Added

`internal/gamecore/roles/assignment_test.go`:
- deterministic assignment for same inputs
- coverage for player counts `6-12`
- fallback behavior for small match sizes
- duplicate/empty input validation
- game state assignment application

`internal/gamecore/roles/visibility_test.go`:
- self visibility preserved
- warden public identity with hidden alignment
- non-self/non-warden redaction in full and delta snapshots

`internal/server/game/manager_test.go`:
- start-match role assignment integration for six-player lobby

`internal/server/networking/server_test.go`:
- websocket `game_start` snapshot redaction integration (`TestGameStartSnapshotHidesNonPublicRolesFromOtherPlayers`)

## Verification

Ran continuously during implementation:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/roles ./internal/server/game ./internal/server/networking`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
