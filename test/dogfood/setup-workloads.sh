#!/usr/bin/env bash
# Deploys all synthetic workloads from the manifests directory.
# See TESTING_VALIDATION.md for the full workload matrix.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NS="sage-dogfood"

echo "Creating namespace ${NS}..."
kubectl create namespace "${NS}" --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "Deploying synthetic workloads from manifests..."
kubectl apply -n "${NS}" -f "${SCRIPT_DIR}/manifests/"

echo ""
echo "All workloads deployed to namespace ${NS}."
echo "Waiting for pods to become ready..."
kubectl wait --for=condition=ready pod -l app --timeout=120s -n "${NS}" 2>/dev/null || true

echo ""
echo "Pod status:"
kubectl get pods -n "${NS}" -o wide
