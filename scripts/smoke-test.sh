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

# check_status: verify HTTP status code only
check_status() {
  local name="$1"
  local want="$2"
  local url="$3"
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' "$url" 2>&1 || echo "000")
  if [ "$code" = "$want" ]; then
    echo "  PASS: $name (status $code)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $name (expected status $want, got $code)"
    FAIL=$((FAIL + 1))
  fi
}

# check_json: verify HTTP 200 and response body contains expected substring
check_json() {
  local name="$1"
  local expected="$2"
  local url="$3"
  local extra_curl="${4:-}"
  local body
  body=$(curl -sS -w '\n%{http_code}' $extra_curl "$url" 2>&1 || echo "")
  local code
  code=$(echo "$body" | tail -1)
  local json
  json=$(echo "$body" | sed '$d')
  if [ "$code" = "200" ] && echo "$json" | grep -q "$expected"; then
    echo "  PASS: $name (status 200, matched '$expected')"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $name (status $code, expected 200 with '$expected', got: $(echo "$json" | head -c 200))"
    FAIL=$((FAIL + 1))
  fi
}

# check_json_with_cookie: same as check_json but with session cookie
check_auth() {
  local name="$1"
  local expected="$2"
  local url="$3"
  check_json "$name" "$expected" "$url" "-b $COOKIE_FILE"
}

# check_admin: admin endpoint — 200 means admin access, 403 means non-admin (both acceptable)
check_admin() {
  local name="$1"
  local url="$2"
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' -b "$COOKIE_FILE" "$url" 2>&1 || echo "000")
  if [ "$code" = "200" ] || [ "$code" = "403" ]; then
    echo "  PASS: $name (status $code)"
    PASS=$((PASS + 1))
  else
    echo "  FAIL: $name (expected 200 or 403, got $code)"
    FAIL=$((FAIL + 1))
  fi
}

echo "=== L4: Smoke Test against $BASE ==="

# 1. Public endpoints (no auth)
echo "-- Public endpoints --"
check_status "GET / (homepage)" "200" "$BASE/"
check_json "GET /api/plaza/models" '"success":true' "$BASE/api/plaza/models"
check_json "GET /api/plaza/filters" '"success":true' "$BASE/api/plaza/filters"
check_json "GET /api/plaza/rank" '"success":true' "$BASE/api/plaza/rank?type=value"

# 2. Register + Login
echo "-- Auth --"
REG_BODY='{"username":"smoketest","password":"secret123","email":"smoke@test.com"}'
REG_CODE=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE/api/user/register" -H "Content-Type: application/json" -d "$REG_BODY" 2>&1 || echo "000")
if [ "$REG_CODE" = "200" ] || [ "$REG_CODE" = "409" ]; then
  echo "  PASS: Register (status $REG_CODE)"
  PASS=$((PASS + 1))
else
  echo "  FAIL: Register (expected 200 or 409, got $REG_CODE)"
  FAIL=$((FAIL + 1))
fi

LOGIN_CODE=$(curl -sS -o /dev/null -w '%{http_code}' -X POST "$BASE/api/user/login" -H "Content-Type: application/json" -d '{"username":"smoketest","password":"secret123"}' -c "$COOKIE_FILE" 2>&1 || echo "000")
if [ "$LOGIN_CODE" = "200" ]; then
  echo "  PASS: Login (status $LOGIN_CODE)"
  PASS=$((PASS + 1))
else
  echo "  FAIL: Login (expected 200, got $LOGIN_CODE)"
  FAIL=$((FAIL + 1))
fi

# 3. Authenticated user endpoints
echo "-- User endpoints --"
check_auth "GET /api/user/self" '"success":true' "$BASE/api/user/self"
check_auth "GET /api/wallet" '"success":true' "$BASE/api/wallet"
check_auth "GET /api/token" '"success":true' "$BASE/api/token"
check_auth "GET /api/user/usage" '"success":true' "$BASE/api/user/usage?days=7"
check_auth "GET /api/log" '"success":true' "$BASE/api/log"
check_auth "GET /api/service-quality" '"success":true' "$BASE/api/service-quality?hours=24"
check_auth "GET /api/ledger" '"success":true' "$BASE/api/ledger"
check_auth "GET /api/recharge" '"success":true' "$BASE/api/recharge"
check_auth "GET /api/export" '"success":true' "$BASE/api/export"

# 4. Admin endpoints (smoketest user may not be admin; tolerate 403)
echo "-- Admin endpoints (tolerate 403) --"
check_admin "GET /api/user (list users)" "$BASE/api/user"
check_admin "GET /api/audit" "$BASE/api/audit"
check_admin "GET /api/metrics" "$BASE/api/metrics"
check_admin "GET /api/channel" "$BASE/api/channel"
check_admin "GET /api/redemption" "$BASE/api/redemption"
check_admin "GET /api/option/status" "$BASE/api/option/status"
check_admin "GET /api/option/history" "$BASE/api/option/history"
check_admin "GET /api/service-group" "$BASE/api/service-group"
check_admin "GET /api/plaza/meta" "$BASE/api/plaza/meta"

echo ""
echo "=== Smoke test complete: $PASS passed, $FAIL failed ==="
exit $FAIL
