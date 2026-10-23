#!/usr/bin/env bash
# Release tier test runner — builds the release binary and runs the full
# test suite plus a smoke test against the actual binary in an isolated
# environment (temp DB, random port).
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

# 4. Build the release binary (uses absolute path to build-release.mjs).
cd "$ROOT"
echo ""
echo "=== Release Binary Build ==="
if [ -f "$ROOT/web/scripts/build-release.mjs" ]; then
    (cd "$ROOT/web" && node "$ROOT/web/scripts/build-release.mjs" > /dev/null 2>&1) \
        && echo "PASS: Release Build" \
        || { echo "FAIL: Release Build"; FAILED=1; }
else
    echo "SKIP: build-release.mjs not found"
fi

# 5. Smoke test against the release binary (isolated DB + random port).
BINARY=""
if [ -f "$ROOT/release/starhub" ]; then
    BINARY="$ROOT/release/starhub"
elif [ -f "$ROOT/release/starhub.exe" ]; then
    BINARY="$ROOT/release/starhub.exe"
fi

if [ -z "$BINARY" ]; then
    echo "FAIL: release binary not found after build"
    FAILED=1
else
    echo "Binary found: $BINARY"

    # Create an isolated temp directory for the DB.
    TEMP_DIR=$(mktemp -d)
    # Find a free port.
    PORT=$(python3 -c "import socket; s=socket.socket(); s.bind(('',0)); print(s.getsockname()[1]); s.close()" 2>/dev/null || echo "3000")

    echo "Smoke test: port=$PORT, DB=$TEMP_DIR"

    # Start the binary in the temp directory.
    cd "$TEMP_DIR"
    "$BINARY" -port "$PORT" &
    PID=$!

    # Wait for the server to be ready (poll every 500ms, max 15s).
    READY=0
    for i in $(seq 1 30); do
        sleep 0.5
        if curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/api/plaza/models" 2>/dev/null | grep -q "200"; then
            READY=1
            break
        fi
    done

    if [ "$READY" -eq 0 ]; then
        echo "FAIL: server did not become ready within 15s"
        FAILED=1
    else
        echo "Server ready on port $PORT"
        run_step "Smoke Test (release binary, isolated)" \
            "$ROOT/scripts/smoke-test.sh" "http://localhost:$PORT"
    fi

    # Cleanup: kill the server and remove the temp directory.
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
    rm -rf "$TEMP_DIR"
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
