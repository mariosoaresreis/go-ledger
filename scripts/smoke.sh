#!/usr/bin/env bash
# scripts/smoke.sh
#
# End-to-end smoke test against the running local stack.
# Prerequisites: docker compose up --build -d (from the repo root).
# Usage:  bash scripts/smoke.sh [COMMAND_BASE] [QUERY_BASE]
#
# Defaults to localhost:8080 / localhost:8081.

set -euo pipefail

CMD="${1:-http://localhost:8080}"
QRY="${2:-http://localhost:8081}"
WAIT_SECS=2   # seconds to wait for the Kafka projection to land

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
RST='\033[0m'

pass() { echo -e "${GRN}✔ $1${RST}"; }
fail() { echo -e "${RED}✘ $1${RST}" >&2; exit 1; }
step() { echo -e "\n${YLW}▶ $1${RST}"; }

json_get() {
  # Usage: json_get KEY json-string
  # Extracts a top-level string value from JSON output using parameter expansion.
  local key="$1" js="$2"
  python3 -c "import sys,json; d=json.loads(sys.argv[1]); print(d['${key}'])" "$js"
}

# ── Health ────────────────────────────────────────────────────────────────────
step "1. Health checks"
CMD_PING=$(curl -sf "$CMD/api/ping") || fail "command-service unreachable at $CMD/api/ping"
QRY_PING=$(curl -sf "$QRY/api/ping") || fail "query-service unreachable at $QRY/api/ping"
echo "  command: $CMD_PING"
echo "  query:   $QRY_PING"
pass "both services healthy"

# ── Create two accounts ────────────────────────────────────────────────────────
step "2. Create accounts"

SRC_RESP=$(curl -sf -X POST "$CMD/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"ownerId":"owner-alice","currency":"USD"}')
SRC_ID=$(json_get id "$SRC_RESP")
echo "  source account id: $SRC_ID"
pass "source account created"

DST_RESP=$(curl -sf -X POST "$CMD/api/v1/accounts" \
  -H "Content-Type: application/json" \
  -d '{"ownerId":"owner-bob","currency":"USD"}')
DST_ID=$(json_get id "$DST_RESP")
echo "  target account id: $DST_ID"
pass "target account created"

# ── Credit source ─────────────────────────────────────────────────────────────
step "3. Credit source account with 50000 cents (\$500)"
curl -sf -X POST "$CMD/api/v1/accounts/$SRC_ID/credits" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: seed-credit-$(uuidgen 2>/dev/null || date +%s%N)" \
  -d "{\"amount\":50000,\"currency\":\"USD\",\"reference\":\"initial-deposit\"}" \
  | python3 -c "import sys,json; e=json.load(sys.stdin); print('  eventType:', e.get('event_type','?'))"
pass "credit applied"

# ── Transfer ───────────────────────────────────────────────────────────────────
step "4. Transfer 12000 cents (\$120) from source to target"
TX_RESP=$(curl -sf -X POST "$CMD/api/v1/transfers" \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: transfer-$(uuidgen 2>/dev/null || date +%s%N)" \
  -d "{\"sourceAccountId\":\"$SRC_ID\",\"targetAccountId\":\"$DST_ID\",\"amount\":12000,\"currency\":\"USD\"}")
TX_TYPE=$(json_get event_type "$TX_RESP")
echo "  saga event: $TX_TYPE"
pass "transfer saga initiated and completed"

# ── Wait for projection ────────────────────────────────────────────────────────
step "5. Wait ${WAIT_SECS}s for Kafka projection to land in query service"
sleep "$WAIT_SECS"

# ── Query balances ─────────────────────────────────────────────────────────────
step "6. Query balances"
SRC_BAL=$(curl -sf "$QRY/api/v1/accounts/$SRC_ID/balance")
echo "  source: $SRC_BAL"
SRC_BAL_AMOUNT=$(json_get balance "$SRC_BAL")
if [[ "$SRC_BAL_AMOUNT" -ne 38000 ]]; then
  fail "source balance: expected 38000, got $SRC_BAL_AMOUNT"
fi
pass "source balance correct (38000 cents = \$380)"

DST_BAL=$(curl -sf "$QRY/api/v1/accounts/$DST_ID/balance")
echo "  target: $DST_BAL"
DST_BAL_AMOUNT=$(json_get balance "$DST_BAL")
if [[ "$DST_BAL_AMOUNT" -ne 12000 ]]; then
  fail "target balance: expected 12000, got $DST_BAL_AMOUNT"
fi
pass "target balance correct (12000 cents = \$120)"

# ── Transaction history ────────────────────────────────────────────────────────
step "7. List source transactions"
SRC_TXN=$(curl -sf "$QRY/api/v1/accounts/$SRC_ID/transactions?size=10")
echo "  $SRC_TXN" | python3 -c "
import sys,json
resp = json.load(sys.stdin)
data = resp.get('data', [])
print(f'  total: {resp.get(\"totalCount\", 0)} transaction(s)')
for t in data:
    print(f'  {t[\"direction\"]:6s}  {t[\"amount\"]:>10}  {t[\"reference\"]}')
"
pass "transaction history retrieved"

# ── Freeze source account ──────────────────────────────────────────────────────
step "8. Freeze source account"
curl -sf -X PATCH "$CMD/api/v1/accounts/$SRC_ID/status" \
  -H "Content-Type: application/json" \
  -d '{"status":"FROZEN"}' \
  | python3 -c "import sys,json; e=json.load(sys.stdin); print('  eventType:', e.get('event_type','?'))"
pass "status changed to FROZEN"

# ── Verify raw event log ───────────────────────────────────────────────────────
step "9. Fetch raw event log for source account (from command service)"
EVENTS=$(curl -sf "$CMD/api/v1/accounts/$SRC_ID/events")
COUNT=$(echo "$EVENTS" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d))")
echo "  events found for source: $COUNT"
if [[ "$COUNT" -lt 3 ]]; then
  fail "expected at least 3 events, got $COUNT"
fi
pass "event log correct (>= 3 events)"

# ── Monthly statement ──────────────────────────────────────────────────────────
step "10. Monthly statement for source"
MONTH=$(date +%Y-%m)
STMT=$(curl -sf "$QRY/api/v1/accounts/$SRC_ID/statement?month=$MONTH")
echo "  $STMT" | python3 -c "
import sys,json
s = json.load(sys.stdin)
print(f'  month: {s.get(\"month\")}, closing: {s.get(\"closingBalance\")}')
"
pass "statement retrieved"

echo -e "\n${GRN}══════════════════════════════════════${RST}"
echo -e "${GRN}  All smoke tests passed ✔${RST}"
echo -e "${GRN}══════════════════════════════════════${RST}\n"

