# T-008 Input Queue and Command Validation Pipeline

Status: Implemented (code + tests)  
Related roadmap ticket: `T-008` in `roadmap_tickets.md`

## Goal

Implement authoritative input ingestion that:
- rejects invalid/late/spam commands
- deduplicates by `(player_id, client_seq)`
- schedules accepted commands into deterministic per-tick buffers

## Implemented Files

- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/server/game/errors.go` (input-specific error types)
- `internal/server/game/manager.go` (session queue state wiring)
- `internal/server/networking/server.go` (message -> command submission bridge)
- `internal/server/networking/server_test.go` (protocol-level validation behavior)

## Pipeline Behavior

1. Command validation:
- rejects unsupported command types
- rejects malformed or missing required payloads
- rejects unknown/mismatched players
- rejects commands when match is not running

2. Deduplication:
- each player command is keyed by `client_seq`
- duplicate `(player_id, client_seq)` rejected

3. Rate limiting:
- per-player cap per tick window enforced via `constants.MaxAcceptedCommandsPerPlayerPerTick`

4. Scheduling:
- default scheduling: current tick + 1
- `target_tick` clamped to `[current+1, current+2]`
- late discrete commands dropped
- late continuous commands (move/aim) rescheduled to `current+1`

5. Deterministic queue ordering:
- consume order is sorted by:
  - `ingress_seq`
  - `player_id`
  - `client_seq`
  - command type
  - payload bytes (final tie-break)

## Networking Integration

- `player_input`, `ability_use`, and `card_use` now submit into the manager queue.
- protocol errors map domain failures to stable error codes:
  - invalid command/payload -> `invalid_payload`
  - duplicate/too-late -> `out_of_date_command`
  - rate limit -> `rate_limited`
  - unauthorized state/player mismatch -> `unauthorized`

## Test Coverage

Game pipeline tests:
- invalid command and payload rejection
- non-running match rejection
- scheduling and ingress sequencing
- late discrete drop + late continuous reschedule
- rate limiting with window reset after tick advance
- duplicate command rejection
- deterministic consume and tick-bucket deletion
- pending counts missing-match behavior

Networking integration tests:
- command rejected before join
- command accepted and queued after join+start
- duplicate and rate-limit protocol error mapping

## Verification

Ran:
- `go test ./...`

Result:
- all tests passing across `internal/server/game`, `internal/server/networking`, and existing packages.
