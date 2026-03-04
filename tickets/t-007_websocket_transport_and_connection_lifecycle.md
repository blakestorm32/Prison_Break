# T-007 WebSocket Transport and Connection Lifecycle

Status: Implemented (code + tests)  
Related roadmap ticket: `T-007` in `roadmap_tickets.md`

## Goal

Implement WebSocket transport and connection lifecycle handling so clients can:
- connect/disconnect
- send protocol messages (join, ping, gameplay commands)
- receive server events (join accepted, game start, broadcast events, errors)

## Implemented Files

- `internal/server/networking/config.go`
- `internal/server/networking/types.go`
- `internal/server/networking/errors.go`
- `internal/server/networking/server.go`
- `internal/server/networking/server_test.go`
- `cmd/server/main.go`

## Transport and Lifecycle Features

1. WebSocket upgrade endpoint handler (`HandleWebSocket`).
2. Connection lifecycle tracking:
- connected/disconnected state
- active connection snapshots
- lifecycle event log
3. Read/write pumps with:
- read size limit
- write deadlines
- ping/pong keepalive
- non-blocking send queue with overflow handling
4. Protocol handling:
- `join_game` -> `join_accepted`
- `ping` -> `pong`
- `leave_match` disconnect path
- gameplay command gate (requires successful join)
- invalid envelopes -> structured protocol error response
5. Match-scoped server event delivery via `BroadcastToMatch`.
6. Auto-start bridge:
- attempts match start after joins
- emits `game_start` messages when minimum players are reached.

## Test Coverage

Unit + integration coverage in `internal/server/networking/server_test.go`:
- connect/disconnect lifecycle events and connection count transitions
- join acceptance and connection-to-player/match binding
- ping/pong request-response behavior
- invalid JSON envelope error response path
- match broadcast delivery to all participants
- gameplay command authorization before/after join
- auto-start `game_start` event emission when min players threshold is met

Edge cases covered:
- protocol errors for malformed payloads
- join-required enforcement for gameplay commands
- deterministic handling of multi-client match broadcasts

## Notes

- Uses `github.com/gorilla/websocket` for WebSocket transport.
- `cmd/server/main.go` now boots a real HTTP server exposing `/ws` and defaulting to `:8080` (override with `PRISON_SERVER_ADDR`).
