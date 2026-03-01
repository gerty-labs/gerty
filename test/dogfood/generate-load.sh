#!/usr/bin/env bash
# Generates load against bursty workloads to create realistic traffic patterns.
# Requires: hey (https://github.com/rakyll/hey) or vegeta
#
# Usage: ./generate-load.sh [duration_seconds]
set -euo pipefail

DURATION="${1:-300}"
NS="sage-dogfood"

# Determine load tool.
LOAD_TOOL=""
if command -v hey &> /dev/null; then
    LOAD_TOOL="hey"
elif command -v vegeta &> /dev/null; then
    LOAD_TOOL="vegeta"
else
    echo "Neither 'hey' nor 'vegeta' found."
    echo "Install hey: go install github.com/rakyll/hey@latest"
    echo ""
    echo "Falling back to kubectl port-forward + curl loop..."
    LOAD_TOOL="curl"
fi

# Get the api-bursty service ClusterIP for in-cluster load gen.
echo "Generating load against api-bursty service for ${DURATION}s..."
echo "Load tool: ${LOAD_TOOL}"
echo ""

# Use port-forward to reach the service.
kubectl port-forward -n "${NS}" svc/api-bursty 8081:80 &
PF_PID=$!
trap "kill ${PF_PID} 2>/dev/null || true" EXIT

sleep 2

TARGET="http://localhost:8081/"

case "${LOAD_TOOL}" in
    hey)
        echo "=== Burst 1: High concurrency spike ==="
        hey -z "${DURATION}s" -c 50 -q 100 "${TARGET}"
        ;;
    vegeta)
        echo "=== Burst 1: Sustained load ==="
        echo "GET ${TARGET}" | vegeta attack -duration="${DURATION}s" -rate=200/s | vegeta report
        ;;
    curl)
        echo "=== Burst: curl loop (${DURATION}s) ==="
        END=$((SECONDS + DURATION))
        REQUESTS=0
        while [ $SECONDS -lt $END ]; do
            # Fire 10 concurrent requests.
            for i in $(seq 1 10); do
                curl -s -o /dev/null "${TARGET}" &
            done
            wait
            REQUESTS=$((REQUESTS + 10))
            # Brief pause between bursts.
            sleep 0.5
        done
        echo "Completed ${REQUESTS} requests in ${DURATION}s"
        ;;
esac

echo ""
echo "Load generation complete."
