# T-006 Server Match/Session Lifecycle Shell

Status: Implemented (code + tests)  
Related roadmap ticket: `T-006` in `roadmap_tickets.md`

## Goal

Implement a server-side lifecycle shell for matches without full gameplay logic:
- create match
- join match
- start match
- run authoritative tick shell
- end match

## Implemented Files

- `internal/server/game/errors.go`
- `internal/server/game/config.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`
- `cmd/server/main.go`

## Lifecycle Features Implemented

1. `Manager.CreateMatch()` creates deterministic match IDs and lobby sessions.
2. `Manager.JoinMatch()` validates player identity, capacity, and joinable status.
3. `Manager.StartMatch()` validates minimum players and transitions match to running state.
4. Authoritative shell tick loop increments match tick IDs using a ticker abstraction.
5. `Manager.EndMatch()` transitions match to `game_over`, records reason/timestamp, and cleanly stops loop.
6. `Manager.Close()` gracefully ends running loops for shutdown handling.
7. Snapshot and event APIs:
- `MatchSnapshot()`
- `ListMatchSnapshots()`
- `LifecycleEvents()`

## Test Coverage

Unit coverage:
- match creation defaults
- join validation errors (invalid ID/name, missing match, duplicate join, full match)
- start validation errors (not enough players, already running, already ended)
- end validation errors (missing match, already ended)
- deterministic list/snapshot ordering
- config normalization and tick interval edge handling

Integration coverage:
- create -> join -> start -> tick progression -> end lifecycle
- lifecycle event ordering/filtering
- ticker shutdown behavior on `EndMatch` and `Close`
- post-end player remapping release (player can join a new match)

Edge cases covered:
- blank end reason defaults to `manual_end`
- tick interval clamps for extreme rates
- join rejection once match has started
- server shutdown forces running matches to `game_over`

## Notes

- Gameplay simulation is intentionally not implemented here; this is lifecycle and loop shell only.
- `go test` could not be executed in this environment because the Go toolchain is not installed.
