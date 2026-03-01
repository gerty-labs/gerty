#!/usr/bin/env bash
# Tears down the dogfood environment: namespace, helm release, and kind cluster.
set -euo pipefail

echo "Deleting dogfood namespace..."
kubectl delete namespace sage-dogfood --ignore-not-found

echo "Uninstalling helm release..."
helm uninstall k8s-sage --ignore-not-found 2>/dev/null || true

echo "Deleting kind cluster..."
kind delete cluster --name k8s-sage-dev 2>/dev/null || true

echo ""
echo "Teardown complete."
