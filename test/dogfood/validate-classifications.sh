#!/usr/bin/env bash
# Validates that the rules engine classifies each workload correctly.
# Requires: jq, kubectl
set -euo pipefail

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

# Check each expected workload/pattern pair.
check_pattern() {
  local workload="$1"
  local want="$2"
  local got
  got=$(echo "${RECS}" | jq -r --arg name "${workload}" \
    '.data[] | select(.target.name == $name) | .pattern' | head -1)

  if [ -z "${got}" ] || [ "${got}" = "null" ]; then
    echo "FAIL: ${workload} — no recommendation found"
    FAIL=$((FAIL + 1))
  elif [ "${got}" = "${want}" ]; then
    echo "PASS: ${workload} — pattern=${got}"
    PASS=$((PASS + 1))
  else
    echo "FAIL: ${workload} — expected=${want}, got=${got}"
    FAIL=$((FAIL + 1))
  fi
}

check_pattern "nginx-overprovisioned" "steady"
check_pattern "api-bursty" "burstable"
check_pattern "cronjob-batch" "batch"
check_pattern "idle-dev" "idle"

echo ""
echo "Results: ${PASS} passed, ${FAIL} failed"

if [ "${FAIL}" -gt 0 ]; then
  exit 1
fi
