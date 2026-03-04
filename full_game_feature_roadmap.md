# Full Game Feature Roadmap

Scope: deliver a fully playable multiplayer game with complete role/card/ability content, production-ready client UX, and release operations.

Source specs:
- `game_idea.txt`
- `repo_layout_and_architecture`
- `roadmap_tickets.md` (detailed sequential tickets)

## Current Baseline

- Core authoritative simulation and major game systems exist.
- Client is primarily a shell/prototype and still requires full live gameplay productization.
- Detailed sequential implementation tickets now run through `T-050`.

## Phase Map

## Phase A: From Prototype to Playable Match Loop

Tickets:
- `T-026` spectator read-only flow
- `T-027` real client networking (remove demo seed dependence)
- `T-028` live command dispatch
- `T-029` snapshot-ack/session reconciliation

Exit criteria:
- A user can launch client, join a real lobby/match, move/act against authoritative server, and complete a basic end-to-end round loop.

## Phase B: Core Player Experience and UI Completeness

Tickets:
- `T-030` pre-match menu/lobby UX
- `T-031` in-match HUD/action surfaces
- `T-032` inventory/cards/abilities UI
- `T-033` black-market UX
- `T-034` escape-route UX
- `T-035` role/objective hidden-info presentation

Exit criteria:
- Players can discover and execute game actions using in-client UX, without dev tooling or implicit knowledge.

## Phase C: Full Feature Content Completion

Tickets:
- `T-036` full ability matrix
- `T-037` full card matrix
- `T-038` NPC bribe + economy interactions
- `T-039` combat/audio/visual feedback pass

Exit criteria:
- All features specified in `game_idea.txt` are implemented and visible in normal gameplay.

## Phase D: Service Reliability, Security, and Operations

Tickets:
- `T-040` reconnect/resume
- `T-041` persistence/accounts/stats
- `T-042` auth + protocol hardening
- `T-043` admin/moderation/observability
- `T-044` queue + region-aware matchmaking
- `T-045` spectator product surfaces

Exit criteria:
- Multiplayer sessions are resilient, secure, and operable at scale with live-service controls.

## Phase E: Shipping and Launch

Tickets:
- `T-046` tutorial/onboarding/rules codex
- `T-047` balance + pacing tuning from playtests
- `T-048` perf/scale hardening
- `T-049` packaging/store readiness
- `T-050` RC and launch checklist closure

Exit criteria:
- Production release candidate is stable, balanced, and deployable to target platforms.

## Critical Path Milestones

1. Networked playability milestone: `T-029`
2. Player-facing UX completeness milestone: `T-035`
3. Feature completeness milestone: `T-039`
4. Live-service readiness milestone: `T-045`
5. Launch readiness milestone: `T-050`

## Recommendation for Execution Cadence

1. Keep strict sequential dependency by ticket.
2. Require green determinism + integration + fuzz smoke gates at every ticket.
3. Bundle playtests at phase boundaries (`T-029`, `T-035`, `T-039`, `T-045`, `T-050`) before advancing.
