# k8s-sage Model Design

## Why a Specialist Small Language Model

General-purpose LLMs know about Kubernetes from pretraining but lack three critical capabilities for resource efficiency:

1. **Metric interpretation**: Given a CPU time series (P50=120m, P95=340m, P99=890m), recognise that this is a burstable workload with periodic spikes — not a steady workload with outliers. A general model treats these as just numbers.

2. **Runtime awareness**: Know that a JVM workload using 2.8Gi of 3Gi memory is healthy (GC heap reservation), not wasteful. A general model sees "93% memory utilisation" and either panics or ignores it.

3. **Operational context**: Understand that DaemonSets should be sized differently from Deployments, that init containers skew steady-state metrics, and that CrashLoopBackOff produces metric spikes that shouldn't inform right-sizing.

A 3.8B parameter model fine-tuned on 12k domain-specific instruction pairs will outperform a 70B general model prompted with the same questions — at 1/20th the cost and latency, running CPU-only inside the cluster.

---

## Base Model: Phi-3 Mini 4K Instruct (3.8B)

### Why Phi-3 Mini

| Criterion | Phi-3 Mini | TinyLlama 1.1B | Qwen2.5 3B | Llama 3.1 8B |
|-----------|-----------|----------------|------------|--------------|
| Parameters | 3.8B | 1.1B | 3B | 8B |
| Q4 size | ~2.2GB | ~0.7GB | ~1.8GB | ~4.5GB |
| CPU RAM | ~3GB | ~1GB | ~2.5GB | ~6GB |
| Reasoning | Strong for size | Weak | Moderate | Strong |
| Licence | MIT | Apache 2.0 | Apache 2.0 | Llama 3.1 Community |
| Instruction-tuned | Yes (4K context) | Yes | Yes | Yes |
| GGUF support | Yes | Yes | Yes | Yes |

**Decision**: Phi-3 Mini is the best balance of reasoning quality, memory footprint, and licence permissiveness.

- Strong enough to understand metric patterns and generate nuanced explanations
- Small enough to run CPU-only with ~3GB RAM (fits in a single pod)
- MIT licence — no restrictions on commercial use or modification
- Well-supported in the GGUF/Ollama ecosystem

### Why Not Bigger

- **Llama 3.1 8B**: 2x the RAM, only marginally better at our domain task after fine-tuning. The bottleneck is training data quality, not model capacity.
- **GPT-4/Claude API**: Violates the "data never leaves the cluster" principle. Also expensive at ~$0.03/request vs. ~$0 for local inference.
- **Llama 3.1 70B**: Absurd for a sidecar use case. Would require GPU.

### Why Not Smaller

- **TinyLlama 1.1B**: Tested in initial experiments. Struggles with multi-step reasoning ("this is a JVM workload, therefore GC reservations apply, therefore memory isn't waste"). Fine for simple right-sizing but can't explain its reasoning convincingly.
- At 3.8B, Phi-3 Mini is the sweet spot where the model can both compute recommendations and explain them.

---

## Fine-Tuning: LoRA

### Why LoRA

Full fine-tuning of a 3.8B model requires ~30GB VRAM (bf16) and risks catastrophic forgetting of the model's general reasoning. LoRA (Low-Rank Adaptation) fine-tunes only a small number of adapter parameters (~0.5% of total), preserving the base model's capabilities while injecting domain knowledge.

### Configuration

```
Method:         LoRA (via HuggingFace PEFT)
Base model:     microsoft/Phi-3-mini-4k-instruct
LoRA rank (r):  16
LoRA alpha:     32 (alpha/rank = 2, standard scaling)
Dropout:        0.05
Target modules: q_proj, k_proj, v_proj, o_proj (attention layers only)
```

#### Why These Hyperparameters

- **Rank 16**: Captures enough domain-specific variation without overfitting on 12k examples. Rank 8 was tested and showed slightly worse performance on runtime-specific questions. Rank 32 showed no improvement.
- **Alpha 32**: Standard 2x scaling factor. Controls how much the LoRA updates influence the base model.
- **Attention layers only**: The attention mechanism is where the model learns which parts of the input matter. For our task (interpreting metric context to generate recommendations), attention is the key bottleneck. MLP layers are less critical and keeping them frozen reduces training time.
- **Dropout 0.05**: Light regularisation. With 12k examples, overfitting risk is moderate.

### Training Configuration

```
Learning rate:        2e-4 (with cosine scheduler, warmup 100 steps)
Batch size:           4 (per device)
Gradient accumulation: 4 steps (effective batch size: 16)
Epochs:               3
Max sequence length:  2048 tokens
Optimizer:            AdamW (weight decay 0.01)
Precision:            bf16 mixed precision
Hardware:             Single GPU (RTX 3090 24GB / A100 40GB)
Estimated time:       4-8 hours (RTX 3090) / 2-4 hours (A100)
```

#### Why 3 Epochs

- Epoch 1: Model learns the instruction format and output structure
- Epoch 2: Model internalises domain-specific patterns (JVM != Go, steady != burstable)
- Epoch 3: Model refines reasoning quality and numerical precision
- Epoch 4+: Diminishing returns, risk of overfitting on training phrasings

Validation loss is monitored per epoch. Training stops early if validation loss increases.

---

## Post-Training Pipeline

### 1. Merge LoRA Adapters

```python
from peft import PeftModel
from transformers import AutoModelForCausalLM

base = AutoModelForCausalLM.from_pretrained("microsoft/Phi-3-mini-4k-instruct")
model = PeftModel.from_pretrained(base, "output/k8s-sage-lora")
merged = model.merge_and_unload()
merged.save_pretrained("output/k8s-sage-merged")
```

### 2. Quantise to GGUF Q4_K_M

```bash
# Using llama.cpp's convert script
python convert_hf_to_gguf.py output/k8s-sage-merged --outtype q4_k_m --outfile k8s-sage-q4.gguf
```

**Why Q4_K_M**:
- ~2.2GB file size (vs. ~7.6GB fp16)
- Minimal quality loss compared to fp16 (~1-2% on benchmarks)
- The "K_M" variant preserves important layers at higher precision
- CPU inference speed: ~10-15 tokens/sec on modern hardware

### 3. Ollama Modelfile

```dockerfile
FROM ./k8s-sage-q4.gguf

PARAMETER temperature 0.3
PARAMETER top_p 0.9
PARAMETER num_predict 1024
PARAMETER stop "<|end|>"

SYSTEM """You are k8s-sage, a Kubernetes resource efficiency specialist.
You analyse workload metrics and provide actionable right-sizing recommendations.
Be specific about numbers, explain your reasoning, and flag risks.
When you see high memory usage in JVM workloads, check for -Xmx before recommending reductions.
When you see low CPU usage, distinguish between idle workloads and event-driven workloads.
Always consider the workload pattern (steady, burstable, batch, idle) when making recommendations."""
```

The Modelfile system prompt extends the canonical training prompt (defined in [TRAINING_DATA.md](TRAINING_DATA.md)) with inference-time operational instructions. The base prompt must remain consistent between training and serving.

**Why temperature 0.3**: We want consistent, reproducible recommendations. Higher temperature introduces randomness that undermines trust. 0.3 allows slight variation in phrasing while keeping numbers stable.

---

## Serving Architecture

### Deployment

The model runs as a single Deployment via Ollama — NOT per-node:

```yaml
# Helm values (slm section)
slm:
  enabled: false  # Opt-in, not default
  image: ollama/ollama:latest
  model: k8s-sage:q4
  resources:
    requests:
      cpu: "1"
      memory: "3Gi"
    limits:
      cpu: "2"
      memory: "4Gi"
```

### Resource Budget

| Component | CPU | Memory | Justification |
|-----------|-----|--------|---------------|
| Ollama process | 100-200m idle | ~200Mi | Ollama server overhead |
| Model loaded | 0 idle / 1-2 cores during inference | ~2.5Gi | Q4_K_M model in memory |
| **Total** | **1 core average** | **~3Gi** | Fits in a single pod |

### Invocation Pattern

The SLM is NOT invoked on every metrics scrape — that would be absurdly wasteful. It is invoked:

1. **On-demand** via CLI: `sage report --explain` triggers SLM analysis
2. **On schedule**: Daily/hourly cluster report with natural language insights
3. **On API request**: `POST /api/v1/explain` sends a workload to the SLM

Typical inference: ~5-10 seconds for a full workload analysis (input + output ~1500 tokens).

---

## Evaluation Plan

### Benchmark Tasks

Create a held-out test set of 500 pairs (not used in training) covering:

1. **Right-sizing accuracy** (200 pairs): Given metrics, does the model recommend values within 20% of the rules engine output? The model should match or improve on rules engine recommendations.

2. **Pattern classification** (100 pairs): Does the model correctly identify steady/burstable/batch/idle patterns from metric descriptions?

3. **Runtime-specific reasoning** (100 pairs): Does the model correctly handle JVM, Go, Python, Node.js workloads? Key test: does it avoid recommending memory reduction for JVM workloads with high -Xmx?

4. **Edge case handling** (100 pairs): CrashLoopBackOff, init containers, sidecars, DaemonSets, StatefulSets, BestEffort QoS.

### Comparison Targets

| Model | Expected Performance | Purpose |
|-------|---------------------|---------|
| k8s-sage rules engine only | ~75% accuracy | Baseline — does the model add value over deterministic rules? |
| Base Phi-3 Mini (no fine-tune) | ~40% accuracy | Baseline — how much does fine-tuning help? |
| k8s-sage SLM (fine-tuned) | ~85% accuracy | Target |
| GPT-4 (prompted) | ~60-75% (to be benchmarked) | Ceiling reference — expensive but general |

"Accuracy" means: recommendation is within 20% of the ground truth, pattern classification is correct, and no safety violations (recommending below P99 for memory, recommending 0, etc.).

### Safety Checks

Every model output is validated against the same safety invariants as the rules engine:
- Memory recommendation >= P99 working set * 1.10
- CPU recommendation >= P95 * headroom (pattern-dependent)
- No recommendation of 0 for any resource
- Upsize recommendations (exceeding current request) are valid for under-provisioned workloads (e.g., OOMKill, CPU throttling) but are evaluated separately from right-sizing accuracy. The accuracy metric only covers downsizing scenarios

If the model violates safety invariants, the rules engine output is used instead (graceful degradation).

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Model hallucinates specific numbers | Wrong recommendations | Safety invariant check against rules engine output; reject if divergent by >50% |
| Overfitting to training data phrasing | Brittle on novel inputs | Diverse training data, validation set monitoring, temperature 0.3 |
| Model too large for customer environments | Can't deploy | If target environment has <4GB RAM available for the SLM pod, fall back to TinyLlama 1.1B with Q4 quantisation (~1GB RAM). If <1GB available, use rules-engine-only mode |
| Quantisation degrades quality | Worse than base model | Benchmark Q4 vs fp16 on test set; if >5% degradation, use Q5 |
| Training data contains errors | Model learns wrong patterns | Human review of all expert pairs, automated validation of metric plausibility |
