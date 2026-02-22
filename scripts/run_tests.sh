#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COVERAGE_OUT="${PERFOLIZER_COVERAGE_OUT:-$ROOT_DIR/dist/coverage.out}"

mkdir -p "$ROOT_DIR/.cache/go-build" "$(dirname "$COVERAGE_OUT")"
export GOCACHE="$ROOT_DIR/.cache/go-build"

echo "Running test suite..."
go test ./... -covermode=atomic -coverpkg=./... -coverprofile="$COVERAGE_OUT"

echo
echo "Coverage summary:"
go tool cover -func="$COVERAGE_OUT"
echo
echo "Coverage profile: $COVERAGE_OUT"
