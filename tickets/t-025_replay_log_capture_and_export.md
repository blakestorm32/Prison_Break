# T-025 Replay Log Capture and Export

Status: Implemented (code + tests)  
Related roadmap ticket: `T-025` in `roadmap_tickets.md`

## Goal

Capture authoritative accepted input history and expose it for replay/debug pipelines.

## Implemented Files

- `internal/server/game/manager.go`
- `internal/server/game/input_queue.go`
- `internal/server/game/replay.go`
- `internal/server/game/replay_test.go`
- `internal/server/networking/server.go`
- `internal/server/networking/server_test.go`
- `internal/shared/protocol/messages.go`
- `internal/shared/protocol/messages_fuzz_test.go`
- `internal/server/networking/envelope_fuzz_test.go`
- `.github/workflows/ci.yml`
- `build/README.md`
- `roadmap_tickets.md`

## Core Behavior

1. Replay recording in authoritative path:
- Every accepted command in `SubmitInput` now appends a replay record with:
  - accepted/scheduled tick
  - ingress sequence
  - UTC acceptance time
  - full normalized `InputCommand`

2. Replay manager API:
- Added `Manager.ReplayLog(matchID)` returning immutable replay data:
  - match metadata (status, tick rate, timing fields)
  - ordered replay entries

3. Replay networking protocol:
- Added client->server request message: `request_replay`
- Added server->client response message: `replay_log`
- Added server handler enforcing match-binding auth semantics before replay export.

## Test Coverage Added

- `internal/server/game/replay_test.go`
  - captures accepted inputs with correct tick/ingress fields
  - duplicate command rejection does not pollute replay log
  - deep-copy safety of replay payloads
  - match-not-found behavior

- `internal/server/networking/server_test.go`
  - replay request unauthorized before join
  - replay request returns accepted entries after gameplay command submission

- Fuzz:
  - `FuzzDecodeReplayLogRequestDoesNotPanic`
  - networking envelope fuzz now includes `request_replay`

## Verification

Executed:
- `C:\Program Files\Go\bin\gofmt.exe -w <all .go files>`
- `C:\Program Files\Go\bin\go.exe test ./...`

Result:
- all tests passing.
