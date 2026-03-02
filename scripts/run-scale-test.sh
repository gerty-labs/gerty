#!/usr/bin/env bash
set -euo pipefail

echo "Running k8s-sage scale tests..."
echo ""

go test -v -count=1 -timeout 600s ./test/scale/...
