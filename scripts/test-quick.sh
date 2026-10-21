#!/usr/bin/env bash
# Quick tier test runner — for local development.
# Runs L0 (static checks) + L1 (Go unit tests, -short) + L1 (frontend unit).
# Skips E2E and long-running performance tests.
# Usage: ./scripts/test-quick.sh
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FAILED=0

run() {
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

# L0: Static Checks
cd "$ROOT"
go build ./... && go vet ./... || { echo "FAIL: L0 Static"; FAILED=1; }
[ "$FAILED" -eq 0 ] && echo "PASS: L0 Static"

# L1: Go Unit (-short, skip e2e/perf)
run "L1: Go Unit (-short)" \
    go test ./controller/... ./model/... ./middleware/... ./relay/... ./service/... ./setting/... ./common/... ./pkg/... ./metric/... -short -count=1

# L1: Frontend Unit
cd "$ROOT/web"
npm run test:unit > /dev/null 2>&1
if [ $? -eq 0 ]; then echo "PASS: L1 Frontend"; else echo "FAIL: L1 Frontend"; FAILED=1; fi

cd "$ROOT"
if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== QUICK TESTS FAILED ==="
    exit 1
else
    echo ""
    echo "=== Quick tests passed ==="
fi
