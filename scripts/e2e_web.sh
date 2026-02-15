#!/usr/bin/env bash
# e2e_web.sh — E2E integration test: upload → verify in list → query → citations → deactivate
#
# Tests the full web integration flow through the Go API gateway:
#   1. Create a small test HTML file
#   2. Upload via POST /v1/ingest (multipart form → Go orchestrator → ingest-worker)
#   3. Poll ingestion run until complete (queued → running → succeeded)
#   4. Verify document appears in document list
#   5. Query about the document content
#   6. Verify answer has citations referencing the uploaded document
#   7. Deactivate the document
#   8. Verify deactivated document no longer returns results
#   9. Clean up
#
# Prerequisites:
#   - Full Docker Compose stack running (postgres, embed, core-api-go, ingest-worker, web)
#   - Migrations applied and seed data loaded
#
# Usage:
#   ./scripts/e2e_web.sh [API_URL]
#
# Default API_URL: http://localhost:8000

set -euo pipefail

API_URL="${1:-http://localhost:8000}"
TENANT_ID="00000000-0000-0000-0000-000000000001"
POLL_INTERVAL=3
POLL_MAX_ATTEMPTS=60  # 3 min max wait for ingestion

PASS=0
FAIL=0
TOTAL=0
CLEANUP_DOC_ID=""

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

cleanup() {
  echo ""
  echo "── Cleanup ──────────────────────────────────────"
  # Remove temp file
  if [ -n "${TEMP_FILE:-}" ] && [ -f "$TEMP_FILE" ]; then
    rm -f "$TEMP_FILE"
    echo "  Removed temp file: $TEMP_FILE"
  fi
  # Deactivate test document if it was created and not already deactivated
  if [ -n "$CLEANUP_DOC_ID" ]; then
    curl -s -X POST "${API_URL}/v1/documents/${CLEANUP_DOC_ID}/deactivate?tenant_id=${TENANT_ID}" >/dev/null 2>&1 || true
    echo "  Deactivated test document: $CLEANUP_DOC_ID"
  fi
}

trap cleanup EXIT

# ── Pre-check: Health ─────────────────────────────────────

header "Pre-check: Health"

HTTP_CODE=$(curl -s -o /dev/null -w "%{http_code}" "${API_URL}/health" 2>/dev/null || echo "000")

if [ "$HTTP_CODE" = "200" ]; then
  pass "GET /health returns 200"
else
  fail "GET /health returns 200" "got HTTP $HTTP_CODE (is the full stack running?)"
  echo ""
  echo "==> Stack does not appear to be running. Aborting."
  echo "    Start with: docker compose up -d"
  exit 1
fi

# ── Step 1: Create test HTML file ─────────────────────────

header "Step 1: Create test HTML file"

TEMP_FILE=$(mktemp /tmp/e2e_web_test_XXXXXX.html)
UNIQUE_KEYWORD="ZephyrQuantum$(date +%s)"

cat > "$TEMP_FILE" <<EOF
<!DOCTYPE html>
<html>
<head><title>${UNIQUE_KEYWORD} Workplace Safety Policy</title></head>
<body>
<h1>${UNIQUE_KEYWORD} Workplace Safety Policy</h1>

<h2>1. Purpose</h2>
<p>This policy establishes the workplace safety requirements for ${UNIQUE_KEYWORD} Corporation.
All employees must follow these guidelines to maintain a safe working environment.</p>

<h2>2. Emergency Procedures</h2>
<p>In case of a fire emergency, all employees must evacuate using the nearest marked exit.
Assembly points are located in the north parking lot and the south courtyard.
Fire wardens on each floor are responsible for ensuring complete evacuation.</p>

<h2>3. Ergonomic Standards</h2>
<p>All workstations must comply with ${UNIQUE_KEYWORD} ergonomic standards.
Monitors should be positioned at arm's length with the top of the screen at eye level.
Chairs must have adjustable height and lumbar support.
Standing desk options are available upon request from the facilities team.</p>

<h2>4. Incident Reporting</h2>
<p>All workplace incidents, including near-misses, must be reported within 24 hours
using the ${UNIQUE_KEYWORD} Safety Incident Form. The safety committee reviews all
reports monthly and publishes findings in the quarterly safety bulletin.</p>

<h2>5. Personal Protective Equipment</h2>
<p>Employees working in laboratory or warehouse areas must wear appropriate PPE
including safety glasses, steel-toed boots, and high-visibility vests.
PPE is provided at no cost and must be replaced when damaged.</p>
</body>
</html>
EOF

if [ -f "$TEMP_FILE" ]; then
  pass "Created test HTML file: $TEMP_FILE"
else
  fail "Created test HTML file" "file not found"
  exit 1
fi

# ── Step 2: Upload via POST /v1/ingest ────────────────────

header "Step 2: Upload document via POST /v1/ingest"

UPLOAD_TITLE="${UNIQUE_KEYWORD} Workplace Safety Policy"

UPLOAD_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST "${API_URL}/v1/ingest" \
  -F "file=@${TEMP_FILE};filename=e2e_test_safety_policy.html" \
  -F "tenant_id=${TENANT_ID}" \
  -F "title=${UPLOAD_TITLE}" 2>/dev/null)

UPLOAD_HTTP_CODE=$(echo "$UPLOAD_RESPONSE" | tail -1)
UPLOAD_BODY=$(echo "$UPLOAD_RESPONSE" | sed '$d')

if [ "$UPLOAD_HTTP_CODE" = "202" ]; then
  pass "POST /v1/ingest returns 202 Accepted"
else
  fail "POST /v1/ingest returns 202 Accepted" "got HTTP $UPLOAD_HTTP_CODE — body: $UPLOAD_BODY"
  echo ""
  echo "==> Upload failed. Is ingest-worker running? Aborting."
  exit 1
fi

# Extract run_id
RUN_ID=$(echo "$UPLOAD_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('run_id',''))" 2>/dev/null || echo "")
if [ -n "$RUN_ID" ]; then
  pass "Got run_id: $RUN_ID"
else
  fail "Got run_id from upload response" "response: $UPLOAD_BODY"
  exit 1
fi

# ── Step 3: Poll ingestion run until complete ─────────────

header "Step 3: Poll ingestion run until complete"

ATTEMPT=0
INGEST_STATUS="queued"

# Poll until terminal status (succeeded/failed) or timeout.
# New flow: queued → running → succeeded/failed (spec v2.3 §9.3)
while [ "$INGEST_STATUS" = "queued" ] || [ "$INGEST_STATUS" = "running" ]; do
  if [ "$ATTEMPT" -ge "$POLL_MAX_ATTEMPTS" ]; then
    break
  fi
  ATTEMPT=$((ATTEMPT + 1))
  sleep "$POLL_INTERVAL"

  RUN_RESPONSE=$(curl -s "${API_URL}/v1/ingestion-runs/${RUN_ID}?tenant_id=${TENANT_ID}" 2>/dev/null)
  INGEST_STATUS=$(echo "$RUN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null || echo "unknown")

  echo "  Poll $ATTEMPT/$POLL_MAX_ATTEMPTS: status=$INGEST_STATUS"
done

if [ "$INGEST_STATUS" = "succeeded" ]; then
  pass "Ingestion completed successfully (status=succeeded)"
elif [ "$INGEST_STATUS" = "running" ] || [ "$INGEST_STATUS" = "queued" ]; then
  fail "Ingestion completed" "still $INGEST_STATUS after $((ATTEMPT * POLL_INTERVAL))s — timed out"
  exit 1
else
  INGEST_ERROR=$(echo "$RUN_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('error','unknown'))" 2>/dev/null || echo "unknown")
  fail "Ingestion completed" "status=$INGEST_STATUS, error=$INGEST_ERROR"
  exit 1
fi

# ── Step 4: Verify document appears in list ───────────────

header "Step 4: Verify document appears in document list"

DOC_LIST_RESPONSE=$(curl -s "${API_URL}/v1/documents?tenant_id=${TENANT_ID}&search=${UNIQUE_KEYWORD}" 2>/dev/null)

DOC_COUNT=$(echo "$DOC_LIST_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total',0))" 2>/dev/null || echo "0")

if [ "$DOC_COUNT" -gt 0 ] 2>/dev/null; then
  pass "Document found in list (search='${UNIQUE_KEYWORD}', count=$DOC_COUNT)"
else
  fail "Document found in list" "search='${UNIQUE_KEYWORD}' returned $DOC_COUNT results"
fi

# Extract doc_id for later use
DOC_ID=$(echo "$DOC_LIST_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
docs = d.get('documents', [])
print(docs[0]['doc_id'] if docs else '')
" 2>/dev/null || echo "")

if [ -n "$DOC_ID" ]; then
  pass "Got doc_id: $DOC_ID"
  CLEANUP_DOC_ID="$DOC_ID"
else
  fail "Got doc_id from document list" "no documents returned"
  exit 1
fi

# Verify document has active version with chunks
HAS_CHUNKS=$(echo "$DOC_LIST_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
docs = d.get('documents', [])
if docs and docs[0].get('active_version'):
    av = docs[0]['active_version']
    print('yes' if av.get('chunk_count', 0) > 0 else 'no')
else:
    print('no')
" 2>/dev/null || echo "no")

if [ "$HAS_CHUNKS" = "yes" ]; then
  CHUNK_COUNT=$(echo "$DOC_LIST_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d['documents'][0]['active_version']['chunk_count'])
" 2>/dev/null || echo "?")
  pass "Document has active version with chunks (count=$CHUNK_COUNT)"
else
  fail "Document has active version with chunks" "no active version or zero chunks"
fi

# ── Step 5: Get document detail ───────────────────────────

header "Step 5: Get document detail"

DOC_DETAIL=$(curl -s "${API_URL}/v1/documents/${DOC_ID}?tenant_id=${TENANT_ID}" 2>/dev/null)

DOC_TITLE=$(echo "$DOC_DETAIL" | python3 -c "import sys,json; print(json.load(sys.stdin).get('title',''))" 2>/dev/null || echo "")

if echo "$DOC_TITLE" | grep -q "$UNIQUE_KEYWORD"; then
  pass "Document detail has correct title containing '$UNIQUE_KEYWORD'"
else
  fail "Document detail has correct title" "got: $DOC_TITLE"
fi

# Check versions
VERSION_COUNT=$(echo "$DOC_DETAIL" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('versions',[])))" 2>/dev/null || echo "0")
if [ "$VERSION_COUNT" -gt 0 ] 2>/dev/null; then
  pass "Document has $VERSION_COUNT version(s)"
else
  fail "Document has versions" "got $VERSION_COUNT versions"
fi

# ── Step 6: Browse document chunks ────────────────────────

header "Step 6: Browse document chunks"

CHUNKS_RESPONSE=$(curl -s "${API_URL}/v1/documents/${DOC_ID}/chunks?tenant_id=${TENANT_ID}&limit=10" 2>/dev/null)

CHUNKS_TOTAL=$(echo "$CHUNKS_RESPONSE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('total',0))" 2>/dev/null || echo "0")

if [ "$CHUNKS_TOTAL" -gt 0 ] 2>/dev/null; then
  pass "Document has $CHUNKS_TOTAL chunks"
else
  fail "Document has chunks" "got $CHUNKS_TOTAL chunks"
fi

# Check chunk has expected fields
CHUNK_VALID=$(echo "$CHUNKS_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
chunks = d.get('chunks', [])
if chunks:
    c = chunks[0]
    has_fields = all(k in c for k in ['chunk_id', 'text', 'token_count', 'ordinal'])
    print('valid' if has_fields else 'missing_fields')
else:
    print('no_chunks')
" 2>/dev/null || echo "error")

if [ "$CHUNK_VALID" = "valid" ]; then
  pass "Chunks have required fields (chunk_id, text, token_count, ordinal)"
else
  fail "Chunks have required fields" "result=$CHUNK_VALID"
fi

# ── Step 7: Query about the uploaded document ─────────────

header "Step 7: Query about the uploaded document"

QUERY_RESPONSE=$(curl -s -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"${TENANT_ID}\",
    \"question\": \"What are the emergency evacuation procedures in the ${UNIQUE_KEYWORD} workplace safety policy?\",
    \"top_k\": 10,
    \"debug\": true
  }" 2>/dev/null)

# Check we got a response
if [ -z "$QUERY_RESPONSE" ]; then
  fail "Got response from query" "empty response"
else
  pass "Got response from query"
fi

# Check answer is non-empty
ANSWER=$(echo "$QUERY_RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('answer',''))" 2>/dev/null || echo "")
if [ -n "$ANSWER" ] && [ "$ANSWER" != "None" ]; then
  pass "Query returned non-empty answer"
else
  fail "Query returned non-empty answer" "answer was empty or missing"
fi

# Check not abstained
ABSTAINED=$(echo "$QUERY_RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('abstained', True))" 2>/dev/null || echo "True")
if [ "$ABSTAINED" = "False" ]; then
  pass "Query did not abstain (answered the question)"
else
  fail "Query did not abstain" "abstained=$ABSTAINED"
fi

# Check answer mentions relevant content (evacuation, fire, assembly, exit)
ANSWER_RELEVANT=$(echo "$QUERY_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
answer = d.get('answer', '').lower()
keywords = ['evacuat', 'fire', 'assembly', 'exit', 'emergency', 'warden', 'parking']
found = [k for k in keywords if k in answer]
print('relevant' if len(found) >= 2 else 'irrelevant')
" 2>/dev/null || echo "error")

if [ "$ANSWER_RELEVANT" = "relevant" ]; then
  pass "Answer is relevant (mentions evacuation/fire/emergency terms)"
else
  fail "Answer is relevant" "result=$ANSWER_RELEVANT — answer: ${ANSWER:0:200}"
fi

# ── Step 8: Verify citations reference the uploaded doc ───

header "Step 8: Verify citations reference the uploaded document"

NUM_CITATIONS=$(echo "$QUERY_RESPONSE" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('citations',[])))" 2>/dev/null || echo "0")
if [ "$NUM_CITATIONS" -gt 0 ] 2>/dev/null; then
  pass "Response has citations (count=$NUM_CITATIONS)"
else
  fail "Response has citations" "got $NUM_CITATIONS citations"
fi

# Check at least one citation references our uploaded document
CITATION_MATCHES=$(echo "$QUERY_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
cites = d.get('citations', [])
doc_id = '$DOC_ID'
keyword = '$UNIQUE_KEYWORD'
matches = [c for c in cites if c.get('doc_id') == doc_id or keyword in c.get('title', '')]
print(len(matches))
" 2>/dev/null || echo "0")

if [ "$CITATION_MATCHES" -gt 0 ] 2>/dev/null; then
  pass "Citations reference the uploaded document (matches=$CITATION_MATCHES)"
else
  # Print citation details for debugging
  CITE_DETAILS=$(echo "$QUERY_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
for c in d.get('citations', [])[:3]:
    print(f\"  doc_id={c.get('doc_id','?')}, title={c.get('title','?')}\")
" 2>/dev/null || echo "  (could not parse)")
  fail "Citations reference the uploaded document" "no matches for doc_id=$DOC_ID or keyword=$UNIQUE_KEYWORD. Citations:\n$CITE_DETAILS"
fi

# Check answer contains [chunk:...] markers
HAS_MARKERS=$(echo "$QUERY_RESPONSE" | python3 -c "
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

# ── Step 9: Deactivate the document ───────────────────────

header "Step 9: Deactivate the document"

DEACTIVATE_RESPONSE=$(curl -s -w "\n%{http_code}" -X POST \
  "${API_URL}/v1/documents/${DOC_ID}/deactivate?tenant_id=${TENANT_ID}" 2>/dev/null)

DEACTIVATE_HTTP_CODE=$(echo "$DEACTIVATE_RESPONSE" | tail -1)
DEACTIVATE_BODY=$(echo "$DEACTIVATE_RESPONSE" | sed '$d')

if [ "$DEACTIVATE_HTTP_CODE" = "200" ]; then
  pass "POST /v1/documents/:id/deactivate returns 200"
  CLEANUP_DOC_ID=""  # Already deactivated, no need to clean up
else
  fail "POST /v1/documents/:id/deactivate returns 200" "got HTTP $DEACTIVATE_HTTP_CODE — body: $DEACTIVATE_BODY"
fi

DEACTIVATE_STATUS=$(echo "$DEACTIVATE_BODY" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status',''))" 2>/dev/null || echo "")
if [ "$DEACTIVATE_STATUS" = "deactivated" ]; then
  pass "Deactivate response status='deactivated'"
else
  fail "Deactivate response status='deactivated'" "got: $DEACTIVATE_STATUS"
fi

# ── Step 10: Verify deactivated doc not returned in query ─

header "Step 10: Verify deactivated document not returned in query"

DEACT_QUERY_RESPONSE=$(curl -s -X POST "${API_URL}/v1/query" \
  -H "Content-Type: application/json" \
  -d "{
    \"tenant_id\": \"${TENANT_ID}\",
    \"question\": \"What are the ${UNIQUE_KEYWORD} emergency evacuation procedures?\",
    \"top_k\": 10,
    \"debug\": true
  }" 2>/dev/null)

# Check that citations do NOT reference the deactivated document
DEACT_CITATION_MATCHES=$(echo "$DEACT_QUERY_RESPONSE" | python3 -c "
import sys, json
d = json.load(sys.stdin)
cites = d.get('citations', [])
doc_id = '$DOC_ID'
keyword = '$UNIQUE_KEYWORD'
matches = [c for c in cites if c.get('doc_id') == doc_id or keyword in c.get('title', '')]
print(len(matches))
" 2>/dev/null || echo "0")

if [ "$DEACT_CITATION_MATCHES" = "0" ]; then
  pass "Deactivated document not cited in query results"
else
  fail "Deactivated document not cited in query results" "found $DEACT_CITATION_MATCHES citations still referencing deactivated doc"
fi

# ── Summary ──────────────────────────────────────────────

echo ""
echo "════════════════════════════════════════════════"
echo "  E2E Web Integration Test Results: $PASS/$TOTAL passed"
if [ "$FAIL" -gt 0 ]; then
  echo "  ❌ $FAIL test(s) FAILED"
  echo "════════════════════════════════════════════════"
  exit 1
else
  echo "  ✅ All tests passed!"
  echo "════════════════════════════════════════════════"
  exit 0
fi
