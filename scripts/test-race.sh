#!/usr/bin/env bash
# Race detector test runner — detects data races using Go's race detector.
# Requires CGO_ENABLED=1 (the race detector needs a C compiler).
# On Linux/macOS this works natively with gcc/clang.
#
# Usage: ./scripts/test-race.sh [package_filter...]
# Default: tests all packages.
# Example: ./scripts/test-race.sh ./controller/... ./model/...
#
# Note: The race detector does not work with CGO_ENABLED=0.
# This script sets CGO_ENABLED=1. The production build remains CGO_ENABLED=0.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [ $# -gt 0 ]; then
    PKGS="$@"
else
    PKGS="./..."
fi

echo "=== Race Detector Test ==="
echo "Packages: $PKGS"
echo "CGO_ENABLED=1 (required for -race)"
echo ""

export CGO_ENABLED=1

go test -race -count=1 $PKGS 2>&1 | while IFS= read -r line; do
    if echo "$line" | grep -q "DATA RACE"; then
        echo "$line" >&2
        FAILED=1
    elif echo "$line" | grep -q "^--- FAIL"; then
        echo "$line" >&2
        FAILED=1
    elif echo "$line" | grep -q "^ok"; then
        echo "$line"
    else
        echo "$line"
    fi
done

EXIT_CODE=$?

export CGO_ENABLED=0

if [ $EXIT_CODE -ne 0 ]; then
    echo ""
    echo "=== RACE TEST FAILED ==="
    exit 1
else
    echo ""
    echo "=== Race test passed ==="
fi
