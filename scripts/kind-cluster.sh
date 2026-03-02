#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="${1:-k8s-sage-dev}"

if ! command -v kind &> /dev/null; then
    echo "ERROR: kind is not installed. Install from https://kind.sigs.k8s.io/"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "ERROR: kubectl is not installed."
    exit 1
fi

# Create cluster if it doesn't exist.
if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
    echo "Cluster ${CLUSTER_NAME} already exists"
else
    echo "Creating kind cluster ${CLUSTER_NAME}..."
    kind create cluster --name "${CLUSTER_NAME}" --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
  - role: worker
EOF
fi

echo ""
echo "Cluster ${CLUSTER_NAME} is ready"
kubectl cluster-info --context "kind-${CLUSTER_NAME}"

# Build and load images if Docker is available.
if command -v docker &> /dev/null; then
    echo ""
    echo "Building container images..."
    REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

    docker build -t k8s-sage-agent:latest -f "${REPO_ROOT}/Dockerfile.agent" "${REPO_ROOT}"
    docker build -t k8s-sage-server:latest -f "${REPO_ROOT}/Dockerfile.server" "${REPO_ROOT}"

    echo ""
    echo "Loading images into kind cluster..."
    kind load docker-image k8s-sage-agent:latest --name "${CLUSTER_NAME}"
    kind load docker-image k8s-sage-server:latest --name "${CLUSTER_NAME}"

    echo ""
    echo "Images loaded. Deploy with:"
    echo "  helm install k8s-sage ${REPO_ROOT}/deploy/helm/k8s-sage/ --set image.pullPolicy=Never"
else
    echo ""
    echo "Docker not available — skipping image build/load."
    echo "Build images manually and load with: kind load docker-image <image> --name ${CLUSTER_NAME}"
fi
