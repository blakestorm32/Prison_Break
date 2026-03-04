# T-018 Prison Systems: Power, Alarm, and NPC Guard Reaction

Status: Implemented (code + tests)  
Related roadmap ticket: `T-018` in `roadmap_tickets.md`

## Goal

Implement deterministic prison-system rules so:
- power affects cameras, ammo room access, and door behavior
- alarm can be triggered and resolved deterministically
- NPC guards react to restricted-area prisoners during alarm

## Implemented Files

- `internal/gamecore/prison/systems.go`
- `internal/gamecore/prison/systems_test.go`
- `internal/gamecore/map/access_control.go`
- `internal/gamecore/map/access_control_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_prison_systems_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Power state system:
- added deterministic power helpers in `gamecore/prison`
- when power turns **off**:
  - all doors are forced open
  - doors become non-closable (`can_close=false`)
  - camera room already blocked while power off
  - ammo room entry now blocked while power off
- when power turns **on**:
  - doors become closable again

2. Power toggle interaction:
- player in `power_room` can toggle power using empty `interact` payload
- door deltas and `power_on` delta are emitted correctly

3. Alarm activation:
- `use_ability` with `alarm` now works for Warden
- constraints:
  - Warden only
  - DAY phase only
  - once per day
- alarm duration is fixed at `5 seconds` (tick-rate converted)

4. Alarm auto-stop rules:
- alarm ends when duration expires
- alarm auto-stops immediately if no prisoners are in restricted rooms

5. NPC guard reaction:
- while alarm is active and restricted prisoners exist:
  - one NPC guard entity is maintained per restricted zone
  - guards shoot restricted prisoners at fixed cadence (`1 second`)
  - each shot deals `1 heart` (`2` half-hearts)
- guard entities are removed when alarm ends/stops

## Test Coverage

`internal/gamecore/prison/systems_test.go`:
- alarm/guard tick-duration conversion
- power-state door mutation rules (off/on/idempotent)
- restricted-prisoner deterministic filtering

`internal/gamecore/map/access_control_test.go`:
- ammo-room access blocked when power off

`internal/server/game/manager_prison_systems_test.go`:
- power-room interact toggles power and door closability behavior
- alarm ability:
  - activation and deterministic end tick
  - NPC guard spawn
  - restricted-prisoner damage cadence
  - auto-stop when prisoners leave restricted rooms
- once-per-day alarm lock and fixed-duration expiry

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/prison ./internal/gamecore/map ./internal/server/game`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
