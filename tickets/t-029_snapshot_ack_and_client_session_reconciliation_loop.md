# T-029 Snapshot Ack And Client Session Reconciliation Loop

Status: Implemented (code + tests)  
Related roadmap ticket: `T-029` in `roadmap_tickets.md`

## Goal

Implement full client-side snapshot acknowledgment and reconciliation flow so that:
- client sends `ack_snapshot` for authoritative snapshots
- client tracks server ack progress (`player_acks`)
- prediction pending commands stay bounded and stable during long sessions

## Implemented Files

- `internal/client/netclient/session.go`
- `internal/client/netclient/session_test.go`
- `internal/client/netclient/snapshot_store.go`
- `internal/client/netclient/snapshot_store_test.go`
- `internal/client/render/shell.go`
- `internal/client/render/shell_prediction_test.go`
- `internal/client/prediction/engine.go`
- `internal/client/prediction/engine_test.go`
- `roadmap_tickets.md`

## Core Behavior

1. Session-level snapshot ack send path:
- `netclient.Session` now sends `ack_snapshot` envelopes for applied authoritative snapshots.
- Ack payload includes:
  - `client_tick_id`: latest applied snapshot tick
  - `last_processed_client_seq`: latest server-acknowledged local client sequence from snapshot `player_acks`
- Spectator sessions remain read-only and do not send snapshot acks.

2. Session-level server-ack tracking:
- Session tracks:
  - last snapshot tick acknowledged by client
  - last server-acked local client sequence
  - tick where latest server ack was observed
- Exposed via getters for diagnostics and future HUD/telemetry hooks.

3. Store metadata for reconciliation:
- `SnapshotStore` now persists latest snapshot metadata (`tick_id` and `player_acks`) alongside authoritative state.
- Added `LatestSnapshotMeta()` with defensive deep-copy behavior.

4. Runtime reconciliation loop wiring:
- `render.Shell` now reconciles prediction directly from live store snapshots during `Update`/render resolution.
- This closes the runtime gap where prediction previously only reconciled in test helper paths.

5. Pending queue stability and UX protection:
- Prediction config now includes `PendingLimit` (default `256`).
- Pending local prediction commands are trimmed to cap memory growth and long-session drift pressure.
- Reconciliation still drops server-acked commands first; cap acts as hard safety bound.

## Test Coverage Added

`internal/client/netclient/session_test.go`:
- `TestSessionReadLoopSendsSnapshotAckAndTracksServerAck`
- `TestSessionReadLoopDoesNotSendSnapshotAckForSpectator`
- Existing config normalization test extended for new defaults.

`internal/client/netclient/snapshot_store_test.go`:
- validates snapshot meta tracking and immutable deep-copy behavior for `player_acks`.

`internal/client/render/shell_prediction_test.go`:
- `TestShellUpdateReconcilesFromStoreSnapshotsAndDropsAckedPendingCommands`
- verifies runtime store-driven reconciliation path drops acked pending commands.

`internal/client/prediction/engine_test.go`:
- `TestRecordLocalCommandsCapsPendingQueueToConfiguredLimit`
- verifies pending queue cap behavior and newest-command retention policy.

## Verification

Executed:
- `gofmt -w internal/client/netclient/session.go internal/client/netclient/session_test.go internal/client/netclient/snapshot_store.go internal/client/netclient/snapshot_store_test.go internal/client/prediction/engine.go internal/client/prediction/engine_test.go internal/client/render/shell.go internal/client/render/shell_prediction_test.go`
- `go test ./internal/client/netclient ./internal/client/prediction ./internal/client/render`
- `go test ./...`

Result:
- all tests passing.
