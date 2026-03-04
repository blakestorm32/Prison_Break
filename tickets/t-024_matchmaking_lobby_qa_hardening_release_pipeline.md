# T-024 Matchmaking/Lobby, QA Hardening, and Release Pipeline

Status: Implemented (code + tests + build paths)  
Related roadmap ticket: `T-024` in `roadmap_tickets.md`

## Goal

Ship a complete pre-release slice with:
- working lobby/matchmaking flow
- stronger QA gates (integration, determinism, fuzz smoke)
- defined Docker/Android/iOS build paths

## Implemented Files

- `internal/server/matchmaking/service.go`
- `internal/server/matchmaking/service_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`
- `internal/server/game/manager_determinism_test.go`
- `internal/server/networking/server.go`
- `internal/server/networking/server_test.go`
- `internal/server/networking/envelope_fuzz_test.go`
- `internal/shared/protocol/messages.go`
- `internal/shared/protocol/messages_fuzz_test.go`
- `cmd/server/main.go`
- `build/docker/Dockerfile.server`
- `build/docker/docker-compose.yml`
- `build/android/build_android.ps1`
- `build/ios/build_ios.sh`
- `build/README.md`
- `.github/workflows/ci.yml`
- `roadmap_tickets.md`

## Core Behavior

1. Matchmaking service:
- Added `internal/server/matchmaking` service to rank and select lobbies.
- Empty `join_game` now reuses existing joinable lobby first, then creates a lobby only when needed.

2. Lobby discovery protocol:
- Added protocol messages:
  - client -> server: `list_lobbies`
  - server -> client: `lobby_list`
- Added lobby summary schema with player count, open slots, joinability, and readiness metadata.

3. Networking integration:
- WebSocket server now supports `list_lobbies` requests.
- `join_game` includes stale-lobby retry fallback when a selected lobby becomes unavailable.
- Join response now uses live manager min/max constraints instead of hard-coded constants.

4. Server observability endpoints:
- Added `GET /healthz`
- Added `GET /lobbies?include_running=true|false`

5. QA hardening:
- Added deterministic replay integration test:
  - same config + same input sequence -> identical snapshots and final state.
- Added fuzz tests for network/protocol payload decode paths.

6. Release paths:
- Added dedicated server Dockerfile and compose path.
- Added Android and iOS build scripts for Ebiten client packaging.
- Added CI workflow that runs:
  - full tests
  - determinism gate
  - fuzz smoke gate
  - Docker build path check

## Test Coverage Added

- `internal/server/matchmaking/service_test.go`
  - lobby selection priority
  - lobby creation fallback
  - sorting and joinability edge cases

- `internal/server/networking/server_test.go`
  - list-lobbies response and ordering
  - quick join (empty preferred match) lobby reuse
  - new-lobby fallback when only running matches exist

- `internal/server/game/manager_determinism_test.go`
  - deterministic snapshot stream and final-state replay check

- `internal/server/networking/envelope_fuzz_test.go`
- `internal/shared/protocol/messages_fuzz_test.go`

## Verification

Executed:
- `C:\Program Files\Go\bin\gofmt.exe -w <all .go files>`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
