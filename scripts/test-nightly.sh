#!/usr/bin/env bash
# Nightly test runner — runs the full test suite with shuffle (3 runs)
# and coverage profiling. Designed for scheduled nightly execution.
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

# 1. Go full test suite.
run_step "Go Full Test Suite" go test ./... -count=1

# 2. Shuffle runs (3x).
for i in 1 2 3; do
    run_step "Shuffle Run $i/3" go test ./... -count=1 -shuffle=on
done

# 3. Go coverage profile.
run_step "Go Coverage" go test ./... -count=1 \
    -coverprofile=coverage-nightly.out -coverpkg=./...

# 4. Frontend unit tests.
(cd "$ROOT/web" && npm run test:unit > /dev/null 2>&1) \
    && echo "PASS: Frontend Unit" || { echo "FAIL: Frontend Unit"; FAILED=1; }

# 5. Frontend coverage.
(cd "$ROOT/web" && npm run test:coverage > /dev/null 2>&1) \
    && echo "PASS: Frontend Coverage" || { echo "FAIL: Frontend Coverage"; FAILED=1; }

# 6. Frontend build.
(cd "$ROOT/web" && npm run build > /dev/null 2>&1) \
    && echo "PASS: Frontend Build" || { echo "FAIL: Frontend Build"; FAILED=1; }

if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== NIGHTLY TESTS FAILED ==="
    exit 1
else
    echo ""
    echo "=== All nightly tests passed ==="
fi
