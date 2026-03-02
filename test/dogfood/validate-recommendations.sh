#!/usr/bin/env bash
# Validates recommendation sanity: no negative values, no over-sizing,
# and best-effort pods have no recommendations.
# Requires: jq, kubectl
set -euo pipefail

NS="sage-dogfood"
PASS=0
FAIL=0

# Port-forward to sage-server.
SERVER_NS="${SERVER_NS:-default}"
kubectl port-forward -n "${SERVER_NS}" svc/k8s-sage-server 8080:8080 &
PF_PID=$!
trap "kill ${PF_PID} 2>/dev/null || true" EXIT
sleep 2

echo "Fetching recommendations..."
RECS=$(curl -s http://localhost:8080/api/v1/recommendations)

# Check: no recommendedRequest is <= 0
ZERO_RECS=$(echo "${RECS}" | jq '[.data[] | select(.recommendedRequest <= 0)] | length')
if [[ "${ZERO_RECS}" -eq 0 ]]; then
  echo "PASS: no recommendedRequest <= 0"
  PASS=$((PASS + 1))
else
  echo "FAIL: ${ZERO_RECS} recommendations have recommendedRequest <= 0"
  FAIL=$((FAIL + 1))
fi

# Check: no recommendedRequest exceeds currentRequest
OVER_RECS=$(echo "${RECS}" | jq '[.data[] | select(.recommendedRequest > .currentRequest)] | length')
if [[ "${OVER_RECS}" -eq 0 ]]; then
  echo "PASS: no recommendedRequest exceeds currentRequest"
  PASS=$((PASS + 1))
else
  echo "FAIL: ${OVER_RECS} recommendations have recommendedRequest > currentRequest"
  FAIL=$((FAIL + 1))
fi

# Check: all estimatedSavings >= 0
NEG_SAVINGS=$(echo "${RECS}" | jq '[.data[] | select(.estimatedSavings < 0)] | length')
if [[ "${NEG_SAVINGS}" -eq 0 ]]; then
  echo "PASS: all estimatedSavings >= 0"
  PASS=$((PASS + 1))
else
  echo "FAIL: ${NEG_SAVINGS} recommendations have negative estimatedSavings"
  FAIL=$((FAIL + 1))
fi

# Check: best-effort has no recommendation (no resource request baseline)
BE_RECS=$(echo "${RECS}" | jq '[.data[] | select(.target.name == "best-effort")] | length')
if [[ "${BE_RECS}" -eq 0 ]]; then
  echo "PASS: best-effort has no recommendations (expected)"
  PASS=$((PASS + 1))
else
  echo "FAIL: best-effort has ${BE_RECS} recommendations (expected 0)"
  FAIL=$((FAIL + 1))
fi

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"

if [[ "${FAIL}" -gt 0 ]]; then
  exit 1
fi
