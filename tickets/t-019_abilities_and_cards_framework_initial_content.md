# T-019 Abilities and Cards Framework (Initial Content)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-019` in `roadmap_tickets.md`

## Goal

Implement deterministic ability/card systems so:
- ability cooldowns and once-per-day checks are enforced
- card slot limit (`3`) is enforced
- initial listed ability/card effects execute in simulation

## Implemented Files

- `internal/gamecore/abilities/framework.go`
- `internal/gamecore/abilities/framework_test.go`
- `internal/gamecore/cards/framework.go`
- `internal/gamecore/cards/framework_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_abilities_cards_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/engine/physics/motion.go`
- `internal/engine/physics/motion_test.go`
- `internal/gamecore/combat/engine.go`
- `internal/shared/model/types.go`
- `roadmap_tickets.md`

## Core Behavior

1. Ability framework:
- known-ability registry with deterministic specs:
  - scope (`warden_only`, `authority`, `prisoner`)
  - cooldown seconds
  - once-per-day policy
- helper APIs:
  - known ability checks
  - player eligibility checks
  - cooldown tick conversion
  - effect duration tick conversion

2. Card framework:
- known-card registry
- deterministic card inventory operations
  - `AddCard` with max slot limit `3`
  - `RemoveCard`
  - `HasCard`
- deterministic item-grab card roll helper
- speed and door-stop duration helpers

3. Manager command integration:
- `CmdUseAbility` now executes non-placeholder ability effects with:
  - cooldown enforcement
  - once-per-day enforcement
  - faction/role scope checks
- implemented initial ability effects:
  - `alarm`
  - `search` (contraband confiscation)
  - `camera_man` (restricted-prisoner tracking mark)
  - `detainer` (solitary/lock)
  - `tracker` (tracking mark)
  - `pick_pocket` (deterministic item steal)
  - `hacker` (power toggle)
  - `disguise` (self disguise effect)
  - `locksmith` (door unlock/open)
  - `chameleon` (self chameleon effect)

- `CmdUseCard` now executes initial card effects and consumes card on success:
  - `morphine`, `bullet`, `money`, `speed`, `armor_plate`, `lock_snap`
  - `item_steal`, `item_grab`, `scrap_bundle`, `door_stop`, `get_out_of_jail_free`

4. Timed state handling:
- expired player effects are cleaned deterministically per tick
- expired door-stop block states are cleaned deterministically per tick

5. Movement integration:
- active `speed_boost` effect modifies movement step

6. Validation hardening:
- input validation now rejects unknown ability/card payload values

## Test Coverage

`internal/gamecore/abilities/framework_test.go`:
- ability registry/spec checks
- cooldown and once-per-day flags
- role/faction scope checks
- effect duration conversion

`internal/gamecore/cards/framework_test.go`:
- card known checks
- card slot limit (`3`) enforcement
- deterministic card storage behavior
- deterministic item-grab and duration conversion

`internal/server/game/manager_abilities_cards_test.go`:
- search confiscation + once-per-day block
- tracker cooldown enforcement and reuse after cooldown
- pick-pocket + hacker effects
- morphine healing + consumption behavior
- speed card movement boost + timed expiry
- door-stop blocking behavior until expiry
- get-out-of-jail-free clearing solitary lock

`internal/server/game/input_queue_test.go`:
- unknown ability/card payload rejection

`internal/engine/physics/motion_test.go`:
- speed-boost movement behavior and expiry fallback

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/abilities ./internal/gamecore/cards ./internal/engine/physics ./internal/server/game`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
