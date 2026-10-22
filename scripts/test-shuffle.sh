#!/usr/bin/env bash
# Nightly shuffle test runner — runs tests 3 times with shuffled execution
# order to detect order-dependent tests. Records the seed on failure.
# Usage: ./scripts/test-shuffle.sh [count]
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
COUNT="${1:-3}"
FAILED=0

echo "=== Shuffle Test ($COUNT runs) ==="

for i in $(seq 1 "$COUNT"); do
    echo ""
    echo "--- Run $i/$COUNT ---"
    OUTPUT=$(go test ./... -count=1 -shuffle=on 2>&1)
    EXIT_CODE=$?
    echo "$OUTPUT"
    if [ $EXIT_CODE -ne 0 ]; then
        echo "FAIL: Run $i"
        # Extract the seed for reproduction.
        SEED=$(echo "$OUTPUT" | grep -o '\-shuffle=[0-9]*' | head -1)
        if [ -n "$SEED" ]; then
            echo "SEED: $SEED (reproduce with: go test ./... -count=1 $SEED)"
        fi
        FAILED=1
    else
        echo "PASS: Run $i"
    fi
done

if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== SHUFFLE TESTS FAILED ==="
    exit 1
else
    echo ""
    echo "=== All $COUNT shuffle runs passed ==="
fi
