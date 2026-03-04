# T-015 Win-Condition Engine (Global + Role-Specific)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-015` in `roadmap_tickets.md`

## Goal

Implement deterministic game-ending evaluation and role-specific winner resolution.

## Implemented Files

- `internal/gamecore/winconditions/evaluator.go`
- `internal/gamecore/winconditions/evaluator_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`

## Core Behavior

1. Deterministic global reason priority:
- `gang_leader_escaped`
- `warden_died`
- `all_gang_members_dead`
- `max_cycles_reached`

2. Role-specific outcome mapping:
- resolves winner player IDs by role/alignment based on reason
- winner list is sorted deterministically

3. Server loop integration:
- win evaluation runs after per-tick command application and phase advancement
- on game over:
  - status transitions to `game_over`
  - ended reason/time are recorded
  - game over state is attached to game state and snapshot delta
  - input queues and player-match bindings are cleared
  - match-ended lifecycle event is emitted

## Test Coverage

`internal/gamecore/winconditions/evaluator_test.go`:
- reason detection and priority ordering
- role-specific winner resolution
- deterministic winner ordering

`internal/server/game/manager_test.go`:
- auto-end on max cycles reached
- auto-end on warden death with gang winners
- auto-end on all gang members dead with authority winners
- priority case where gang leader escape beats simultaneous warden death

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/winconditions ./internal/server/game ./internal/server/networking`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
