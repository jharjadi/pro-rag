#!/usr/bin/env bash
# e2e_smoke.sh — End-to-end smoke test for pro-rag
#
# Prerequisites:
#   - Docker Compose stack running (postgres, embed, core-api-go)
#   - Migrations applied and seed data loaded
#   - Test corpus ingested (make ingest-corpus)
#
# Usage:
#   ./scripts/e2e_smoke.sh [API_URL]
#
# Default API_URL: http://localhost:8000

set -euo pipefail

API_URL="${1:-http://localhost:8000}"
TENANT_ID="00000000-0000-0000-0000-000000000001"
WRONG_TENANT="99999999-9999-9999-9999-999999999999"

PASS=0
FAIL=0
TOTAL=0

# ── Helpers ───────────────────────────────────────────────

pass() {
  PASS=$((PASS + 1))
  TOTAL=$((TOTAL + 1))
  echo "  ✅ PASS: $1"
}

fail() {
  FAIL=$((FAIL + 1))
  TOTAL=$((TOTAL + 1))
  echo "  ❌ FAIL: $1"
  if [ -n "${2:-}" ]; then
    echo "         Detail: $2"
  fi
}

header() {
  echo ""
  echo "── $1 ──────────────────────────────────────"
}

# ── Test 1: Health check ──────────────────────────────────

header "Test 1: Health check"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_URL}/health" 2>/dev/null || echo "000")

if [ "$HTTP_CODE" = "200" ]; then
  pass "GET /health returns 200"
else
  fail "GET /health returns 200" "got HTTP $HTTP_CODE (is the stack running?)"
  echo ""
  echo "==> Stack does not appear to be running. Aborting."
  echo "    Start with: docker compose up -d"
  exit 1
fi

# ── Test 2: Valid query returns answer with citations ─────

header "Test 2: Valid query with citations"

RESPONSE=$(curl -s -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"${TENANT_ID}\",
    \"question\": \"What is the password policy?\",
    \"top_k\": 10,
    \"debug\": true
  }" 2>/dev/null)

# Check we got a response
if [ -z "$RESPONSE" ]; then
  fail "Got response from POST /v1/query" "empty response"
else
  pass "Got response from POST /v1/query"
fi

# Check answer field exists and is non-empty
ANSWER=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('answer',''))" 2>/dev/null || echo "")
if [ -n "$ANSWER" ] && [ "$ANSWER" != "None" ]; then
  pass "Response has non-empty answer"
else
  fail "Response has non-empty answer" "answer was empty or missing"
fi

# Check abstained is false
ABSTAINED=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('abstained', True))" 2>/dev/null || echo "True")
if [ "$ABSTAINED" = "False" ]; then
  pass "Response abstained=false (answered the question)"
else
  fail "Response abstained=false" "abstained=$ABSTAINED"
fi

# Check citations exist
NUM_CITATIONS=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('citations',[])))" 2>/dev/null || echo "0")
if [ "$NUM_CITATIONS" -gt 0 ] 2>/dev/null; then
  pass "Response has citations (count=$NUM_CITATIONS)"
else
  fail "Response has citations" "got $NUM_CITATIONS citations"
fi

# Check citations have required fields (chunk_id, doc_id, title)
CITATION_VALID=$(echo "$RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
cites = d.get('citations', [])
if not cites:
    print('no_citations')
else:
    c = cites[0]
    has_fields = all(k in c and c[k] for k in ['chunk_id', 'doc_id', 'title'])
    print('valid' if has_fields else 'missing_fields')
" 2>/dev/null || echo "error")

if [ "$CITATION_VALID" = "valid" ]; then
  pass "Citations have required fields (chunk_id, doc_id, title)"
else
  fail "Citations have required fields" "result=$CITATION_VALID"
fi

# Check debug info is populated
DEBUG_VEC=$(echo "$RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('debug',{}).get('vec_candidates',0))" 2>/dev/null || echo "0")
if [ "$DEBUG_VEC" -gt 0 ] 2>/dev/null; then
  pass "Debug info populated (vec_candidates=$DEBUG_VEC)"
else
  fail "Debug info populated" "vec_candidates=$DEBUG_VEC"
fi

# ── Test 3: Tenant isolation ─────────────────────────────

header "Test 3: Tenant isolation (wrong tenant_id)"

WRONG_RESPONSE=$(curl -s -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"${WRONG_TENANT}\",
    \"question\": \"What is the password policy?\",
    \"top_k\": 10
  }" 2>/dev/null)

WRONG_ABSTAINED=$(echo "$WRONG_RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('abstained', False))" 2>/dev/null || echo "False")
if [ "$WRONG_ABSTAINED" = "True" ]; then
  pass "Wrong tenant_id returns abstained=true"
else
  fail "Wrong tenant_id returns abstained=true" "abstained=$WRONG_ABSTAINED"
fi

# ── Test 4: Bad request handling ─────────────────────────

header "Test 4: Bad request handling"

# Missing tenant_id
BAD_CODE=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d '{"question": "test"}' 2>/dev/null || echo "000")

if [ "$BAD_CODE" = "400" ]; then
  pass "Missing tenant_id returns 400"
else
  fail "Missing tenant_id returns 400" "got HTTP $BAD_CODE"
fi

# Missing question
BAD_CODE2=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d "{\"tenant_id\": \"${TENANT_ID}\"}" 2>/dev/null || echo "000")

if [ "$BAD_CODE2" = "400" ]; then
  pass "Missing question returns 400"
else
  fail "Missing question returns 400" "got HTTP $BAD_CODE2"
fi

# Invalid JSON
BAD_CODE3=$(curl -s -o /dev/null -w "%{http_code}" -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d 'not json' 2>/dev/null || echo "000")

if [ "$BAD_CODE3" = "400" ]; then
  pass "Invalid JSON returns 400"
else
  fail "Invalid JSON returns 400" "got HTTP $BAD_CODE3"
fi

# ── Test 5: Answer content quality ───────────────────────

header "Test 5: Answer content quality"

# Check the answer mentions something related to passwords/security
ANSWER_RELEVANT=$(echo "$RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
answer = d.get('answer', '').lower()
keywords = ['password', 'character', 'security', 'complex', 'length', 'policy']
found = [k for k in keywords if k in answer]
print('relevant' if len(found) >= 2 else 'irrelevant')
" 2>/dev/null || echo "error")

if [ "$ANSWER_RELEVANT" = "relevant" ]; then
  pass "Answer is relevant to the question (mentions password/security terms)"
else
  fail "Answer is relevant to the question" "result=$ANSWER_RELEVANT"
fi

# Check answer contains citation markers [chunk:...]
HAS_MARKERS=$(echo "$RESPONSE" | python3 -c "
import sys, json, re
d = json.load(sys.stdin)
answer = d.get('answer', '')
markers = re.findall(r'\[chunk:[^\]]+\]', answer)
print('has_markers' if markers else 'no_markers')
" 2>/dev/null || echo "error")

if [ "$HAS_MARKERS" = "has_markers" ]; then
  pass "Answer contains [chunk:...] citation markers"
else
  fail "Answer contains [chunk:...] citation markers" "result=$HAS_MARKERS"
fi

# ── Summary ──────────────────────────────────────────────

echo ""
echo "════════════════════════════════════════════════"
echo "  E2E Smoke Test Results: $PASS/$TOTAL passed"
if [ "$FAIL" -gt 0 ]; then
  echo "  ❌ $FAIL test(s) FAILED"
  echo "════════════════════════════════════════════════"
  exit 1
else
  echo "  ✅ All tests passed!"
  echo "════════════════════════════════════════════════"
  exit 0
fi
