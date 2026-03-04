# Store Readiness Checklist

Use this checklist before pushing a release candidate to distribution channels.

## Identity and Versioning

- [ ] Release version is locked and matches `build/release/out/<version>/manifest.json`.
- [ ] Artifact hashes are generated and archived with the release ticket.
- [ ] Build provenance notes include build machine, timestamp, and toolchain version.

## Security and Privacy

- [ ] `PRISON_AUTH_REQUIRED=true` and production auth secret rotation process is verified.
- [ ] Session tokens are short-lived and replay-safe for gameplay/admin scopes.
- [ ] Privacy policy and support contact links are present in store metadata.

## Crash and Supportability

- [ ] Crash reporting output path is configured (`PRISON_CRASH_REPORT_DIR`) in runtime environments.
- [ ] Crash report retention + redaction policy is documented for operators.
- [ ] Post-crash runbook exists for triage and customer communication.

## Gameplay and UX

- [ ] Tutorial/codex pages are accessible and readable on desktop/mobile aspect ratios.
- [ ] Spectator mode follows role-safe visibility and camera switching controls.
- [ ] Day/night pacing and cycle cap align with current balance targets (5m/2m, 6 cycles).

## Platform Packaging

- [ ] Desktop binaries launch without external dependency assumptions.
- [ ] Android APK path builds and signs using the release key material.
- [ ] iOS app bundle path builds and signs with the intended provisioning profile.

## Operational Gates

- [ ] `go test ./...` passes on release branch commit.
- [ ] Performance gate passes (`build/perf/perf_gate.ps1`).
- [ ] Release smoke gate passes (`build/release/run_rc_gate.ps1`).
