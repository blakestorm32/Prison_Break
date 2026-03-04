# T-002 Deterministic Server-Authoritative Tick Contract

Status: Draft v1 (implementation baseline)  
Related roadmap ticket: `T-002` in `roadmap_tickets.md`

## 1. Goal

Define a deterministic simulation contract for a server-authoritative real-time match so implementation teams can build networking, gamecore, and client reconciliation against one source of truth.

This document locks:
- fixed tick model
- input ingestion and ordering
- RNG policy
- client reconciliation behavior

## 2. Scope and Non-Goals

In scope:
- one match simulation contract (5-12 players)
- authoritative server behavior
- client prediction/reconciliation for movement and responsiveness

Out of scope:
- full message schema details (`T-004`)
- final numeric tuning for every gameplay parameter
- matchmaking/persistence

## 3. Determinism Rules (Hard Requirements)

1. All game-state mutation happens on one simulation thread per match.
2. Simulation uses fixed timestep only (no variable `dt`).
3. `internal/gamecore` must not read wall clock time, OS randomness, or network state directly.
4. Randomness must come only from the match RNG service defined here.
5. Iteration over unordered collections (maps/hash sets) is not allowed for gameplay outcomes unless keys are sorted first.
6. Server is always authoritative; client state is advisory only.

## 4. Simulation Time Contract

### 4.1 Tick Rate
- Authoritative server tick rate: `30 ticks/sec`
- Fixed timestep: `33_333_333 ns` per tick
- Tick ID type: `uint64`
- First simulation tick in a match: `1`

Rationale:
- Matches architecture note for mobile-friendly fixed loop.
- Keeps CPU/network cost reasonable for 5-12 player sessions.

### 4.2 Time Source
- Use monotonic server clock only to drive the tick loop scheduler.
- World time inside gamecore is derived from tick count, not wall clock.
- Canonical elapsed match time = `tick_id * fixed_dt`.

### 4.3 Tick Overrun Policy
- If simulation falls behind real time, process catch-up ticks with the same fixed `dt`.
- Maximum catch-up ticks per loop iteration: `3`
- If still behind after catch-up, continue next loop; never change `dt` to compensate.

## 5. Input Contract and Ordering

### 5.1 Minimum Command Metadata
Each accepted command must carry:
- `player_id`
- `client_seq` (monotonic per connection)
- `cmd_type`
- `payload`
- `target_tick` (optional hint from client)
- `ingress_seq` (assigned by server when accepted)

`ingress_seq` is a strictly increasing `uint64` per match and is the final tie-breaker for deterministic ordering.

### 5.2 Validation Rules
- Reject command if player is not in match / disconnected / dead (where action is invalid).
- Reject duplicate `client_seq` for the same player.
- Reject malformed payloads.
- Per-player rate cap: max `8` commands accepted per tick window.

### 5.3 Scheduling Rules
At server tick `N`, accepted commands are scheduled as follows:
- Default scheduled tick: `N + 1`
- If `target_tick` is present, clamp to `[N + 1, N + 2]`
- Commands targeting `< N` are late:
  - discrete actions (shoot, card use, ability use) are dropped
  - continuous intent updates (movement/aim intent) are applied at `N + 1`

### 5.4 Deterministic Apply Order Inside a Tick
For all commands scheduled for tick `T`, apply in this exact order:
1. ascending `ingress_seq`
2. if equal (defensive): ascending `player_id`
3. if equal (defensive): ascending `client_seq`

### 5.5 Idempotency
- Commands identified by `(player_id, client_seq)` must be processed at most once.
- Replays must preserve original `ingress_seq` to reproduce outcomes exactly.

## 6. Authoritative Tick Pipeline

For each tick `T`, execute exactly this sequence:
1. Collect all commands scheduled for `T`.
2. Apply input intents/actions in deterministic order.
3. Update movement and collision.
4. Resolve interactions (doors, cells, access checks, pickups).
5. Resolve abilities/cards and cooldown state transitions.
6. Resolve combat damage, knockback, stuns, and penalties.
7. Update power/alarm/NPC guard systems.
8. Update phase timers and day/night transitions.
9. Evaluate global and role-specific win conditions.
10. Build state delta and emit snapshot tagged with `tick_id = T`.

No subsystem may mutate world state outside this order.

## 7. RNG Policy

### 7.1 Seed Source
- On match start, server generates `match_seed` from cryptographic randomness.
- `match_seed` is stored in match metadata and replay headers.

### 7.2 PRNG Algorithm
- Use a dedicated deterministic PRNG implementation in gamecore (for example: PCG32 or xoroshiro128+).
- Do not use global `math/rand` for authoritative gameplay outcomes.

### 7.3 Stream Derivation
Derive independent deterministic streams from `match_seed`, e.g.:
- `roles_stream`
- `loot_stream`
- `ai_stream`
- `events_stream`

Derivation must be stable and documented in code (`seed = hash64(match_seed, stream_name)`).

### 7.4 Deterministic Selection
- When choosing random element from dynamic collections, sort IDs first, then sample index.
- Random call count/order must be stable for a given input sequence.

## 8. Snapshot and Client Reconciliation Contract

### 8.1 Server Snapshot Rules
- Send full snapshot on join.
- Send delta snapshot every server tick.
- Every snapshot includes:
  - `tick_id`
  - changed entities/components
  - per-player `last_processed_client_seq`

### 8.2 Client Prediction Rules
- Client may locally predict only local movement/aim responsiveness.
- Client must never predict authority-only outcomes (damage, arrests, role reveals, win state).

### 8.3 Reconciliation Rules
On snapshot `S`:
1. Apply authoritative state at `tick_id = S.tick_id`.
2. Drop local pending inputs where `client_seq <= last_processed_client_seq`.
3. Reapply remaining pending local movement inputs in original sequence order.
4. Render correction:
   - if position error <= `0.20` tile: smooth over up to `100 ms`
   - if position error > `0.20` tile: snap immediately

### 8.4 Interpolation for Other Players
- Render remote entities using interpolation between latest two snapshots.
- Suggested render delay buffer: `100 ms` (tunable later).

## 9. Determinism Test Contract (for T-005 alignment)

Minimum required tests:
1. Same `match_seed` + same ordered input log => identical final `GameState` hash.
2. Same input set with different network arrival batching but same final `ingress_seq` ordering => identical result.
3. Duplicate commands with same `(player_id, client_seq)` do not change outcome.
4. RNG stream calls are reproducible across runs.
5. Reconciliation test: server acked sequence removes matching local pending inputs only.

## 10. Acceptance Criteria for T-002

`T-002` is complete when:
1. Fixed tick rate and timestep are documented.
2. Input validation, scheduling, late-input handling, and deterministic ordering are documented.
3. RNG seed source, algorithm policy, and stream-derivation policy are documented.
4. Snapshot/reconciliation rules are documented with ack semantics.
5. This contract is approved as baseline for `T-003` and `T-004`.

## 11. Implementation Notes for Next Tickets

- `T-003`: directory/package scaffolding should reserve modules for tick loop, command queue, RNG service, and snapshot builder.
- `T-004`: shared protocol must include `tick_id`, `client_seq`, `last_processed_client_seq`, and delta/full snapshot message variants.
