#!/bin/bash
# Launch llama.cpp server with k8s-sage model.
#
# Usage:
#   ./run_llama_cpp.sh                           # default model path
#   ./run_llama_cpp.sh /path/to/model.gguf       # custom model path
#   LLAMA_THREADS=4 ./run_llama_cpp.sh            # override thread count

set -euo pipefail

MODEL_PATH="${1:-/models/k8s-sage-q4.gguf}"
CTX_SIZE="${LLAMA_CTX_SIZE:-1024}"
THREADS="${LLAMA_THREADS:-2}"
PORT="${LLAMA_PORT:-8080}"

if [ ! -f "$MODEL_PATH" ]; then
    echo "Error: model file not found: $MODEL_PATH" >&2
    echo "Build the model first with: python ml/training/merge_and_quantize.py --gguf" >&2
    exit 1
fi

echo "Starting llama.cpp server..."
echo "  Model:   $MODEL_PATH"
echo "  Context: $CTX_SIZE"
echo "  Threads: $THREADS"
echo "  Port:    $PORT"

exec llama-server \
    --model "$MODEL_PATH" \
    --ctx-size "$CTX_SIZE" \
    --threads "$THREADS" \
    --port "$PORT"
