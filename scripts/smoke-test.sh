#!/usr/bin/env bash
# L4 Smoke test — verifies key endpoints against a running StarHub server.
# Usage: ./scripts/smoke-test.sh [BASE_URL]
# Default: http://localhost:3000
# Prerequisite: start the server first with ./release/starhub or go run .

set -e

BASE="${1:-http://localhost:3000}"
COOKIE_FILE=$(mktemp)
PASS=0
FAIL=0

trap 'rm -f "$COOKIE_FILE"' EXIT

check() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  if echo "$actual" | grep -q "$expected"; then
    echo "  PASS: $name"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $name (expected '$expected', got: $(echo "$actual" | head -c 200))"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== L4: Smoke Test against $BASE ==="

# 1. Public endpoints (no auth)
echo "-- Public endpoints --"
check "GET /" "StarHub\|<!DOCTYPE\|<html" "$(curl -sS -o /dev/null -w '%{http_code}' "$BASE/")"
check "GET /api/plaza/models" '"success"' "$(curl -sS "$BASE/api/plaza/models")"
check "GET /api/plaza/filters" '"success"' "$(curl -sS "$BASE/api/plaza/filters")"
check "GET /api/plaza/rank?type=value" '"success"\|"items"' "$(curl -sS "$BASE/api/plaza/rank?type=value")"

# 2. Register + Login
echo "-- Auth --"
REG_BODY='{"username":"smoketest","password":"secret123","email":"smoke@test.com"}'
REG_RESP=$(curl -sS -X POST "$BASE/api/user/register" -H "Content-Type: application/json" -d "$REG_BODY" -w "\n%{http_code}" 2>&1 || true)
REG_CODE=$(echo "$REG_RESP" | tail -1)
if [ "$REG_CODE" = "200" ] || [ "$REG_CODE" = "409" ]; then
  echo "  PASS: Register (status $REG_CODE)"
  PASS=$((PASS + 1))
else
  echo "  FAIL: Register (status $REG_CODE)"
  FAIL=$((FAIL + 1))
fi

LOGIN_RESP=$(curl -sS -X POST "$BASE/api/user/login" -H "Content-Type: application/json" -d '{"username":"smoketest","password":"secret123"}' -c "$COOKIE_FILE" -w "\n%{http_code}" 2>&1)
LOGIN_CODE=$(echo "$LOGIN_RESP" | tail -1)
if [ "$LOGIN_CODE" = "200" ]; then
  echo "  PASS: Login (status $LOGIN_CODE)"
  PASS=$((PASS + 1))
else
  echo "  FAIL: Login (status $LOGIN_CODE)"
  FAIL=$((FAIL + 1))
fi

# 3. Authenticated user endpoints
echo "-- User endpoints --"
check "GET /api/user/self" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/user/self")"
check "GET /api/wallet" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/wallet")"
check "GET /api/token" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/token")"
check "GET /api/user/usage?days=7" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/user/usage?days=7")"
check "GET /api/log" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/log")"
check "GET /api/service-quality?hours=24" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/service-quality?hours=24")"
check "GET /api/ledger" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/ledger")"
check "GET /api/recharge" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/recharge")"
check "GET /api/export" '"success"' "$(curl -sS -b "$COOKIE_FILE" "$BASE/api/export")"

# 4. Admin endpoints (smoketest user may not be admin; tolerate 403)
echo "-- Admin endpoints (tolerate 403) --"
for ep in "/api/user" "/api/audit" "/api/metrics" "/api/channel" "/api/redemption" "/api/option/status" "/api/option/history" "/api/service-group" "/api/plaza/meta"; do
  RESP=$(curl -sS -b "$COOKIE_FILE" -o /dev/null -w "%{http_code}" "$BASE$ep" 2>&1 || true)
  if [ "$RESP" = "200" ] || [ "$RESP" = "403" ]; then
    echo "  PASS: GET $ep (status $RESP)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: GET $ep (status $RESP)"
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "=== Smoke test complete: $PASS passed, $FAIL failed ==="
exit $FAIL
