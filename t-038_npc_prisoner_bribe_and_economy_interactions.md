# T-038 Implement NPC Prisoner Bribe and Economy Interactions

Status: Implemented (code + tests)  
Related roadmap ticket: `T-038` in `roadmap_tickets.md`

## Goal

Implement deterministic NPC-prisoner bribe gameplay so money cards and black-market economy influence behavior and outcomes.

## Implemented Files

- `internal/shared/model/types.go`
- `internal/server/game/manager.go`
- `internal/server/game/manager_items_test.go`
- `internal/server/game/manager_npc_bribe_economy_test.go`
- `internal/client/render/shell_panels.go`
- `internal/client/render/shell_panels_test.go`
- `internal/client/render/shell.go`
- `roadmap_tickets.md`

## Core Changes

1. New entity kind and NPC spawning:
- Added `npc_prisoner` entity kind.
- Match start now spawns deterministic NPC prisoner entities in stable room set:
  - `cafeteria`
  - `courtyard`
  - `cell_block_a`

2. Deterministic economy-linked offers:
- Each NPC prisoner gets a deterministic bribe offer derived from:
  - match id
  - room id
  - entity id
  - cycle
- Offer selection comes from black-market catalog and mirrors market price.
- Balance guards:
  - excludes `golden_bullet`
  - excludes direct firearm offers (`pistol`, `hunting_rifle`)

3. Money-card bribe flow:
- `money` card can target an NPC prisoner entity at night.
- Bribe is valid only when:
  - player is alive prisoner faction
  - phase is night
  - player and NPC are in same room
  - NPC has stock and valid offer state
- Multi-card pricing supported:
  - progress accumulates against offer cost
  - final payment grants offered item
  - stock decremented to zero after successful dispense
- UX/fairness behavior:
  - final payment does not consume card if inventory cannot receive item
  - sold-out/invalid attempts preserve the card

4. Nightly refresh lifecycle:
- On each transition into night:
  - NPC offers are re-rolled deterministically for the cycle
  - stock resets
  - payer/progress state resets
- Refreshed NPC entities are emitted as changed-entity deltas.

5. Client targeting UX:
- Money-card panel payload now auto-targets nearest in-room `npc_prisoner` entity.
- Added dedicated render color for `npc_prisoner` entities.

6. Compatibility cleanup:
- Updated item/entity tests to avoid assuming only dropped-item entities exist after match start.

## Test Coverage Added

`internal/server/game/manager_npc_bribe_economy_test.go`:
- `TestNPCPrisonerEntitiesSpawnWithBlackMarketLinkedOffers`
- `TestMoneyCardBribeProgressesAndDispensesItemDeterministically`
- `TestMoneyCardBribeGatesByPhaseRoomAndFaction`
- `TestNPCPrisonerBribeOffersRefreshOnNightStart`

`internal/client/render/shell_panels_test.go`:
- `TestShellActionPanelMoneyCardTargetsNearestLocalNPCPrisoner`

`internal/server/game/manager_items_test.go`:
- updated dropped-item assertions to filter by entity kind in mixed-entity world.

## Verification

Executed:
- `go test ./internal/server/game`
- `go test ./internal/client/render`
- `go test ./...`

Result:
- all tests passing.
