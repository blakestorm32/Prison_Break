param(
    [int]$SmokeMatches = 4,
    [int]$SmokePlayers = 4,
    [int]$SmokeTimeoutSeconds = 8
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required. Install Go and ensure it is in PATH."
}

Write-Host "Running full test suite..."
go test ./...
if ($LASTEXITCODE -ne 0) {
    throw "Full test suite failed."
}

Write-Host "Running performance gate..."
& ./build/perf/perf_gate.ps1
if ($LASTEXITCODE -ne 0) {
    throw "Performance gate failed."
}

Write-Host "Running smoke runner..."
$timeoutArg = "${SmokeTimeoutSeconds}s"
go run ./cmd/tools/smoke -matches $SmokeMatches -players $SmokePlayers -timeout $timeoutArg
if ($LASTEXITCODE -ne 0) {
    throw "Smoke runner failed."
}

Write-Host "RC gate passed."
