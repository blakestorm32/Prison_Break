# T-031 In-Match Gameplay HUD And Action Surfaces

Status: Implemented (code + tests)  
Related roadmap ticket: `T-031` in `roadmap_tickets.md`

## Goal

Upgrade in-match UX so the HUD provides role-safe situational clarity and action affordances for both desktop and mobile.

## Implemented Files

- `internal/client/render/hud.go`
- `internal/client/render/hud_test.go`
- `internal/client/render/shell.go`
- `internal/client/render/shell_input_test.go`
- `internal/client/input/controller.go`
- `internal/client/input/controller_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Richer in-match HUD lines:
- Added configurable HUD options via `BuildHUDLinesWithOptions`.
- HUD now includes:
  - match/phase/cycle progress (`cycle current/max`)
  - power/alarm/market state
  - local identity (`faction`, `role`) for player sessions
  - objective summary text
  - health/temp health/ammo/room
  - active effects summary
  - cooldown timers (stun/solitary/effect durations)
  - low-resource warnings (critical health, out of ammo)
  - control hint lines

2. Role-safe spectator handling:
- When no local player id is bound (spectator), HUD avoids private role details and shows spectator-safe objective text.

3. Desktop + mobile action affordances:
- Desktop affordances are explicitly shown in HUD control lines.
- Mobile affordances:
  - on narrow screens or active touch sessions, shell draws joystick and action buttons (`FIRE`, `USE`, `RELOAD`) overlays.

4. UX readability improvements:
- Dynamic HUD panel width based on content length, clamped to viewport.
- Panel line rendering is bounded by available screen height to avoid draw overflow on smaller screens.

## Test Coverage Added

`internal/client/render/hud_test.go`:
- verifies objective/effects/cooldowns/control hints appear for local player
- verifies missing-local fallback messaging
- verifies spectator role-safe output (no leaked local role line)
- verifies optional mobile control hint rendering

`internal/client/render/shell_input_test.go`:
- verifies narrow-screen shell enables mobile action surface visibility logic

`internal/client/input/controller_test.go`:
- verifies `MobileLayout()` accessor round-trips configured mobile layout values

## UX/Balancing Notes

- Added “critical health” and “out of ammo” warnings to reduce avoidable deaths and pacing confusion.
- Objective summaries are role/faction-targeted to reduce onboarding friction and improve decision quality during matches.
- Mobile overlays auto-appear under mobile-like conditions to avoid over-cluttering desktop gameplay.

## Verification

Executed:
- `gofmt -w internal/client/render/hud.go internal/client/render/hud_test.go internal/client/render/shell.go internal/client/render/shell_input_test.go internal/client/input/controller.go internal/client/input/controller_test.go`
- `go test ./internal/client/input ./internal/client/render`
- `go test ./...`

Result:
- all tests passing.
