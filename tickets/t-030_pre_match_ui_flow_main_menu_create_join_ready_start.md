# T-030 Pre-Match UI Flow (Main Menu, Create/Join Lobby, Ready/Start)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-030` in `roadmap_tickets.md`

## Goal

Provide an in-client pre-match flow so players can go from launch -> lobby selection/join -> waiting lobby -> match start without terminal/manual API calls.

## Implemented Files

- `cmd/client/main.go`
- `internal/client/render/client_app.go`
- `internal/client/prematch/flow.go`
- `internal/client/prematch/flow_test.go`
- `internal/client/netclient/lobbies.go`
- `internal/client/netclient/lobbies_test.go`
- `internal/client/netclient/session.go`
- `internal/client/netclient/session_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. New pre-match client app wrapper:
- Added `render.ClientApp` as the new Ebiten entry flow.
- Stages:
  - `main_menu`
  - `lobby_list`
  - `connecting`
  - `lobby_wait`
  - `in_match`
  - `error_notice`
- Pre-match controls:
  - main menu: up/down + enter
  - lobby list: up/down + enter to join + `R` refresh + `Esc` back
  - connecting/lobby wait/in match: `Esc` exits to menu safely

2. Launch UX:
- `cmd/client/main.go` now boots into pre-match menu by default instead of immediate network join.
- Gameplay shell is created only after successful join and match start conditions.

3. Lobby discovery support:
- Added `netclient.FetchLobbies(ctx, serverURL, includeRunning)` using server `/lobbies` endpoint.
- Converts ws/wss URLs to the matching http/https `/lobbies` URL.

4. Create/join + ready/start UX semantics:
- `Quick Play (create/join)` uses existing server auto-matchmaking behavior.
  - joins an open lobby if one exists
  - creates a lobby when none are joinable
- `Browse Lobbies` allows explicit join by selected match ID.
- Lobby wait view shows player count, min/max, and ready/start readiness.
- Start transition is automatic when server moves lobby status to running (current authoritative behavior).

5. Session metadata support:
- `netclient.Session` now tracks and exposes `MinPlayers()` / `MaxPlayers()` from `join_accepted` for lobby UI clarity.

## Test Coverage Added

`internal/client/prematch/flow_test.go`:
- main menu -> quick play -> connecting
- browse lobbies selection and join intent
- empty-lobby edge case for join
- lobby wait -> running transition to in-match
- error stage handling and return-to-menu flow

`internal/client/netclient/lobbies_test.go`:
- successful lobby fetch/parse
- include-running query behavior
- unsupported URL scheme rejection
- non-200 HTTP status handling
- ws/wss to http/https URL conversion behavior

`internal/client/netclient/session_test.go`:
- added checks for `MinPlayers()` / `MaxPlayers()` on player and spectator sessions

## UX/Balancing Notes

- Pre-match now prevents accidental gameplay input before match start.
- Lobby status surfaces readiness clearly (players/minimum/start readiness) to reduce confusion.
- Async operations (lobby fetch/connect) run with timeouts and error states to avoid frozen UI during network issues.

## Verification

Executed:
- `gofmt -w cmd/client/main.go internal/client/netclient/session.go internal/client/netclient/session_test.go internal/client/netclient/lobbies.go internal/client/netclient/lobbies_test.go internal/client/prematch/flow.go internal/client/prematch/flow_test.go internal/client/render/client_app.go`
- `go test ./internal/client/netclient ./internal/client/prematch ./internal/client/render`
- `go test ./...`

Result:
- all tests passing.
