# T-028 Connect Client Input Pipeline To Live Server Command Dispatch

Status: Implemented (code + tests)  
Related roadmap ticket: `T-028` in `roadmap_tickets.md`

## Goal

Route real in-match client gameplay inputs to the live websocket session so shell-generated commands (`move/aim/interact/fire/reload`) are serialized and sent to the authoritative server.

## Implemented Files

- `cmd/client/main.go`
- `internal/client/netclient/session.go`
- `internal/client/netclient/session_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Session send pipeline and guardrails:
- Added session-side outgoing queue and writer loop.
- Added `SendInputCommand(model.InputCommand) error`.
- Enforces session permissions:
  - spectator sessions return `ErrReadOnlySession`
  - closed sessions return `ErrSessionClosed`
- Applies authoritative local player identity to outbound commands before sending.
- Returns `ErrSendQueueFull` when the client-side dispatch queue is saturated.

2. Client shell -> network wiring:
- `cmd/client/main.go` now registers `OnInputCommand` with the render shell.
- Every shell command is dispatched through `session.SendInputCommand`.
- Added throttled warning logs for dispatch failures to avoid per-frame log spam.
- Removed pre-T-028 no-op input snapshot override, so real desktop/mobile input capture now feeds live command transport.

3. Session configuration defaults:
- Extended session config defaults/normalization with:
  - `WriteTimeout`
  - `SendQueueDepth`

## Test Coverage Added

`internal/client/netclient/session_test.go`:
- integration: player session command dispatch reaches server as `player_input`
- integration: spoofed `player_id` in outbound command is overwritten with authoritative local session player id
- integration: spectator command dispatch returns `ErrReadOnlySession`
- integration: dispatch after `Close()` returns `ErrSessionClosed`
- unit: saturated send queue returns `ErrSendQueueFull`
- config normalization test expanded for `WriteTimeout` and `SendQueueDepth` defaults

## Verification

Executed:
- `gofmt -w cmd/client/main.go internal/client/netclient/session.go internal/client/netclient/session_test.go`
- `go test ./internal/client/netclient`
- `go test ./...`

Result:
- all tests passing.
