#!/usr/bin/env bash
# Nightly shuffle test runner — runs tests N times with shuffled execution
# order to detect order-dependent tests. Persists failure seeds to a file.
# Usage: ./scripts/test-shuffle.sh [count]
# Default count: 3
#
# On failure, the seed is:
# 1. Printed to stdout
# 2. Appended to shuffle-failures.log with timestamp and commit SHA
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
COUNT="${1:-3}"
FAILED=0
SEED_LOG="$ROOT/shuffle-failures.log"

echo "=== Shuffle Test ($COUNT runs) ==="

for i in $(seq 1 "$COUNT"); do
    echo ""
    echo "--- Run $i/$COUNT ---"
    OUTPUT=$(go test ./... -count=1 -shuffle=on 2>&1)
    EXIT_CODE=$?
    echo "$OUTPUT"

    if [ $EXIT_CODE -ne 0 ]; then
        echo "FAIL: Run $i (exit $EXIT_CODE)"

        # Extract seeds from output.
        SEEDS=$(echo "$OUTPUT" | grep -o '\-shuffle=[0-9]*' | sort -u)

        COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
        TIMESTAMP=$(date -Iseconds)

        if [ -n "$SEEDS" ]; then
            for SEED in $SEEDS; do
                echo "SEED: $SEED"
                echo -e "$TIMESTAMP\t$COMMIT\tRun $i\t$SEED\tgo test ./... -count=1 $SEED" >> "$SEED_LOG"
            done
        else
            echo "WARN: no seed found in output (failure may be non-order-dependent)"
        fi

        FAILED=1
    else
        echo "PASS: Run $i"
    fi
done

if [ "$FAILED" -eq 1 ]; then
    echo ""
    echo "=== SHUFFLE TESTS FAILED ==="
    echo "Failure seeds saved to: $SEED_LOG"
    echo "Reproduce with: go test ./... -count=1 -shuffle=<seed>"
    echo "View log: cat $SEED_LOG"
    exit 1
else
    echo ""
    echo "=== All $COUNT shuffle runs passed ==="
fi
