# T-035 Role Objective And Hidden-Info Presentation Layer

Status: Implemented (code + tests)  
Related roadmap ticket: `T-035` in `roadmap_tickets.md`

## Goal

Complete role-safe hidden-information presentation and add objective-progress indicators that reflect role and alignment variants during live matches.

## Implemented Files

- `internal/gamecore/roles/visibility.go`
- `internal/gamecore/roles/visibility_test.go`
- `internal/client/render/hud.go`
- `internal/client/render/hud_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Hidden-info projection hardened:
- Non-self player snapshots now hide private loadout/intel fields:
  - inventory
  - cards
  - active effects
  - ammo/temp hearts
  - stun/solitary timers
  - last escape-attempt feedback
- Existing role visibility policy remains:
  - viewer sees own full identity/state
  - Warden identity stays public (role + faction)
  - non-Warden role/faction/alignment remain hidden
  - Warden alignment remains hidden from others

2. Role + alignment objective text variants:
- Updated role objective summary to include alignment-aware variants:
  - evil deputy objective messaging differs from good deputy
  - snitch objective messaging varies by alignment
- Neutral-prisoner objective language tuned to opportunistic survival/escape framing.

3. Objective-progress indicators in HUD:
- Added `ObjectiveProgress` line with live, role-specific telemetry.
- Progress includes deterministic match metrics such as:
  - cycles remaining
  - gang/authority alive counts
  - gang-leader alive/escaped states
  - local route readiness counts
  - local alive/escaped status
- Prisoner-specific escape progress + last escape feedback from `T-034` remains integrated.

4. UX consistency updates:
- Desktop control hint remains aligned with panel shortcuts including `X`.
- Spectator path remains role-safe and does not expose hidden local-role internals.

## Test Coverage Added/Updated

- `internal/gamecore/roles/visibility_test.go`
  - validates self-preservation vs hidden-other behavior
  - validates private-field redaction for non-self players
  - validates policy across all role variants

- `internal/client/render/hud_test.go`
  - verifies `ObjectiveProgress` rendering
  - verifies control hint consistency
  - verifies evil deputy alignment variant objective/progress messaging

- Existing networking visibility tests remain green under hardened projection policy.

## Verification

Executed:
- `gofmt -w internal/gamecore/roles/visibility.go internal/gamecore/roles/visibility_test.go internal/client/render/hud.go internal/client/render/hud_test.go`
- `go test ./internal/gamecore/roles ./internal/client/render ./internal/server/networking`
- `go test ./...`

Result:
- all tests passing.

