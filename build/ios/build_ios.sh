#!/usr/bin/env bash
set -euo pipefail

OUTPUT="${1:-build/ios/PrisonBreak.app}"
TARGET="${2:-./cmd/client}"

if ! command -v go >/dev/null 2>&1; then
  echo "Go is required. Install Go and ensure it is in PATH." >&2
  exit 1
fi

if ! command -v gomobile >/dev/null 2>&1; then
  go install github.com/ebitengine/gomobile/cmd/gomobile@latest
fi

gomobile init
mkdir -p "$(dirname "${OUTPUT}")"
gomobile build -target=ios -o "${OUTPUT}" "${TARGET}"
echo "iOS build generated at ${OUTPUT}"
