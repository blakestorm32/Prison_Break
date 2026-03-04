param(
    [string]$Version = "",
    [string]$OutputRoot = "build/release/out",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required. Install Go and ensure it is in PATH."
}

if ([string]::IsNullOrWhiteSpace($Version)) {
    $Version = (Get-Date -Format "yyyy.MM.dd-HHmm")
}

$releaseDir = Join-Path $OutputRoot $Version
if (-not (Test-Path $releaseDir)) {
    New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
}

if (-not $SkipTests) {
    go test ./...
}

$buildFlags = "-trimpath -ldflags=""-s -w -X main.buildVersion=$Version"""

$serverWin = Join-Path $releaseDir "prison-server-windows-amd64.exe"
$serverLinux = Join-Path $releaseDir "prison-server-linux-amd64"
$clientWin = Join-Path $releaseDir "prison-client-windows-amd64.exe"

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
go build $buildFlags -o $serverWin ./cmd/server
go build $buildFlags -o $clientWin ./cmd/client

$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build $buildFlags -o $serverLinux ./cmd/server

Remove-Item Env:\GOOS -ErrorAction SilentlyContinue
Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
Remove-Item Env:\CGO_ENABLED -ErrorAction SilentlyContinue

$artifacts = Get-ChildItem -Path $releaseDir -File | Sort-Object Name
$hashes = @()
foreach ($artifact in $artifacts) {
    $hash = Get-FileHash -Algorithm SHA256 -Path $artifact.FullName
    $hashes += [pscustomobject]@{
        file = $artifact.Name
        sha256 = $hash.Hash.ToLowerInvariant()
        bytes = $artifact.Length
    }
}

$manifest = [pscustomobject]@{
    version = $Version
    built_at_utc = (Get-Date).ToUniversalTime().ToString("o")
    artifacts = $hashes
}
$manifestPath = Join-Path $releaseDir "manifest.json"
$manifest | ConvertTo-Json -Depth 5 | Set-Content -Path $manifestPath -Encoding UTF8

Write-Host "Release artifacts generated under $releaseDir"
