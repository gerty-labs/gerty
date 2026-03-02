#!/bin/bash
# Train the k8s-sage SLM (Jamba 3B QLoRA)
#
# Run on Threadripper + dual 3090 (24GB each).
# Expected: 3-6 hours for 3 epochs on ~7,000 pairs.
#
# Prerequisites:
#   pip install -e ".[train]"
#   Dataset exists at ml/dataset/data/training_data.jsonl

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

DATASET="ml/dataset/data/training_data.jsonl"
CONFIG="ml/training/configs/default.yaml"

# Check dataset exists
if [ ! -f "$DATASET" ]; then
    echo "ERROR: Dataset not found at $DATASET"
    echo ""
    echo "Generate it first:"
    echo "  python3 ml/dataset/generate_synthetic.py"
    echo "  python3 ml/dataset/format_instruct.py"
    exit 1
fi

PAIRS=$(wc -l < "$DATASET" | tr -d ' ')
echo "=== k8s-sage Training ==="
echo "Dataset: $DATASET ($PAIRS pairs)"
echo "Config:  $CONFIG"
echo ""

# Optional: dry-run first
if [ "${DRY_RUN:-}" = "1" ]; then
    echo "--- Dry Run ---"
    python3 ml/training/finetune_lora.py --config "$CONFIG" --dry-run
    exit 0
fi

# Install deps if needed
if ! python3 -c "import transformers" 2>/dev/null; then
    echo "Installing training dependencies..."
    pip install -e ".[train]"
fi

echo "Starting training..."
python3 ml/training/finetune_lora.py --config "$CONFIG"

echo ""
echo "=== Training Complete ==="
echo "LoRA adapter saved to: output/k8s-sage-lora"
echo ""
echo "Next steps:"
echo "  ./scripts/eval_and_deploy.sh"
