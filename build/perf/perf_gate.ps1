param(
    [string]$Output = "build/perf/bench.out"
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required. Install Go and ensure it is in PATH."
}

$outputDir = Split-Path -Parent $Output
if ($outputDir -and -not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir | Out-Null
}

$benchOutput = go test -run '^$' -bench 'BenchmarkSnapshotStoreApplyDelta|BenchmarkMatchmakingRegionAllocation' -benchmem ./internal/client/netclient ./internal/server/matchmaking
if ($LASTEXITCODE -ne 0) {
    throw "Benchmark command failed."
}
$benchOutput | Set-Content -Path $Output -Encoding UTF8

go run ./cmd/tools/perfgate -in $Output -budget SnapshotStoreApplyDelta=500000 -budget MatchmakingRegionAllocation=2000000
if ($LASTEXITCODE -ne 0) {
    throw "Performance budget check failed."
}
Write-Host "Performance gate passed."
