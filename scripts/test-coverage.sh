#!/usr/bin/env bash
# Generate Go test coverage report.
# Usage: ./scripts/test-coverage.sh
# Output: coverage.out (raw), coverage-summary.txt (per-function),
#         coverage.html (browsable).
# Requires: go tool cover (built into Go toolchain).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

OUT="coverage.out"
SUMMARY="coverage-summary.txt"
HTML="coverage.html"

echo "=== Generating Go test coverage ==="

# Run all tests with coverage profiling. -coverpkg=./... aggregates
# cross-package coverage (a test in controller may exercise model code).
go test ./... -count=1 -coverprofile="$OUT" -coverpkg=./...

# Per-function summary (sorted by coverage %).
go tool cover -func="$OUT" | sort -t$'\t' -k3 -n -r > "$SUMMARY"

# Browsable HTML report.
go tool cover -html="$OUT" -o "$HTML"

# Print top-level stats.
TOTAL=$(grep -c "^" "$SUMMARY")
echo ""
echo "=== Coverage Summary ==="
echo "Total functions: $TOTAL"
echo "Top 10 (by coverage):"
head -10 "$SUMMARY"
echo ""
echo "Bottom 10 (by coverage):"
tail -10 "$SUMMARY"
echo ""
echo "Reports: $OUT, $SUMMARY, $HTML"
