#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${1:-k8s-sage-dev}"

if ! command -v kind &> /dev/null; then
    echo "ERROR: kind is not installed."
    exit 1
fi

if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster ${CLUSTER_NAME} already exists"
else
    kind create cluster --name "${CLUSTER_NAME}" --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
EOF
fi

echo "Cluster ${CLUSTER_NAME} is ready"
kubectl cluster-info --context "kind-${CLUSTER_NAME}"
