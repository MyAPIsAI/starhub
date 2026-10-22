#!/usr/bin/env bash
# Release tier test runner — builds the release binary and runs the full
# test suite plus a smoke test against the actual binary.
# Usage: ./scripts/test-release.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
FAILED=0

run_step() {
    local name="$1"; shift
    echo ""
    echo "=== $name ==="
    if "$@"; then
        echo "PASS: $name"
    else
        echo "FAIL: $name"
        FAILED=1
    fi
}

# 1. Go static checks.
run_step "L0: Go Static Checks" go build ./...
go vet ./... || { echo "FAIL: go vet"; FAILED=1; }

# 2. Go full test suite.
run_step "Go Full Test Suite" go test ./... -count=1

# 3. Frontend build + unit tests.
cd "$ROOT/web"
npm run build > /dev/null 2>&1 || { echo "FAIL: Frontend Build"; FAILED=1; }
[ "$FAILED" -eq 0 ] && echo "PASS: Frontend Build"
npm run test:unit > /dev/null 2>&1 || { echo "FAIL: Frontend Unit"; FAILED=1; }
[ "$FAILED" -eq 0 ] && echo "PASS: Frontend Unit"

# 4. Build the release binary.
cd "$ROOT"
echo ""
echo "=== Release Binary Build ==="
if [ -f "scripts/build-release.mjs" ]; then
    (cd "$ROOT/web" && node scripts/build-release.mjs > /dev/null 2>&1) \
        && echo "PASS: Release Build" \
        || { echo "FAIL: Release Build"; FAILED=1; }
else
    echo "SKIP: build-release.mjs not found"
fi

# 5. Smoke test against the release binary.
BINARY=""
if [ -f "$ROOT/release/starhub" ]; then
    BINARY="$ROOT/release/starhub"
elif [ -f "$ROOT/release/starhub.exe" ]; then
    BINARY="$ROOT/release/starhub.exe"
fi

if [ -n "$BINARY" ]; then
    echo ""
    echo "=== Smoke Test (release binary) ==="
    cd "$ROOT/release"
    "$BINARY" &
    PID=$!
    sleep 3

    if "$ROOT/scripts/smoke-test.sh" "http://localhost:3000"; then
        echo "PASS: Smoke Test"
    else
        echo "FAIL: Smoke Test"
        FAILED=1
    fi

    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
else
    echo "WARN: release binary not found — skipping smoke test"
fi

cd "$ROOT"
if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== RELEASE TESTS FAILED ==="
    exit 1
else
    echo ""
    echo "=== All release tests passed ==="
fi
