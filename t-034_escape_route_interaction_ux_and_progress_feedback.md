# T-034 Escape-Route Interaction UX And Progress Feedback

Status: Implemented (code + tests)  
Related roadmap ticket: `T-034` in `roadmap_tickets.md`

## Goal

Expose escape-route requirements/status clearly in-client and provide authoritative success/fail feedback (with server reason) for escape attempts.

## Implemented Files

- `internal/shared/model/types.go`
- `internal/shared/model/state.go`
- `internal/gamecore/escape/routes.go`
- `internal/gamecore/escape/routes_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_escape_feedback_test.go`
- `internal/client/input/controller.go`
- `internal/client/input/controller_test.go`
- `internal/client/render/shell.go`
- `internal/client/render/shell_panels.go`
- `internal/client/render/shell_panels_test.go`
- `internal/client/render/hud.go`
- `internal/client/render/hud_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Escape route evaluation model:
- Added deterministic route evaluation APIs:
  - `KnownRoutes`
  - `EvaluateRoute`
  - `EvaluateAllRoutes`
  - `RouteRequirement` and `RouteEvaluation`
- Each route now reports:
  - `can_attempt`
  - unmet requirement reason
  - requirement checklist state

2. Authoritative escape attempt feedback from server:
- Added per-player escape attempt feedback payload on state:
  - route attempted
  - success/failed status
  - server-side reason text
  - tick id
- Server now records feedback for every route attempt (successful and failed).

3. Escape interaction UX panel:
- Added new in-match `Escape` panel tab:
  - desktop shortcut `X`
  - mobile touch tab support
- Panel supports:
  - browse route list
  - per-route readiness/progress summary
  - submit escape attempt via interact command
  - show latest authoritative attempt feedback

4. HUD progress and feedback:
- Prisoner HUD now includes:
  - route readiness summary across all routes
  - latest authoritative escape feedback line when available
- Desktop controls hint updated to include `X` panel shortcut.

5. Input-controller support:
- Added explicit `BuildInteractCommand` builder to emit interact payloads from panels with proper sequence ordering.

## Test Coverage Added

- `internal/gamecore/escape/routes_test.go`
  - deterministic requirement feedback and ordering
  - full route matrix behavior remains validated

- `internal/server/game/manager_escape_feedback_test.go`
  - failed route attempt emits failed status + requirement-based reason
  - successful attempt emits success status + success reason
  - room transition to escaped remains correct

- `internal/client/input/controller_test.go`
  - interact builder success + sequencing for known escape routes
  - rejection for unknown escape route payloads

- `internal/client/render/shell_panels_test.go`
  - escape panel emits route interact command
  - touch augmentation includes escape tab input

- `internal/client/render/hud_test.go`
  - escape progress + updated panel controls present in HUD output

## UX/Balancing Notes

- Route list always visible in one panel, with per-route readiness summary for quick decision making.
- Attempts are still server-authoritative; client-side readiness is guidance only.
- Failure reasons are deterministic and requirement-driven, reducing “nothing happened” confusion.

## Verification

Executed:
- `gofmt -w internal/shared/model/types.go internal/shared/model/state.go internal/gamecore/escape/routes.go internal/gamecore/escape/routes_test.go internal/server/game/manager.go internal/server/game/manager_escape_feedback_test.go internal/client/input/controller.go internal/client/input/controller_test.go internal/client/render/shell.go internal/client/render/shell_panels.go internal/client/render/shell_panels_test.go internal/client/render/hud.go internal/client/render/hud_test.go`
- `go test ./internal/gamecore/escape ./internal/client/input ./internal/client/render ./internal/server/game`
- `go test ./...`

Result:
- all tests passing.

