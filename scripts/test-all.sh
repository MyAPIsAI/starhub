#!/usr/bin/env bash
# Unified test runner for StarHub.
# Runs L0 (static checks), L1 (unit tests), and L3 (e2e) in sequence.
# Any test failure causes non-zero exit.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAILED=0

run_step() {
    local name="$1"
    shift
    echo ""
    echo "=== $name ==="
    if "$@"; then
        echo "PASS: $name"
    else
        echo "FAIL: $name (exit code $?)"
        FAILED=1
    fi
}

# L0: Static Checks
echo "=== L0: Static Checks ==="
cd "$ROOT"
go build ./... && go vet ./... || { echo "FAIL: L0 Go"; FAILED=1; }
cd "$ROOT/web"
npm run build > /dev/null 2>&1 || { echo "FAIL: L0 Frontend build"; FAILED=1; }
[ "$FAILED" -eq 0 ] && echo "PASS: L0 static checks"

# L1: Go Unit Tests (go test ./... includes common, e2e, all packages)
run_step "L1: Go Unit Tests" bash -c 'cd "$ROOT" && go test ./... -count=1'

# L1: Frontend Unit Tests
cd "$ROOT/web"
npm run test:unit > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "PASS: L1: Frontend Unit Tests"
else
    echo "FAIL: L1: Frontend Unit Tests"
    FAILED=1
fi

# L3: E2E is already included in go test ./... above, but run explicitly for clarity
# Skip if L1 already ran it

cd "$ROOT"
if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== TESTS FAILED ==="
    exit 1
else
    echo ""
    echo "=== All tests passed ==="
fi
