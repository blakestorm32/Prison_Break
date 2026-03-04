# T-017 Combat System and Authority Penalties

Status: Implemented (code + tests)  
Related roadmap ticket: `T-017` in `roadmap_tickets.md`

## Goal

Implement deterministic combat rules for:
- hearts damage/death
- shiv, gun, golden bullet, baton behavior
- stun and knockback effects
- unjust authority shooting penalties

## Implemented Files

- `internal/gamecore/combat/engine.go`
- `internal/gamecore/combat/engine_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_combat_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/engine/physics/motion.go`
- `internal/engine/physics/motion_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Role loadouts at match start:
- Warden: `5 hearts` (`10` half-hearts), `3 bullets`
- Deputy: `4 hearts` (`8` half-hearts), `3 bullets`
- Prisoner/neutral roles: `3 hearts` (`6` half-hearts), `0 bullets`

2. Supported combat weapons (`CmdFireWeapon`):
- `pistol`
- `hunting_rifle`
- `shiv`
- `baton` (server-recognized combat weapon token)

3. Damage and effects:
- `shiv`: `0.5 heart` damage (`1` half-heart), requires shiv in inventory
- firearms: `1 heart` damage (`2` half-hearts), consume one standard bullet
- `golden_bullet`: `2 hearts` damage (`4` half-hearts), consumes golden bullet item (does not consume standard ammo)
- `baton`: no heart damage, applies deterministic knockback + `3s` stun

4. Action gating:
- dead, stunned, or solitary-penalized players cannot execute actions (except aim intent)
- movement is blocked while solitary penalty is active

5. Authority unjust-shoot penalty:
- applies when authority firearm shot hits a player during DAY and target is not exempt
- penalty sets solitary lock through end of day (`phase.ends_tick - 1`) and locks shooter to assigned cell

Exemptions:
- target in restricted room
- power is off
- target carrying illegal contraband (gun/shiv/ladder/shovel/wire cutters)

6. Deterministic targeting:
- nearest valid target to aim point within weapon range and aim-assist radius
- ties resolved deterministically by player ID

## Test Coverage

`internal/gamecore/combat/engine_test.go`:
- role loadout assignment
- deterministic target selection tie-breaks
- weapon eligibility rules
- shot cost + golden bullet consumption
- temp-heart and base-heart damage accounting
- unjust-shot penalty exemptions and day-only application

`internal/server/game/manager_combat_test.go`:
- manager applies role loadouts on start
- pistol damage + ammo spend + unjust authority penalty and movement lock
- exemption cases: restricted area, power off, contraband
- baton (authority-only) stun/knockback without damage
- shiv inventory requirement and half-heart damage
- golden bullet damage and item consumption

`internal/server/game/input_queue_test.go`:
- fire-weapon validation accepts baton token and rejects unknown weapons

`internal/engine/physics/motion_test.go`:
- solitary penalty blocks movement

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/gamecore/combat ./internal/engine/physics ./internal/server/game`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
