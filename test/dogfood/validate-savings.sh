#!/usr/bin/env bash
# Validates that estimatedSavings is consistent with currentRequest - recommendedRequest.
# Allows +-1 tolerance for ceiling rounding.
# Requires: jq, kubectl
set -euo pipefail

NS="sage-dogfood"
PASS=0
FAIL=0

# Port-forward to sage-server.
kubectl port-forward -n "${NS}" svc/k8s-sage-server 8080:8080 &
PF_PID=$!
trap "kill ${PF_PID} 2>/dev/null || true" EXIT
sleep 2

echo "Fetching recommendations..."
RECS=$(curl -s http://localhost:8080/api/v1/recommendations)

COUNT=$(echo "${RECS}" | jq '.data | length')
echo "Checking ${COUNT} recommendations..."

# For each recommendation, verify |currentRequest - recommendedRequest - estimatedSavings| <= 1
MISMATCHES=$(echo "${RECS}" | jq '[
  .data[] |
  {
    target: .target.name,
    resource: .resource,
    diff: ((.currentRequest - .recommendedRequest - .estimatedSavings) | fabs)
  } |
  select(.diff > 1)
] | length')

if [ "${MISMATCHES}" -eq 0 ]; then
  echo "PASS: all savings calculations are consistent (within +-1 rounding)"
  PASS=$((PASS + 1))
else
  echo "FAIL: ${MISMATCHES} recommendations have inconsistent savings"
  echo "${RECS}" | jq '[
    .data[] |
    {
      target: .target.name,
      resource: .resource,
      current: .currentRequest,
      recommended: .recommendedRequest,
      savings: .estimatedSavings,
      diff: ((.currentRequest - .recommendedRequest - .estimatedSavings) | fabs)
    } |
    select(.diff > 1)
  ]'
  FAIL=$((FAIL + 1))
fi

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"

if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
