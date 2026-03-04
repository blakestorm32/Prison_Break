# T-009 Snapshot Pipeline (Full on Join, Delta per Tick)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-009` in `roadmap_tickets.md`

## Goal

Implement authoritative snapshot delivery so clients can:
- receive a full snapshot immediately after joining
- receive ordered, tick-tagged delta snapshots each server tick
- consume server ack metadata (`player_acks`) for reconciliation

## Implemented Files

- `internal/server/networking/server.go`
- `internal/server/networking/server_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`

## Snapshot Delivery Behavior

1. Full snapshot on join:
- after successful `join_game`, server sends `join_accepted`
- server then sends `snapshot` (`kind=full`) for the joined match/player

2. Delta snapshots per tick:
- server runs a snapshot dispatch loop at tick cadence
- for each connected player in a match, server fetches snapshots using `SnapshotsSince(last_sent_tick)`
- server sends ordered `snapshot` messages (`kind=delta`, `tick_id`, `base_tick_id`)
- per-connection snapshot cursor prevents duplicate delivery

3. Snapshot state contract:
- tick loop stores history in manager (`snapshotHistory`)
- full snapshots include full game state plus `player_acks`
- delta snapshots include changed sections plus `player_acks`

4. Ack message handling:
- `ack_snapshot` payload is decoded and tracked per connection (`client_tick_id`, `last_processed_client_seq`)
- values are stored monotonically for future reconciliation/observability work

## Tests Added/Updated

Game manager tests (`internal/server/game/manager_test.go`):
- full snapshot includes current state and is deep-copy safe
- `SnapshotsSince` returns ordered deltas with expected tick/base tick progression
- delta history includes command effects and processed client sequence ack

Networking tests (`internal/server/networking/server_test.go`):
- join flow emits a full `snapshot` message in addition to `join_accepted`
- running match streams delta `snapshot` messages as ticks advance
- streamed deltas carry processed command acks and changed player data
- existing broadcast test updated to ignore interleaved snapshot traffic safely

## Verification

Ran:
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing across `internal/server/game`, `internal/server/networking`, and existing packages.
