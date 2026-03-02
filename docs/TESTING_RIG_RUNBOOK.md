# Testing Rig Runbook

Step-by-step for running training, evaluation, and dogfooding on the Threadripper + dual RTX 3090.

---

## Prerequisites

- Python 3.11+
- CUDA 12.x + PyTorch with GPU support
- Go 1.22+
- Docker + Kind (for dogfooding)
- Helm 3
- ~20GB free disk (model weights + training artifacts)

---

## 1. Clone and Setup

```bash
git clone <repo-url> k8s-sage && cd k8s-sage

# Install Python training deps
pip install -e ".[train]"

# Verify Go builds
make build
make test
```

---

## 2. Verify Dataset

```bash
# Check dataset exists and has expected size
wc -l ml/dataset/data/training_data.jsonl
# Expected: ~6,982 lines

# Optional: regenerate from scratch
python3 ml/dataset/generate_synthetic.py --count 4500 --seed 42
python3 ml/dataset/format_instruct.py --output ml/dataset/data/training_data.jsonl
```

---

## 3. Dry-Run Training

Validates config, dataset loading, model parameter counts — no GPU needed.

```bash
DRY_RUN=1 ./scripts/train.sh
```

Expected output:
- ~6,982 training examples
- Source distribution (synthetic ~3,500, stackoverflow ~2,400, etc.)
- Training config summary (3 epochs, batch 4, grad accum 4)
- Trainable parameters: ~8M (LoRA) out of ~3B total

---

## 4. Train

```bash
./scripts/train.sh
```

- **Duration**: 3-6 hours on dual 3090
- **Output**: `output/k8s-sage-lora/` (LoRA adapter ~30MB)
- **Monitoring**: WandB dashboard (or `--no-wandb` to disable)
- **VRAM**: ~20GB per GPU with 4-bit quantization

If training fails with OOM:
```bash
# Reduce batch size in ml/training/configs/default.yaml
# per_device_train_batch_size: 2 (instead of 4)
# gradient_accumulation_steps: 8 (instead of 4, to keep effective batch)
```

---

## 5. Merge + Quantize

```bash
./scripts/eval_and_deploy.sh
```

This runs:
1. Merge LoRA adapter into base model → `output/k8s-sage-merged/`
2. Convert to GGUF Q4_K_M → `output/k8s-sage-q4.gguf` (~1.8GB)
3. Evaluate against held-out test set

**Evaluation targets** (from MODEL_DESIGN.md):
- Right-sizing accuracy: >82% within 20% of ground truth
- Pattern classification: >85%
- Safety invariant compliance: 100% (no violations)

---

## 6. Smoke Test Inference

Start llama.cpp server locally:

```bash
./ml/serving/run_llama_cpp.sh output/k8s-sage-q4.gguf
```

In another terminal:

```bash
python3 ml/serving/test_inference.py --url http://localhost:8080
```

Expected: 5 scenarios pass, <5s latency each, valid JSON responses.

---

## 7. Deploy to Kind Cluster

```bash
# Build container images with latest Go code
make docker-build

# Create Kind cluster (if not running)
make dev-cluster

# Deploy with SLM disabled first (validates L1)
make dev-deploy

# Wait for pods to be ready
kubectl -n default get pods -w

# Setup dogfood workloads
make setup-workloads
make generate-load
```

---

## 8. Dogfood v2 — L1 Only

Wait 30-60 minutes for metric collection, then:

```bash
make validate
```

Check:
- [ ] Classification accuracy maintained (6/6 from v1)
- [ ] No aggressive reductions (floors at 50m CPU, 64Mi memory)
- [ ] Memory leak workload flagged as anomalous
- [ ] No duplicate per-replica entries

---

## 8.5. Scale Test

Run before dogfood v2 to establish baseline performance:

```bash
make test-scale
```

Check:
- [ ] All scale points up to 100 nodes complete without drops
- [ ] L1 full cycle (ingest + report + analyze) < 10s at 100 nodes
- [ ] L2 estimated cycle time documented (owners x 4s) — note where it exceeds 5-min budget
- [ ] Ingest latency < 1s per node report at 200 nodes
- [ ] Report build < 500ms at 200 nodes
- [ ] Memory stays under 512MB at 200 nodes
- [ ] Graceful degradation at 500+ nodes (logged capacity warnings)
- [ ] Replica guidance table produced with clear thresholds

---

## 9. Dogfood v3 — With L2 (SLM)

Copy the GGUF model into the cluster and enable L2:

```bash
# Load model into Kind node
docker cp output/k8s-sage-q4.gguf k8s-sage-dev-worker:/tmp/
kubectl exec -it <slm-pod> -- cp /tmp/k8s-sage-q4.gguf /models/

# Or: create a PV with the model pre-loaded
# (see deploy/helm/k8s-sage/values.yaml slm.persistence)

# Enable SLM
helm upgrade k8s-sage deploy/helm/k8s-sage --set slm.enabled=true

# Wait for SLM pod ready
kubectl get pods -l app.kubernetes.io/component=slm -w
```

After SLM is ready, wait for an analysis cycle (5 minutes), then:

```bash
make validate
```

Check L2-specific:
- [ ] Recommendations include L2 explanations (longer, more detailed)
- [ ] Confidence scores reflect L2 analysis
- [ ] Safety invariants still pass (L1 as floor)
- [ ] No L2 timeout errors in server logs

---

## Troubleshooting

### Training OOM
Reduce `per_device_train_batch_size` to 2, increase `gradient_accumulation_steps` to 8.

### llama.cpp won't load model
Check GGUF format: `file output/k8s-sage-q4.gguf` should show GGUF header. If conversion failed, re-run `merge_and_quantize.py` with verbose output.

### SLM pod CrashLoopBackOff
Check logs: `kubectl logs -l app.kubernetes.io/component=slm`. Common issues: model file not found at `/models/k8s-sage-q4.gguf`, insufficient memory (needs 2.5Gi).

### L2 falling back to L1 on every request
Check SLM health: `kubectl exec <server-pod> -- curl -s http://k8s-sage-slm:8080/health`. Check server logs for SLM timeout or parse errors.
