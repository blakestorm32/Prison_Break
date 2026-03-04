# T-022 Client Input Layer (Desktop + Mobile Controls)

Status: Implemented (code + tests)  
Related roadmap ticket: `T-022` in `roadmap_tickets.md`

## Goal

Build a client input layer that generates valid network commands from:
- desktop controls (keyboard + mouse)
- mobile controls (touch joystick + touch action buttons)

## Implemented Files

- `internal/client/input/controller.go`
- `internal/client/input/controller_test.go`
- `internal/client/render/camera.go`
- `internal/client/render/camera_test.go`
- `internal/client/render/shell.go`
- `internal/client/render/shell_input_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Deterministic command controller:
- Added `input.Controller` that builds `model.InputCommand` with monotonically increasing `client_seq`.
- Generated commands are server-valid payloads for:
  - `move_intent`
  - `aim_intent`
  - `fire_weapon`
  - `interact`
  - `reload`
- `TargetTick` is set by caller and propagated to every emitted command.

2. Desktop controls:
- Movement:
  - `WASD` or arrow keys.
  - diagonal movement is normalized.
  - sprint via shift.
- Aim:
  - mouse cursor projected from screen space into world space.
- Actions:
  - fire: mouse-left or space.
  - interact: `E`/`F`.
  - reload: `R`.

3. Mobile controls:
- Added default mobile layout with:
  - left joystick region
  - right-side fire/interact/reload button regions
- Joystick produces normalized move vector and auto-sprint at near-max deflection.
- Buttons are edge-triggered (press event) to prevent repeated action spam while held.

4. Edge-trigger action semantics:
- Fire/interact/reload emit once per press transition.
- Move/aim remain continuous while held/active.

5. Shell integration:
- `render.Shell.Update()` now samples input each frame, builds commands with `input.Controller`, and stores outgoing commands in a queue.
- Added:
  - `DrainOutgoingCommands()` for transport layer handoff in later tickets.
  - optional `OnInputCommand` callback.
  - optional `InputSnapshotProvider` override for deterministic tests.

6. Camera support:
- Added `ScreenToWorld` helper to support mouse/touch aim mapping from viewport pixels to authoritative world coordinates.

## Test Coverage

`internal/client/input/controller_test.go`:
- keyboard move/aim/fire command generation and payload correctness
- edge-trigger behavior for action buttons
- touch joystick + touch button behavior
- empty-player-id no-op behavior
- invalid configured weapon fallback to supported default

`internal/client/render/shell_input_test.go`:
- shell queues generated commands from update loop
- command queue drain semantics
- fire edge-trigger behavior across multiple shell update frames

`internal/client/render/camera_test.go`:
- world-to-screen centering
- screen-to-world inversion accuracy
- map clamp bounds behavior

## Verification

Executed:
- `C:\Program Files\Go\bin\go.exe test ./internal/client/...`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
