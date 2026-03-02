# k8s-sage Model Design

## Architecture Overview: Two-Layer Intelligence

k8s-sage uses a two-layer architecture. L1 handles safety and real-time guarantees. L2 handles intelligent right-sizing with natural language explanations.

```
┌─────────────────────────────────────────────────────────┐
│  L2: Fine-tuned Hybrid Mamba SLM                        │
│  Jamba Reasoning 3B — near-real-time analysis            │
│  Pattern classification + right-sizing + explanations    │
│  Latency: 3-6s │ Footprint: ~2GB                        │
├─────────────────────────────────────────────────────────┤
│  L1: Rules Engine                                       │
│  Go — every workload, every cycle                       │
│  Safety floor, percentile math, hard constraints        │
│  Latency: <1ms │ Footprint: 0 (part of server binary)  │
└─────────────────────────────────────────────────────────┘
```

### Why Two Layers

- **L1 (Rules engine)** runs on every workload, every cycle, in <1ms. It's the real-time production path — percentile-based right-sizing, safety checks, alert thresholds. This is always on, cannot be disabled. L1 outputs are the safety floor that L2 can improve upon but never violate.

- **L2 (Hybrid Mamba SLM)** runs near-real-time, processing workloads from a priority queue each cycle. Unlike a pure transformer, the hybrid Mamba architecture is fast enough on CPU to approach real-time coverage — a 3B hybrid Mamba model generates 25-35 tokens/second on CPU, compared to 5-8 tokens/second for a 4B transformer. This means L2 can process a 100-token structured recommendation in ~3-4 seconds, covering 50+ workloads in a single 5-minute push cycle.

### Why Not Three Layers (XGBoost Middle Layer)

An earlier design included an XGBoost/LightGBM classifier between L1 and L2 for microsecond inference on tabular features. We rejected this because:

1. **The speed problem is solved by architecture choice.** The three-layer design was necessitated by slow transformer inference (12-20s per workload). Hybrid Mamba at 25-35 tok/s eliminates this bottleneck.
2. **XGBoost can't reason about novel combinations.** A JVM DaemonSet with init containers requires understanding what those concepts mean together — not just a learned correlation from training features.
3. **One model to train and maintain** is simpler than two models with different training pipelines.
4. **XGBoost trained on rules-engine-generated labels** would largely learn to mimic L1, adding marginal value.

### What Each Layer Does

| Capability | L1 Rules | L2 Hybrid Mamba SLM |
|-----------|----------|---------------------|
| Percentile-based right-sizing | ✓ | ✓ (improved) |
| Pattern classification | Basic threshold | ✓ (learned) |
| Runtime-aware adjustment | | ✓ |
| Confidence scoring | | ✓ |
| Safety invariant checks | ✓ | |
| Natural language explanations | | ✓ |
| Edge case reasoning | | ✓ |

---

## Why a Hybrid Mamba Architecture

### The Problem with Pure Transformers

The initial design targeted Qwen3-4B, a pure transformer. At Q4_K_M quantisation on CPU, it generates 5-8 tokens/second. A structured JSON recommendation (~100 tokens) takes 12-20 seconds. For 50 workloads on a 5-minute push cycle, that's ~12 minutes — the model can never keep up.

### How Hybrid Mamba Solves This

Hybrid Mamba-Transformer models (Mamba layers for efficient sequence processing, sparse attention layers for precise reasoning) deliver fundamentally faster inference:

- **No KV cache**: Mamba layers maintain a fixed-size state summary instead of storing every previous token. Memory is constant regardless of sequence length.
- **Linear inference scaling**: Unlike transformers' quadratic attention, Mamba scales linearly.
- **4-7x faster generation**: Hybrid Mamba 3B models generate 25-35 tokens/second on CPU, compared to 5-8 tok/s for a pure transformer 4B.

At 30 tok/s, a 100-token structured recommendation takes ~3 seconds. 50 workloads takes ~2.5 minutes. That fits in a 5-minute push cycle. The architectural bottleneck that forced the three-layer design disappears.

### Why Specialist Fine-Tuning Still Matters

General-purpose LLMs know about Kubernetes from pretraining but lack three critical capabilities:

1. **Metric interpretation**: Given a CPU time series (P50=120m, P95=340m, P99=890m), recognise a burstable workload with periodic spikes — not a steady workload with outliers.

2. **Runtime awareness**: Know that a JVM workload using 2.8Gi of 3Gi memory is healthy (GC heap reservation), not wasteful. A general model sees "93% memory utilisation" and panics.

3. **Operational context**: Understand that DaemonSets should be sized differently from Deployments, that init containers skew steady-state metrics, and that CrashLoopBackOff produces metric spikes that shouldn't inform right-sizing.

---

## Base Model: Jamba Reasoning 3B

### Model Selection

| Criterion | Jamba Reasoning 3B | Granite 4.0-H-Micro | Qwen3-4B (rejected) |
|-----------|-------------------|---------------------|---------------------|
| Architecture | Hybrid Mamba-Transformer (26 Mamba + 2 Attention layers) | Hybrid Mamba-2/Transformer (9:1 ratio), dense | Pure transformer |
| Parameters | 3B | 3B | 4B |
| Q4_K_M size | ~1.8GB | ~1.8GB | ~2.5GB |
| CPU tok/s | 25-35 tok/s | ~20-30 tok/s | 5-8 tok/s |
| KV cache | Minimal (2 attention layers only) | Minimal (10% attention) | Full (all layers) |
| Instruction-tuned | Yes (SFT + DPO + RLVR) | Yes (SFT + RL + model merging) | Yes |
| Reasoning optimised | Yes (cold-start distillation + online RL) | No | No (non-thinking mode) |
| Licence | Apache 2.0 | Apache 2.0 | Apache 2.0 |
| GGUF/llama.cpp | Yes (official GGUF release) | Yes (Unsloth GGUF) | Yes |
| LoRA fine-tuning | Yes (PEFT + VeRL, documented) | Yes (Unsloth, documented) | Yes |
| Context window | 256K (1M theoretical) | 128K | 32K |

**Decision: Jamba Reasoning 3B** based on:

- **Best inference speed at 3B class**: The hybrid architecture with only 2 attention layers means almost no KV cache overhead. This is the fastest 3B model available for CPU inference.
- **Reasoning-optimised training**: Cold-start distillation + RLVR specifically targets structured output, mathematical problem-solving, and information extraction — exactly our use case.
- **Proven fine-tuning path**: AI21 provides official LoRA fine-tuning documentation with target modules for both Mamba layers (`x_proj`, `in_proj`, `out_proj`) and attention layers (`q_proj`, `k_proj`, `v_proj`, `o_proj`).
- **Benchmarks beat competitors**: Outperforms Gemma 3 4B, Llama 3.2 3B, and Granite 4.0 Micro on combined intelligence scores despite being the same size or smaller.

### Why Not Granite 4.0-H-Micro

Granite 4.0-H-Micro is a strong alternative — same hybrid architecture concept, same size class, excellent enterprise pedigree from IBM. It remains a viable fallback if Jamba Reasoning 3B fine-tuning proves problematic. However, Jamba Reasoning 3B's reasoning-specific training (RLVR on code generation, math, structured output, information extraction) gives it an edge on our task profile. Granite 4.0 was not trained with reasoning-specific RL.

### Why Not Granite 4.0-H-Tiny

7B total parameters with 1B active (MoE). Despite only activating 1B, the full 7B must be loaded into memory (~4GB Q4_K_M). Jamba Reasoning 3B at ~1.8GB gives better quality at lower footprint.

### Why Not Pure Mamba (Falcon Mamba 7B)

Too large (7B parameters). Also, research shows pure Mamba models have "fuzzy memory" — they struggle with exact recall and copying tasks. The hybrid approach (Mamba + sparse attention) addresses this weakness. Our task requires exact numerical output (recommending "512Mi" not "roughly 500Mi"), making hybrid essential.

### Why Not Pure Transformers (Qwen3-4B, Phi-4 Mini)

5-8x slower inference on CPU. The entire reason for the hybrid Mamba architecture is that it makes the SLM fast enough to be the real-time intelligence layer, not just an on-demand explainer.

---

## Structured Output Format

L2 outputs structured JSON. This is critical for speed and reliability:

```json
{
  "cpu_request": "250m",
  "cpu_limit": "500m",
  "memory_request": "512Mi",
  "memory_limit": "768Mi",
  "pattern": "burstable",
  "confidence": 0.82,
  "reasoning_code": "jvm_gc_reservation",
  "explanation": "Memory usage at 93% is expected for JVM with -Xmx2g. G1GC reserves heap up to the configured maximum. Reducing memory would trigger OOMKills under GC pressure.",
  "risk": "low"
}
```

Benefits:
- **Faster generation**: 80-120 tokens vs 300+ for free-form prose
- **Machine-parseable**: No fragile LLM output parsing
- **Human-readable explanations**: Full natural language reasoning in the `explanation` field
- **~3-4 seconds per workload** at 25-35 tok/s

---

## Inference Speed: Why Hybrid Mamba Changes Everything

### Token Generation Rates (CPU-only, Q4_K_M, llama.cpp)

| Model | Architecture | Tokens/sec | 100-token output | 300-token output |
|-------|-------------|-----------|-----------------|-----------------|
| **Jamba Reasoning 3B** | Hybrid Mamba | 25-35 tok/s | **3-4 seconds** | 9-12 seconds |
| Granite 4.0-H-Micro 3B | Hybrid Mamba-2 | 20-30 tok/s | 3-5 seconds | 10-15 seconds |
| Qwen3-4B (rejected) | Transformer | 5-8 tok/s | 12-20 seconds | 37-60 seconds |
| Qwen3-1.7B | Transformer | 12-18 tok/s | 6-10 seconds | 17-25 seconds |

### Cluster-Scale Coverage Per 5-Minute Cycle

| Workloads | Jamba 3B (hybrid Mamba) | Qwen3-4B (transformer) |
|-----------|------------------------|------------------------|
| 10 | **~35 sec** ✓ | ~2.5 min ✓ |
| 50 | **~2.5 min** ✓ | ~12 min ✗ |
| 100 | **~5 min** ≈ | ~25 min ✗ |
| 200 | **~10 min** (2 cycles) | ~50 min ✗ |

With Jamba Reasoning 3B, k8s-sage can analyse 50+ workloads within a single push cycle. For larger clusters, a priority queue ensures every workload is reviewed within 2-3 cycles. This is a fundamentally different product capability than was possible with a pure transformer.

### L2 Invocation Pattern

L2 runs continuously, processing a priority queue each cycle:

1. **Every cycle**: L2 processes the top N workloads ranked by L1 confidence (lowest first), change magnitude (highest first), or staleness (longest since last L2 review).
2. **On-demand** via CLI: `sage report --explain` triggers immediate L2 analysis of specified workloads.
3. **On schedule**: Daily/hourly cluster report with natural language insights.
4. **On API request**: `POST /api/v1/explain` sends a workload to L2.

Over 24 hours with 5-minute cycles (288 cycles × ~50 workloads per cycle), L2 reviews ~14,000 workload-analyses. Even a 500-workload cluster gets each workload reviewed ~28 times per day.

---

## Competitive Context

### Resource Footprint Comparison

| Tool | In-cluster footprint | Where ML runs | Continuous? | Right-sizing? | Data sovereign? | Cost |
|------|---------------------|---------------|------------|---------------|----------------|------|
| **PerfectScale** | ~100MB (agent) | SaaS cloud | ✓ | ✓ | ✗ | $5-15k/mo |
| **StormForge** | ~200MB (agent + forwarder) | SaaS cloud | ✓ | ✓ | ✗ | $$$$/mo |
| **Kubecost** | 3-5GB (analyzer + Prometheus + Grafana) | In-cluster (rules) | ✓ | Basic recs | ✓ | Free/paid |
| **K8sGPT** | ~50MB (+ LocalAI if local) | OpenAI API default | ✗ (on-demand) | ✗ (diagnostics) | ✗ default | API costs |
| **VPA** | ~500MB-1GB | In-cluster (percentile) | ✓ | Basic | ✓ | Free |
| **k8s-sage (L1 only)** | **~80MB** | **In-cluster (rules)** | **✓** | **Basic** | **✓** | **Free** |
| **k8s-sage (L1+L2)** | **~2GB** | **In-cluster (hybrid Mamba SLM)** | **✓** | **✓ + explains** | **✓** | **Free** |

### Key Differentiators

1. **Data sovereignty**: PerfectScale and StormForge ship metrics to their cloud. k8s-sage never phones home.
2. **No SaaS dependency**: K8sGPT defaults to OpenAI API. k8s-sage runs fully air-gapped.
3. **Continuous + intelligent**: VPA does percentile math. k8s-sage classifies workload patterns and applies runtime-aware reasoning.
4. **Lighter than Kubecost**: k8s-sage at ~2GB is less than half of Kubecost — and does intelligent right-sizing with natural language explanations.
5. **Deployable without L2**: `--set slm.enabled=false` runs L1 only at ~80MB. Every feature except ML-powered recommendations and explanations works.
6. **Cost**: PerfectScale/StormForge charge $5-15k+/month. k8s-sage is free, forever.

---

## Fine-Tuning: LoRA

### Why LoRA

Full fine-tuning of a 3B model requires ~24GB VRAM (bf16) and risks catastrophic forgetting. LoRA fine-tunes only adapter parameters (~0.5-1% of total), preserving the base model's capabilities while injecting domain knowledge. Research on PEFT for Mamba architectures (Sony MambaPEFT, 2024) shows that PEFT is more effective on Mamba than on transformers — Mamba's modular SSM structure prevents pre-trained memory corruption from additional parameters.

### Configuration

```
Method:         LoRA (via HuggingFace PEFT / TRL SFTTrainer)
Base model:     ai21labs/AI21-Jamba-Reasoning-3B
LoRA rank (r):  8
LoRA alpha:     16 (alpha/rank = 2, standard scaling)
Dropout:        0.05
Target modules: x_proj, in_proj, out_proj    (Mamba layers)
                q_proj, k_proj, v_proj, o_proj (Attention layers)
                gate_proj, up_proj, down_proj  (MLP layers)
```

#### Why These Target Modules

Unlike pure transformers where only attention layers are targeted, hybrid Mamba models benefit from LoRA on all three module types:

- **Mamba layers** (`x_proj`, `in_proj`, `out_proj`): Where the model learns sequence-level patterns. Research shows LoRAp(X) — partial LoRA on the X projection — is particularly effective for Mamba on small datasets (<8k examples).
- **Attention layers** (`q_proj`, `k_proj`, `v_proj`, `o_proj`): The 2 attention layers in Jamba handle precise local reasoning. Critical for exact numerical output.
- **MLP layers** (`gate_proj`, `up_proj`, `down_proj`): Feed-forward layers that transform representations. Including these follows AI21's official fine-tuning guidance.

#### Why Rank 8 (Not 16)

Jamba Reasoning 3B has more LoRA target modules than a pure transformer (Mamba + Attention + MLP = 10 modules per layer vs 4 for attention-only). At rank 8, the total trainable parameter count is comparable to rank 16 on a pure transformer targeting only attention. This provides sufficient capacity without overfitting on ~8k examples.

### Training Configuration

```
Learning rate:         2e-4 (with cosine scheduler, warmup 100 steps)
Batch size:            4 (per device)
Gradient accumulation: 4 steps (effective batch size: 16)
Epochs:                3
Max sequence length:   2048 tokens
Optimizer:             AdamW (weight decay 0.01)
Precision:             bf16 mixed precision
Hardware:              Single GPU (RTX 3090 24GB / A100 40GB)
Estimated time:        3-6 hours (RTX 3090) / 1.5-3 hours (A100)
```

Note: Jamba requires `mamba-ssm` and `causal-conv1d` packages for optimised Mamba kernels during training. QLoRA (4-bit quantisation during training) is supported and documented by AI21 for single-GPU fine-tuning.

---

## Post-Training Pipeline

### 1. Merge LoRA Adapters

```python
from peft import PeftModel
from transformers import AutoModelForCausalLM

base = AutoModelForCausalLM.from_pretrained(
    "ai21labs/AI21-Jamba-Reasoning-3B",
    torch_dtype=torch.bfloat16,
    device_map="auto"
)
model = PeftModel.from_pretrained(base, "output/k8s-sage-lora")
merged = model.merge_and_unload()
merged.save_pretrained("output/k8s-sage-merged")
```

### 2. Quantise to GGUF Q4_K_M

```bash
python convert_hf_to_gguf.py output/k8s-sage-merged --outtype q4_k_m --outfile k8s-sage-q4.gguf
```

**Why Q4_K_M**:
- ~1.8GB file size for 3B model
- Minimal quality loss compared to fp16 (~1-2% on benchmarks)
- The "K_M" variant preserves important layers at higher precision
- CPU inference: 25-35 tokens/sec on modern vCPU

### 3. Serving: llama.cpp Server

```yaml
# Helm values
slm:
  enabled: true       # false = L1-only (~80MB footprint)
  image: ghcr.io/ggerganov/llama.cpp:server
  resources:
    requests:
      cpu: "1"
      memory: "2.5Gi"
    limits:
      cpu: "2"
      memory: "3Gi"
```

#### Optimisation: Reduced Context Window

Jamba supports 256K context, but our inputs are ~200-400 tokens with ~100-300 token output. Setting `--ctx-size 1024` minimises memory overhead. Since Jamba has only 2 attention layers, KV cache is already minimal — but reducing context still saves memory for the Mamba state buffers.

```
# llama.cpp server args
--model /models/k8s-sage-q4.gguf
--ctx-size 1024
--threads 2
--port 8080
```

#### Why llama.cpp Instead of Ollama

- **Leaner**: Single static binary, no model management overhead (~200-300MB saved)
- **Predictable**: No background model loading/unloading, no registry pulls
- **Configurable**: Direct control over ctx-size, thread count, batch size
- **Mamba support**: llama.cpp supports Jamba/Mamba GGUF natively

---

## Resource Footprint Summary

### L1 Only (SLM Disabled)

| Component | CPU | Memory | Notes |
|-----------|-----|--------|-------|
| k8s-sage server + L1 | ~20m | ~50Mi | Go binary |
| k8s-sage agents (×3) | ~30m total | ~150Mi | Go binaries |
| **Total** | **~50m** | **~200Mi** | Rules-only right-sizing |

### L1+L2 (Full)

| Component | CPU | Memory | Notes |
|-----------|-----|--------|-------|
| llama.cpp server (idle) | ~30m | ~1.8Gi | Model loaded, waiting |
| llama.cpp server (inference) | 1-2 cores | ~2.0Gi | Minimal KV cache (2 attn layers) |
| k8s-sage server + L1 | ~20m | ~50Mi | Go binary |
| k8s-sage agents (×3) | ~30m total | ~150Mi | Go binaries |
| **Total (idle)** | **~80m** | **~2.0Gi** | |
| **Total (inference)** | **~2 cores** | **~2.2Gi** | |

Note: The Mamba hybrid architecture's minimal KV cache means idle and inference memory are nearly identical — unlike transformers where KV cache can add 500MB-1GB during inference.

---

## Evaluation Plan

### Benchmark Tasks

Create a held-out test set of 500 pairs (not used in training) covering:

1. **Right-sizing accuracy** (200 pairs): Given metrics, does the model recommend values within 20% of the ground truth? The model should match or improve on L1 rules engine recommendations.

2. **Pattern classification** (100 pairs): Does the model correctly identify steady/burstable/batch/idle patterns from metric descriptions?

3. **Runtime-specific reasoning** (100 pairs): Does the model correctly handle JVM, Go, Python, Node.js workloads? Key test: does it avoid recommending memory reduction for JVM workloads with high -Xmx?

4. **Edge case handling** (100 pairs): CrashLoopBackOff, init containers, sidecars, DaemonSets, StatefulSets, BestEffort QoS.

### Comparison Targets

| Model | Expected Performance | Purpose |
|-------|---------------------|---------|
| L1 rules engine alone | ~75% accuracy | Baseline — L2 must beat this |
| Base Jamba Reasoning 3B (no fine-tune) | ~45-55% accuracy | Baseline — reasoning training helps but lacks domain knowledge |
| k8s-sage L2 (fine-tuned Jamba 3B) | ~85% accuracy | Target |
| GPT-4 (prompted) | ~60-75% accuracy | Reference ceiling |

"Accuracy" means: recommendation is within 20% of ground truth, pattern classification is correct, and no safety violations.

### Safety Checks

Every L2 output is validated against L1's safety invariants:
- Memory recommendation >= P99 working set × 1.10
- CPU recommendation >= P95 × headroom (pattern-dependent)
- No recommendation of 0 for any resource
- No recommendation exceeding current request (upsizes handled separately)

If L2 violates safety invariants, the L1 output is used instead (graceful degradation).

---

## PEFT Research: Mamba-Specific Findings

Sony's MambaPEFT research (2024) informs our fine-tuning strategy:

- **PEFT works better on Mamba than transformers**: Mamba models continue to improve accuracy with more trainable parameters without overfitting, whereas transformers start degrading. The modular SSM structure prevents pre-trained memory corruption.
- **LoRAp(X) is optimal for limited data (<8k examples)**: Partial LoRA on the X projection outperforms blanket LoRA on small datasets. This maps directly to our ~8k training pairs.
- **Additional-scan for larger datasets**: A Mamba-specific PEFT method that extends SSM state dimensions. With 170k+ samples it outperforms LoRA at 0.26-0.51% of model parameters. Relevant for future phases when we accumulate production telemetry.
- **Initialisation matters for SSM parameters**: Neighbourhood-based initialisation from pre-trained A values outperforms standard S4D initialisation.
- **Hybrid PEFT combinations outperform individual methods**: An efficient two-step search (activate methods with minimal params, then tune hyperparameters) finds superior combinations.

### Fine-Tuning Roadmap

| Phase | Data Size | PEFT Strategy |
|-------|-----------|---------------|
| **v1 (now)** | ~8k pairs | Standard LoRA on all module types (Mamba + Attention + MLP) |
| **v1.1** | ~8k pairs | Experiment with LoRAp(X) on Mamba layers specifically |
| **v2** | ~50k+ pairs (production telemetry) | Evaluate Additional-scan for Mamba layers, keep LoRA on attention/MLP |
| **v2+** | ~170k+ pairs | Hybrid PEFT search: Additional-scan + LoRA combination |

---

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Jamba Reasoning 3B fine-tuning produces poor results | Wasted effort | Granite 4.0-H-Micro as fallback — same hybrid Mamba architecture, same fine-tuning approach, slightly different training lineage |
| Hybrid Mamba GGUF inference slower than claimed | L2 can't keep up with push cycles | Priority queue already handles this — degrade gracefully to reviewing fewer workloads per cycle |
| L2 hallucinates specific numbers | Wrong recommendations | Safety invariant check against L1 output; L1 is always authoritative on safety bounds |
| L2 overfitting to training data phrasing | Brittle on novel inputs | Diverse training data, validation set monitoring, temperature 0.3 |
| L2 too large for customer environments | Can't deploy | L1-only mode (`--set slm.enabled=false`) at ~200MB; or wait for Granite 4.0 Nano hybrid (1B) as ultra-compact option |
| Mamba GGUF support in llama.cpp is immature | Inference bugs | Jamba has official GGUF releases and is actively tested in llama.cpp; Ollama also supported as fallback runtime |
| Quantisation degrades quality | Worse than base model | Benchmark Q4 vs fp16 on test set; if >5% degradation, use Q5_K_M (slightly larger) |
| Training data contains errors | Model learns wrong patterns | Human review of all expert pairs, automated validation of metric plausibility |
| `mamba-ssm` / `causal-conv1d` CUDA dependencies for training | Complex training setup | QLoRA path documented by AI21; single GPU sufficient; VeRL framework support coming |
