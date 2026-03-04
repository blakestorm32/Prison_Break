# T-013 Phase Manager (DAY/NIGHT + Cycle Counter)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-013` in `roadmap_tickets.md`

## Goal

Implement deterministic phase timing and transitions so:
- DAY/NIGHT auto-transition on tick schedule
- cycle counter increments on complete night->day transitions
- phase hooks run on transitions
- cycle progression behaves correctly up to configured max cycles

## Balance Applied

Per your update:
- DAY duration: **5 minutes**
- NIGHT duration: **2 minutes**
- Max escape window: **6 cycles**

At default `30Hz` this resolves to:
- DAY: `9000` ticks
- NIGHT: `3600` ticks

## Implemented Files

- `internal/gamecore/phase/manager.go`
- `internal/gamecore/phase/manager_test.go`
- `internal/server/game/config.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_test.go`
- `internal/shared/constants/simulation.go`

## Core Behavior

1. Phase model:
- phases use deterministic half-open windows `[started_tick, ends_tick)`
- initial phase is DAY

2. Transition rules:
- on boundary tick, phase advances
- `DAY -> NIGHT` does not increment cycle
- `NIGHT -> DAY` increments cycle
- cycle count saturates at configured max (`6`)

3. Hooks:
- `OnDayStart` and `OnNightStart` hook points added
- server hook currently resets alarm state on DAY start

4. Server integration:
- `manager` now advances phase each simulation tick
- snapshot deltas include phase/cycle changes (`delta.phase`, `delta.cycle_count`)
- alarm reset from DAY-start hook is reflected through state/delta

## Test Coverage

`internal/gamecore/phase/manager_test.go`:
- initial phase window correctness
- day->night and night->day transitions
- catch-up transitions in a single advance call
- max-cycle saturation behavior
- day/night hook invocation

`internal/server/game/manager_test.go`:
- configured short durations transition correctly in live tick loop
- DAY-start hook resets alarm state
- cycle count saturates at max in running match
- phase transitions appear in emitted delta snapshots

## Verification

Ran during implementation:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/phase ./internal/server/game ./internal/server/networking`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
