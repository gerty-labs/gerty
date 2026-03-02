#!/usr/bin/env bash
set -euo pipefail

echo "Setting up k8s-sage development environment..."

# Check Go version
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed. Install Go 1.22+ first." >&2
    exit 1
fi

echo "Go version: $(go version)"

# Install dependencies
go mod tidy

# Install test tools
go install github.com/stretchr/testify@latest 2>/dev/null || true

echo "Dev environment ready."
