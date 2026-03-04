# Build and Release Paths

## QA Gate

Run the default gate locally before a release cut:

```powershell
& "C:\Program Files\Go\bin\go.exe" test ./...
& "C:\Program Files\Go\bin\go.exe" test -count=1 -run Determin ./internal/gamecore/determinism ./internal/server/game ./internal/client/prediction
& "C:\Program Files\Go\bin\go.exe" test -run '^$' -fuzz '^FuzzDecodeJoinGameRequestDoesNotPanic$' -fuzztime=5s ./internal/shared/protocol
& "C:\Program Files\Go\bin\go.exe" test -run '^$' -fuzz '^FuzzDecodePlayerInputMessageDoesNotPanic$' -fuzztime=5s ./internal/shared/protocol
& "C:\Program Files\Go\bin\go.exe" test -run '^$' -fuzz '^FuzzDecodeReplayLogRequestDoesNotPanic$' -fuzztime=5s ./internal/shared/protocol
& "C:\Program Files\Go\bin\go.exe" test -run '^$' -fuzz '^FuzzEnvelopeDecodeAndTypedPayloadUnmarshalDoesNotPanic$' -fuzztime=5s ./internal/server/networking
```

## Docker (Dedicated Server)

Server container build path:

- Dockerfile: `build/docker/Dockerfile.server`
- Compose path: `build/docker/docker-compose.yml`

Build and run:

```bash
docker build -f build/docker/Dockerfile.server -t prison-break/server:local .
docker compose -f build/docker/docker-compose.yml up --build
```

## Android (Ebiten Client)

Build script path:

- `build/android/build_android.ps1`

Usage:

```powershell
./build/android/build_android.ps1
```

Output:

- `build/android/prison-break.apk`

## iOS (Ebiten Client)

Build script path:

- `build/ios/build_ios.sh`

Usage:

```bash
bash build/ios/build_ios.sh
```

Output:

- `build/ios/PrisonBreak.app`

## Performance Gate

Benchmark and budget gate script path:

- `build/perf/perf_gate.ps1`

Usage:

```powershell
./build/perf/perf_gate.ps1
```

This runs focused client/server performance benchmarks and fails if tracked `ns/op` budgets regress.

## Local Scale Simulation

Load simulation tool path:

- `cmd/tools/loadtest/main.go`

Usage:

```powershell
go run ./cmd/tools/loadtest -matches 20 -players 8 -seconds 15 -tickrate 30
```

## Release Packaging

Release build script path:

- `build/release/build_release.ps1`

Usage:

```powershell
./build/release/build_release.ps1 -Version "2026.03.04-rc1"
```

Store/compliance readiness checklist:

- `build/release/store_readiness_checklist.md`

RC gate and launch checklist:

- Gate script: `build/release/run_rc_gate.ps1`
- Checklist: `build/release/launch_checklist.md`
