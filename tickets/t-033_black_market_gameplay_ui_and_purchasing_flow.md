# T-033 Black-Market Gameplay UI And Purchasing Flow

Status: Implemented (code + tests)  
Related roadmap ticket: `T-033` in `roadmap_tickets.md`

## Goal

Allow eligible players to see nightly black-market context, browse purchase options in-client, and complete authoritative transactions that mutate inventory and currency safely.

## Implemented Files

- `internal/shared/model/types.go`
- `internal/shared/model/input.go`
- `internal/gamecore/items/market.go`
- `internal/gamecore/items/market_test.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/input_queue_test.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_black_market_purchase_test.go`
- `internal/client/input/controller.go`
- `internal/client/input/controller_test.go`
- `internal/client/render/shell.go`
- `internal/client/render/shell_panels.go`
- `internal/client/render/shell_panels_test.go`
- `internal/client/render/hud.go`
- `roadmap_tickets.md`

## Core Behavior

1. New command path for purchases:
- Added `CmdBlackMarketBuy` and `BlackMarketPurchasePayload`.
- Input queue now validates purchase payloads against the market catalog.

2. Shared market catalog with balancing defaults:
- Added deterministic black-market offer catalog in `internal/gamecore/items/market.go`.
- Prices are money-card based and tuned to current 3-card cap:
  - low tier: 1 card
  - mid tier: 2 cards
  - high tier: 3 cards

3. Server-authoritative transaction rules:
- Purchases only succeed when all conditions hold:
  - alive prisoner
  - `NIGHT` phase
  - player located in active market room
  - enough `money` cards
  - inventory can accept the item
- Currency is consumed only on successful purchase.
- Added hard one-per-match enforcement for `golden_bullet`.

4. UX and panel flow:
- Added fourth in-match panel tab: `Market` (desktop shortcut `B`, mobile touch tab).
- Market panel supports browse (`Prev`/`Next`) + buy (`Use`) with guardrails:
  - affordability and contextual eligibility checks before sending command
  - status text with money count, cost, and block reason
  - list rendering now scrolls with selected index so deeper entries remain visible
- HUD hint line updated to include market tab control.

5. Currency handling correction:
- `Money` card can no longer be consumed via generic `use_card`.
- It remains reserved for black-market economy flows.

## Test Coverage Added

- `internal/gamecore/items/market_test.go`
  - catalog determinism and validity
  - lookup and inclusion rules

- `internal/server/game/input_queue_test.go`
  - rejects unknown black-market purchase items
  - accepts known black-market purchase payloads

- `internal/server/game/manager_black_market_purchase_test.go`
  - successful purchase consumes money and grants inventory item
  - phase/location gating behavior
  - insufficient-funds and authority-player rejection
  - golden bullet one-per-match lock (including repeat attempts)
  - money card cannot be consumed via `use_card`

- `internal/client/input/controller_test.go`
  - purchase command builder payload/sequence validation
  - rejects non-catalog items

- `internal/client/render/shell_panels_test.go`
  - market panel emits purchase command when eligible and affordable
  - market panel blocks command generation when unaffordable
  - mobile touch maps market tab interaction

## Verification

Executed:
- `gofmt -w internal/shared/model/types.go internal/shared/model/input.go internal/gamecore/items/market.go internal/gamecore/items/market_test.go internal/client/input/controller.go internal/client/input/controller_test.go internal/server/game/input_queue.go internal/server/game/input_queue_test.go internal/server/game/manager.go internal/server/game/manager_black_market_purchase_test.go internal/client/render/shell.go internal/client/render/hud.go internal/client/render/shell_panels.go internal/client/render/shell_panels_test.go`
- `go test ./internal/gamecore/items ./internal/client/input ./internal/client/render ./internal/server/game`
- `go test ./...`

Result:
- all tests passing.

