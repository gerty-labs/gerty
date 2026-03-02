#!/bin/bash
# Merge LoRA adapter, quantize to GGUF, and evaluate.
#
# Run after train.sh completes.
# Produces: output/k8s-sage-q4.gguf (~1.8GB)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

ADAPTER_PATH="${ADAPTER_PATH:-output/k8s-sage-lora}"
MERGED_PATH="output/k8s-sage-merged"
DATASET="ml/dataset/data/training_data.jsonl"

# Check adapter exists
if [[ ! -d "$ADAPTER_PATH" ]]; then
    echo "ERROR: LoRA adapter not found at $ADAPTER_PATH" >&2
    echo "Run ./scripts/train.sh first."
    exit 1
fi

echo "=== Step 1: Merge LoRA + Quantize ==="
python3 ml/training/merge_and_quantize.py \
    --adapter-path "$ADAPTER_PATH" \
    --output-dir "$MERGED_PATH"

echo ""
echo "=== Step 2: Evaluate ==="
# If llama.cpp server is running, use it; otherwise use HF model
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo "Using llama.cpp server at localhost:8080"
    python3 ml/training/eval.py \
        --llama-cpp-url http://localhost:8080 \
        --test-file "$DATASET"
else
    echo "Using merged HF model for evaluation"
    python3 ml/training/eval.py \
        --model-path "$MERGED_PATH" \
        --test-file "$DATASET"
fi

echo ""
echo "=== Complete ==="
echo "Merged model: $MERGED_PATH"
echo ""
echo "Next steps:"
echo "  1. Copy GGUF to cluster: kubectl cp output/k8s-sage-q4.gguf <pod>:/models/"
echo "  2. Enable SLM: helm upgrade k8s-sage deploy/helm/k8s-sage --set slm.enabled=true"
echo "  3. Verify: python3 ml/serving/test_inference.py"
