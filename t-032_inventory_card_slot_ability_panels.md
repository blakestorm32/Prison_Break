# T-032 Inventory, Card-Slot, And Ability Panels

Status: Implemented (code + tests)  
Related roadmap ticket: `T-032` in `roadmap_tickets.md`

## Goal

Allow players to inspect inventory/cards/abilities and activate them through explicit in-match UI panels (not only raw movement/combat keybinds).

## Implemented Files

- `internal/client/render/shell.go`
- `internal/client/render/shell_panels.go`
- `internal/client/render/shell_panels_test.go`
- `internal/client/input/controller.go`
- `internal/client/input/controller_test.go`
- `internal/client/render/hud.go`
- `roadmap_tickets.md`

## Core Behavior

1. New in-match action panels:
- Added three panel modes in shell:
  - Inventory
  - Cards
  - Abilities
- Panel tabs are rendered in-match and can be toggled by:
  - Desktop: `Tab` (Inventory), `C` (Cards), `V` (Abilities)
  - Mobile: touch panel tabs
- Active panel supports selection and activation:
  - Desktop: `[` / `]` for previous/next, `Enter` for use
  - Mobile: touch `Prev`, `Next`, `Use` buttons

2. UI-driven gameplay command dispatch:
- Added controller builders to preserve authoritative client-seq ordering:
  - `BuildUseItemCommand`
  - `BuildUseCardCommand`
  - `BuildUseAbilityCommand`
- Panel actions now generate:
  - `CmdUseItem` from selected inventory stack
  - `CmdUseCard` from selected card
  - `CmdUseAbility` from selected ability

3. Smart target defaults for usability:
- For targeted abilities/cards, panel flow auto-populates likely targets:
  - target player: nearest valid non-self player, preferring same room
  - target door: first door linked to local room (fallback first known door)
- This reduces “press use and nothing happens” friction while staying deterministic.

4. Mobile accessibility and UX:
- Touch snapshot augmentation maps tab/button touches into panel control intents.
- Existing action affordances plus panel controls give mobile users end-to-end panel interaction parity.

5. HUD control hints updated:
- Desktop hints now include panel controls (`Tab/C/V`, `[ ]`, `Enter`).
- Mobile hint line now mentions panel tab/use surfaces.

## Test Coverage Added

`internal/client/render/shell_panels_test.go`:
- card panel emits `CmdUseCard` with selected card payload
- inventory panel navigation + use emits `CmdUseItem` for selected stack
- ability panel emits `CmdUseAbility` and chooses in-room target player
- mobile touch mapping sets panel control inputs for tabs/use buttons

`internal/client/input/controller_test.go`:
- validates use-card command payload + sequence progression
- rejects unknown ability in use-ability builder
- rejects empty item payload in use-item builder

## UX/Balancing Notes

- Panel usage avoids requiring memorized command strings or future niche keybinds.
- Target autofill lowers failure rate for role abilities and situational cards during high-pressure combat.
- Sequence generation remains centralized in input controller to prevent command duplication/ordering bugs over long sessions.

## Verification

Executed:
- `gofmt -w internal/client/input/controller.go internal/client/input/controller_test.go internal/client/render/shell.go internal/client/render/shell_panels.go internal/client/render/shell_panels_test.go internal/client/render/hud.go`
- `go test ./internal/client/input ./internal/client/render`
- `go test ./...`

Result:
- all tests passing.
