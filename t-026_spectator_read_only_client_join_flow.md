# T-026 Spectator Read-Only Client Join Flow

Status: Implemented (code + tests)  
Related roadmap ticket: `T-026` in `roadmap_tickets.md`

## Goal

Allow non-player spectators to join running matches and receive authoritative snapshots while preventing gameplay command authority.

## Implemented Files

- `internal/shared/protocol/messages.go`
- `internal/server/networking/server.go`
- `internal/server/networking/types.go`
- `internal/server/networking/server_test.go`
- `internal/gamecore/roles/visibility.go`
- `internal/gamecore/roles/visibility_test.go`
- `internal/shared/protocol/messages_fuzz_test.go`
- `internal/server/networking/envelope_fuzz_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Spectator join contract:
- `join_game` now accepts `spectator=true`.
- Spectator joins require `preferred_match_id` and only allow joining `running` matches.

2. Session metadata for UX:
- `join_accepted` now includes:
  - `session_kind`: `player` or `spectator`
  - `can_send_commands`: bool
- This gives clients an explicit switch for player UI vs spectator UI behavior.

3. Read-only spectator binding:
- Spectators bind to match with a non-player viewer identity.
- They are included in snapshot/game_start fanout.
- Gameplay command handlers remain player-only and reject spectator commands with `unauthorized`.

4. Visibility/balance hardening:
- Role projection now treats empty viewer ID as non-privileged.
- Spectators only see public information (e.g., Warden identity remains public, hidden role/faction/alignment for others).
- This prevents hidden-info leaks and protects deduction balance.

## Test Coverage Added

- `internal/server/networking/server_test.go`
  - spectator join requires `preferred_match_id`
  - spectator join rejects unknown match
  - spectator join rejects non-running match
  - spectator can join running match, gets `join_accepted` + `game_start` + streamed delta snapshots
  - spectator gameplay command attempts are rejected
  - player joins still return player session metadata and command authority

- `internal/gamecore/roles/visibility_test.go`
  - empty viewer ID no longer bypasses hidden-info projection

- Fuzz seed improvements:
  - spectator join payload variants in protocol/network envelope fuzz suites

## Verification

Executed:
- `C:\Program Files\Go\bin\gofmt.exe -w <all .go files>`
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/roles`
- `C:\Program Files\Go\bin\go.exe test ./internal/server/networking`
- `C:\Program Files\Go\bin\go.exe test ./internal/shared/protocol`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
