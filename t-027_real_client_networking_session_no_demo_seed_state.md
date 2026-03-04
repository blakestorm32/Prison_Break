# T-027 Real Client Networking Session (No Demo Seed State)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-027` in `roadmap_tickets.md`

## Goal

Replace demo-seeded client startup with a real websocket join flow:
- connect to server
- send `join_game`
- consume `join_accepted` and `snapshot` / `game_start`
- boot render shell from authoritative network state

## Implemented Files

- `internal/client/netclient/session.go`
- `internal/client/netclient/session_test.go`
- `cmd/client/main.go`
- `roadmap_tickets.md`

## Core Behavior

1. New network session client (`netclient.Session`):
- Added `DialAndJoin(ctx, SessionConfig)` handshake flow.
- Validates websocket URL scheme (`ws://` / `wss://`).
- Sends `join_game` with optional:
  - `player_id`
  - `preferred_match_id`
  - `spectator`
- Waits for both:
  - `join_accepted`
  - initial authoritative state via `snapshot` or `game_start.initial_snapshot`

2. Continuous snapshot ingestion:
- Background read loop applies streamed `snapshot`/`game_start` updates into `SnapshotStore`.
- Captures async server/protocol errors for diagnostics without crashing the render loop immediately.

3. Client entrypoint moved to live network:
- `cmd/client/main.go` no longer seeds hardcoded demo game state.
- Now loads runtime config from env and dials server session first.
- Shell local player id is sourced from session (`player` mode) or empty in spectator mode (read-only UX).

4. UX considerations:
- Configurable launch through env vars:
  - `PRISON_SERVER_WS_URL` (default `ws://127.0.0.1:8080/ws`)
  - `PRISON_PLAYER_NAME`
  - `PRISON_PLAYER_ID`
  - `PRISON_PREFERRED_MATCH_ID`
  - `PRISON_SPECTATOR`
- Window title reflects session mode (`Player` vs `Spectator`).
- Clear startup logging for connection context.

## Test Coverage Added

`internal/client/netclient/session_test.go`:
- join handshake with `join_accepted` + full `snapshot`
- join handshake with `join_accepted` + `game_start.initial_snapshot`
- post-handshake streamed delta application
- server join rejection surfaced as error
- spectator session handshake behavior
- session config normalization defaults
- URL validation and non-websocket scheme rejection
- async runtime error capture from server `error` message
- player id preservation in `join_game` envelope
- recovery from malformed snapshot payload followed by valid snapshot

## Verification

Executed:
- `C:\Program Files\Go\bin\gofmt.exe -w <all .go files>`
- `C:\Program Files\Go\bin\go.exe test ./internal/client/netclient`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
