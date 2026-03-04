# T-036 Complete Remaining Ability Content Matrix and Validations

Status: Implemented (code + tests)  
Related roadmap ticket: `T-036` in `roadmap_tickets.md`

## Goal

Close the remaining ability matrix gaps so all listed faction abilities are fully validated, executed in simulation, and covered by integration tests:
- `camera_man`
- `search`
- `detainer`
- `tracker`
- `pick_pocket`
- `hacker`
- `disguise`
- `locksmith`
- `chameleon`

## Implemented Files

- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/server/game/manager_abilities_cards_test.go`
- `roadmap_tickets.md`

## Core Changes

1. Ability payload validation hardening (`CmdUseAbility`):
- rejects missing target-player fields for targeted abilities:
  - `search`
  - `detainer`
  - `tracker`
  - `pick_pocket`
- rejects missing target-door field for:
  - `locksmith`
- keeps non-targeted abilities valid without extra fields:
  - `alarm`, `camera_man`, `hacker`, `disguise`, `chameleon`

2. Ability integration coverage expansion:
- `camera_man`
  - fails outside camera room
  - fails when power is off
  - applies tracked effect only to restricted-zone prisoners
- `detainer`
  - requires same-room target
  - applies solitary duration and locks to assigned cell
  - enforces cooldown on immediate reuse
- `disguise`
  - applies timed effect
  - immediate reuse blocked by cooldown
  - expires and is cleaned deterministically
- `chameleon`
  - applies timed effect
  - expires and is cleaned deterministically
- `locksmith`
  - requires power on
  - unlocks and opens targeted door
  - immediate reuse blocked by cooldown

## Test Coverage Added

`internal/server/game/input_queue_test.go`:
- `TestSubmitInputRejectsAbilityPayloadWithoutRequiredTargets`

`internal/server/game/manager_abilities_cards_test.go`:
- `TestAbilityCameraManRequiresCameraRoomAndPowerAndMarksRestrictedPrisoners`
- `TestAbilityDetainerRequiresSameRoomAndLocksAssignedCell`
- `TestAbilityDisguiseAndChameleonApplyDurationsCooldownAndExpire`
- `TestAbilityLocksmithRequiresPowerAndTargetDoor`

## Verification

Executed:
- `go test ./internal/server/game`
- `go test ./...`

Result:
- all tests passing.
