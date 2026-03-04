# T-020 Escape-Route Systems and Nightly Black Market Flow

Status: Implemented (code + tests)  
Related roadmap ticket: `T-020` in `roadmap_tickets.md`

## Goal

Implement deterministic systems for:
- all documented escape routes with required item/state checks
- nightly black market location flow and Gang Leader nightly location control
- Gang Leader succession when the leader dies and gang members survive

## Implemented Files

- `internal/shared/model/types.go`
- `internal/shared/model/input.go`
- `internal/gamecore/escape/routes.go`
- `internal/gamecore/escape/routes_test.go`
- `internal/gamecore/map/black_market.go`
- `internal/gamecore/map/black_market_test.go`
- `internal/gamecore/roles/succession.go`
- `internal/gamecore/roles/succession_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_escape_market_succession_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Escape route validation:
- Added explicit `escape_route` support on `interact` payloads.
- Supported route checks:
  - `courtyard_dig` (Courtyard + shovel)
  - `badge_escape` (Main Corridor + badge)
  - `power_out_escape` (Power Room + power off)
  - `ladder_escape` (Courtyard + two ladders)
  - `roof_helicopter_escape` (Roof Lookout + keys)
- Only living prisoner-side players can complete these routes.
- Successful route attempts move the player to `escaped`.

2. Nightly black market flow:
- Added deterministic nightly black market candidate set and selector.
- On every NIGHT phase start, market location is set deterministically by match/cycle/tick.
- Added explicit `market_room_id` interact payload field so a Gang Leader can set market location during NIGHT.
- Market overrides are limited to approved candidate rooms and rejected for non-leaders or DAY phase.

3. Gang Leader succession:
- Added deterministic succession in roles core:
  - if no living Gang Leader exists and living Gang Members remain, exactly one is promoted
  - selection is deterministic by match/tick/candidate set
  - dead prior leaders are normalized to `gang_member` role

4. Snapshot/input integration:
- Tick delta now emits `black_market_room_id` changes.
- Input validation now rejects unknown escape-route values and invalid market-room targets.

## Test Coverage

`internal/gamecore/escape/routes_test.go`:
- known route registry checks
- route-specific requirement checks
- edge cases for non-prisoners and dead players

`internal/gamecore/map/black_market_test.go`:
- candidate validity checks
- deterministic same-input stability
- varied-input room selection behavior

`internal/gamecore/roles/succession_test.go`:
- deterministic promotion behavior
- no-op when leader remains alive
- no-op when no eligible gang member survives

`internal/server/game/input_queue_test.go`:
- interact payload rejects invalid `escape_route`
- interact payload rejects invalid `market_room_id`

`internal/server/game/manager_escape_market_succession_test.go`:
- integration checks for all escape-route requirements and edge cases
- Gang Leader escape route triggering game over
- nightly market rotation + Gang Leader/night-only override behavior
- gang leader succession after leader death

