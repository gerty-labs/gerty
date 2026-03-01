#!/usr/bin/env bash
# Validates that the rules engine classifies each workload correctly.
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

# Expected workload → pattern mapping.
declare -A EXPECTED=(
  ["nginx-overprovisioned"]="steady"
  ["api-bursty"]="burstable"
  ["cronjob-batch"]="batch"
  ["idle-dev"]="idle"
)

for WORKLOAD in "${!EXPECTED[@]}"; do
  WANT="${EXPECTED[$WORKLOAD]}"
  # Extract pattern for this workload from recommendations.
  GOT=$(echo "${RECS}" | jq -r --arg name "${WORKLOAD}" \
    '.data[] | select(.target.name == $name) | .pattern' | head -1)

  if [ -z "${GOT}" ] || [ "${GOT}" = "null" ]; then
    echo "FAIL: ${WORKLOAD} — no recommendation found"
    FAIL=$((FAIL + 1))
  elif [ "${GOT}" = "${WANT}" ]; then
    echo "PASS: ${WORKLOAD} — pattern=${GOT}"
    PASS=$((PASS + 1))
  else
    echo "FAIL: ${WORKLOAD} — expected=${WANT}, got=${GOT}"
    FAIL=$((FAIL + 1))
  fi
done

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"

if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
