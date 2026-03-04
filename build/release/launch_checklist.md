# Release Candidate Launch Checklist

This checklist is the final go/no-go gate for `T-050`.

## Engineering Gates

- [ ] Full suite passes: `go test ./...`
- [ ] Performance gate passes: `build/perf/perf_gate.ps1`
- [ ] Smoke gate passes: `build/release/run_rc_gate.ps1`
- [ ] No unresolved `P0/P1` defects in active release scope

## Gameplay Readiness

- [ ] Client supports full player flow (menu -> lobby -> match -> game over)
- [ ] Spectator follow-cam and role-safe overlays validated in live session
- [ ] Tutorial/codex is accessible and covers controls/roles/cards/abilities/win conditions
- [ ] Balance telemetry endpoint (`/admin/balance`) reviewed for latest playtest sample

## Operations Readiness

- [ ] Admin endpoints (`overview`, `matches`, `connections`, `queue`, `balance`) reachable with admin scope
- [ ] Crash report directory configured and writable in runtime environments
- [ ] Persistence path configured and backup/restore tested
- [ ] Auth secret rotation and token issuance runbook validated

## Packaging Readiness

- [ ] Release artifacts built with `build/release/build_release.ps1`
- [ ] Artifact manifest and SHA256 hashes archived
- [ ] Store readiness checklist completed (`build/release/store_readiness_checklist.md`)

## Signoff

- [ ] Engineering Lead signoff
- [ ] QA Lead signoff
- [ ] Operations signoff
- [ ] Product go/no-go decision recorded
