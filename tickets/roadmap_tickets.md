# Prison Break Build Roadmap (Sequential Tickets)

Source context: `game_idea.txt` and `repo_layout_and_architecture`.

Current status:
- Complete: `T-001`, `T-002`, `T-003`, `T-004`, `T-005`, `T-006`, `T-007`, `T-008`, `T-009`, `T-010`, `T-011`, `T-012`, `T-013`, `T-014`, `T-015`, `T-016`, `T-017`, `T-018`, `T-019`, `T-020`, `T-021`, `T-022`, `T-023`, `T-024`, `T-025`, `T-026`, `T-027`, `T-028`, `T-029`, `T-030`, `T-031`, `T-032`, `T-033`, `T-034`, `T-035`, `T-036`, `T-037`, `T-038`, `T-039`, `T-040`, `T-041`, `T-042`, `T-043`, `T-044`, `T-045`, `T-046`, `T-047`, `T-048`, `T-049`, `T-050`
- Next up: `T-051` onward (post-playtest UX/gameplay pass)

## Ticket Order

### T-001 Define MVP scope and rule lock
- Depends on: none
- Done when: one agreed MVP ruleset exists (roles, win conditions, combat, one map, 5-12 players).

### T-002 Create deterministic server-authoritative tick spec
- Depends on: `T-001`
- Done when: fixed tick rate, input ordering, RNG seed policy, and reconciliation rules are documented.

### T-003 Scaffold monorepo/package layout
- Depends on: `T-002`
- Done when: `cmd/`, `internal/`, `assets/`, `build/` structure exists with empty package boundaries.

### T-004 Define shared protocol and state schemas
- Depends on: `T-003`
- Done when: serializable `GameState`, `Snapshot`, `InputCommand`, and message envelopes are finalized.

### T-005 Add determinism test harness
- Depends on: `T-004`
- Done when: same seed plus same inputs always produces identical final state in tests.

### T-006 Implement server match/session lifecycle shell
- Depends on: `T-005`
- Done when: create/join/start/end match lifecycle works without full gameplay logic.

### T-007 Implement WebSocket transport and connection lifecycle
- Depends on: `T-006`
- Done when: clients can connect/disconnect, send commands, and receive server events.

### T-008 Build input queue and command validation pipeline
- Depends on: `T-007`
- Done when: invalid/late/spam inputs are rejected and valid inputs are tick-buffered.

### T-009 Implement snapshot pipeline (full on join, delta per tick)
- Depends on: `T-008`
- Done when: server emits tick-tagged snapshots and clients can consume state updates.

### T-010 Implement map model (tiles, rooms, corridors, restricted zones)
- Depends on: `T-009`
- Done when: prison layout supports room access checks and path connectivity.

### T-011 Implement door/cell/access-control rules
- Depends on: `T-010`
- Done when: cell ownership, authority privileges, Warden HQ restriction, camera room restriction, and black market restriction are enforced.

### T-012 Implement movement/collision/physics baseline
- Depends on: `T-011`
- Done when: players move on the tile map with blocking, knockback, and stun-compatible motion control.

### T-013 Implement phase manager (DAY/NIGHT plus cycle counter)
- Depends on: `T-012`
- Done when: timed transitions and per-phase reset hooks execute correctly up to 10 cycles.

### T-014 Implement role assignment engine by player count
- Depends on: `T-013`
- Done when: valid role mixes are assigned for 6-12 players with hidden-role visibility rules.

### T-015 Implement win-condition engine (global plus role-specific)
- Depends on: `T-014`
- Done when: all documented game-ending checks and role outcomes resolve deterministically.

### T-016 Implement inventory/items/contraband tagging
- Depends on: `T-015`
- Done when: item ownership, pickups, transfers, contraband checks, and capacity constraints work.

### T-017 Implement combat system and authority penalties
- Depends on: `T-016`
- Done when: hearts, shiv/gun/golden bullet/baton effects, stun, and unjust-shooting lock-in penalties are enforced.

### T-018 Implement prison systems: power, alarm, and NPC guard reaction
- Depends on: `T-017`
- Done when: power state affects cameras/ammo/doors; alarm and restricted-area guard shooting logic works.

### T-019 Implement abilities and cards framework with initial content
- Depends on: `T-018`
- Done when: cooldowns, once-per-day checks, card slot limit (3), and listed card/ability effects are functional.

### T-020 Implement escape-route systems and nightly black market flow
- Depends on: `T-019`
- Done when: all escape routes validate required items/states; Gang Leader can set nightly market; succession works.

### T-021 Build Ebiten client shell (render plus camera plus HUD skeleton)
- Depends on: `T-020`
- Done when: map/entities/game phase/health/ammo/cooldowns render from authoritative snapshots.

### T-022 Build client input layer (desktop plus mobile controls)
- Depends on: `T-021`
- Done when: keyboard and touch joystick/buttons generate valid network commands.

### T-023 Implement client interpolation and reconciliation
- Depends on: `T-022`
- Done when: movement appears smooth under latency while server authority remains final.

### T-024 Implement matchmaking/lobby, QA hardening, and release pipeline
- Depends on: `T-023`
- Done when: lobby flow works, integration/fuzz/determinism tests pass, and Docker/Android/iOS build paths are defined.

### T-025 Implement replay-log capture and export flow
- Depends on: `T-024`
- Done when: accepted inputs are recorded with tick metadata and can be retrieved via manager API and WebSocket.

### T-026 Implement spectator read-only client join flow
- Depends on: `T-025`
- Done when: non-player spectators can join running matches and receive snapshots without gameplay command authority.

### T-027 Wire real client networking session (no demo seed state)
- Depends on: `T-026`
- Done when: `cmd/client` connects to `/ws`, sends `join_game`, consumes `snapshot/game_start`, and no longer depends on hardcoded demo state.

### T-028 Connect client input pipeline to live server command dispatch
- Depends on: `T-027`
- Done when: `move/aim/interact/fire/reload` commands generated by shell are serialized and transmitted to server in real matches.

### T-029 Add snapshot-ack and client session reconciliation loop
- Depends on: `T-028`
- Done when: client sends `ack_snapshot`, tracks server acks, and prediction pending-queue remains bounded/stable during long sessions.

### T-030 Build pre-match UI flow (main menu, create/join lobby, ready/start)
- Depends on: `T-029`
- Done when: players can navigate from launch -> lobby -> match start without terminal/manual API calls.

### T-031 Build in-match gameplay HUD and action surfaces
- Depends on: `T-030`
- Done when: HUD shows role-safe info, health/ammo/effects/cooldowns/objective text, and action affordances are visible on desktop + mobile.

### T-032 Implement inventory, card-slot, and ability panels
- Depends on: `T-031`
- Done when: player can inspect inventory/cards/abilities and trigger valid use actions through UI (not only raw keybinds).

### T-033 Implement black-market gameplay UI and purchasing flow
- Depends on: `T-032`
- Done when: eligible players can see nightly market location, browse items, transact, and receive authoritative inventory updates.

### T-034 Implement escape-route interaction UX and progress feedback
- Depends on: `T-033`
- Done when: each escape route requirement/state is visible in UI and interactions produce clear success/fail/server-reason feedback.

### T-035 Complete full role objective and hidden-info presentation layer
- Depends on: `T-034`
- Done when: every role/alignment variant gets correct private/public UI visibility and objective progress indicators.

### T-036 Complete remaining ability content matrix and validations
- Depends on: `T-035`
- Done when: all listed faction abilities (`camera_man/search/detainer/tracker/pick_pocket/hacker/disguise/locksmith/chameleon`) are fully implemented, tested, and networked.

### T-037 Complete remaining card content matrix and effect lifecycle
- Depends on: `T-036`
- Done when: all listed cards resolve correctly (`morphine/bullet/money/speed/armor_plate/lock_snap/item_steal/item_grab/scrap_bundle/door_stop/get_out_of_jail_free`) including edge-case timing.

### T-038 Implement NPC prisoner bribe and economy interactions
- Depends on: `T-037`
- Done when: money card and black-market economy can affect NPC prisoner behavior in deterministic, tested rules.

### T-039 Add combat/audio/visual feedback pass
- Depends on: `T-038`
- Done when: hit events, stun, alarm, door actions, purchases, and escapes provide readable VFX/SFX/UI feedback.

### T-040 Implement reconnect/resume session flow
- Depends on: `T-039`
- Done when: disconnected players can rebind to active match identity and recover authoritative state without corrupting match logic.

### T-041 Add persistence layer for accounts, stats, and match history
- Depends on: `T-040`
- Done when: player profile/stats/history are stored and queried through `internal/server/persistence` with migration/versioned schema support.

### T-042 Add server authn/authz and protocol hardening
- Depends on: `T-041`
- Done when: authenticated identity, signed session tokens, and command authorization checks protect all gameplay and admin endpoints.

### T-043 Add moderation/admin observability and ops endpoints
- Depends on: `T-042`
- Done when: match inspection, replay export tooling, lifecycle metrics, and abuse controls are available for operations.

### T-044 Expand matchmaking to queue-based and region-aware allocation
- Depends on: `T-043`
- Done when: players can queue and be allocated to matches by latency/region constraints with deterministic server assignment behavior.

### T-045 Add full spectator product surfaces (follow cams + role-safe overlays)
- Depends on: `T-044`
- Done when: spectators can join by code/slot, follow players, and view only allowed information per game secrecy rules.

### T-046 Add tutorial/onboarding and rules codex UI
- Depends on: `T-045`
- Done when: first-time players can learn controls/roles/cards/abilities in-client without external docs.

### T-047 Balance pass and simulation tuning from structured playtests
- Depends on: `T-046`
- Done when: combat values, day/night pacing (5m/2m), six-cycle balance, item economy, and win-rate targets are tuned from telemetry/playtest data.

### T-048 Performance and scale hardening (server + client)
- Depends on: `T-047`
- Done when: target concurrent matches and low-end mobile FPS/network budgets pass load/perf thresholds.

### T-049 Platform release hardening (desktop/mobile packaging + store readiness)
- Depends on: `T-048`
- Done when: reproducible signed builds, crash reporting, compliance checklists, and deployment scripts are finalized.

### T-050 Release candidate and launch checklist closure
- Depends on: `T-049`
- Done when: all blockers are closed, smoke suite passes in production-like environment, and go/no-go signoff is complete.

## Post-Playtest UX and Gameplay Tickets

### T-051 Implement public/private lobby creation and join-by-code flow
- Depends on: `T-050`
- Done when: players can choose public matchmaking or private lobby creation; private host sees a shareable lobby code; private join requires code entry.

### T-052 Implement contextual HUD actions and role readout
- Depends on: `T-051`
- Done when: compact HUD always shows faction + exact role + phase (+ping when available), and interact/shoot affordances are visibly lit when available and dulled when unavailable.

### T-053 Improve door readability with explicit open/closed presentation
- Depends on: `T-052`
- Done when: doors are visually obvious on the map and clearly communicate open vs closed state at gameplay glance.

### T-054 Implement role-based spawn placement pass
- Depends on: `T-053`
- Done when: prisoners start in individual cells, warden starts in Warden HQ, and deputies/guards spawn in designated guard-side room(s).

### T-055 Rebalance movement speed and camera-map feel
- Depends on: `T-054`
- Done when: baseline movement is slower/controllable, camera keeps player-centered readability, and map presentation no longer feels like a small static board.
