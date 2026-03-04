param(
    [string]$Output = "build/android/prison-break.apk",
    [string]$Target = "./cmd/client",
    [int]$AndroidAPI = 24
)

$ErrorActionPreference = "Stop"

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    throw "Go is required. Install Go and ensure it is in PATH."
}

if (-not (Get-Command gomobile -ErrorAction SilentlyContinue)) {
    go install github.com/ebitengine/gomobile/cmd/gomobile@latest
}

gomobile init

$outputDir = Split-Path -Parent $Output
if ($outputDir -and -not (Test-Path $outputDir)) {
    New-Item -ItemType Directory -Path $outputDir | Out-Null
}

gomobile build -target=android -androidapi $AndroidAPI -o $Output $Target
Write-Host "Android build generated at $Output"
