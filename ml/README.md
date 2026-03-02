# k8s-sage ML Pipeline

Training pipeline for the k8s-sage small language model — a Kubernetes resource efficiency specialist.

## Overview

The pipeline collects training data from multiple sources, validates and deduplicates it, fine-tunes AI21 Jamba Reasoning 3B with QLoRA, and quantises to GGUF Q4_K_M for CPU-only serving via llama.cpp.

See `docs/TRAINING_DATA.md` for the dataset strategy and `docs/MODEL_DESIGN.md` for model selection and training configuration.

## Directory Structure

```
ml/
├── README.md                    # This file
├── dataset/
│   ├── schema.json              # Instruction-tuning JSONL schema
│   ├── sources.md               # Data source documentation and licensing
│   ├── examples/                # Hand-written expert training pairs
│   │   └── expert_pairs.jsonl   # 191 expert pairs
│   ├── raw/                     # Raw collected data
│   │   ├── stackoverflow_filtered.jsonl  # 2,409 pairs
│   │   ├── github_issues_pairs.jsonl     # 530 pairs
│   │   ├── k8s_docs_pairs.jsonl          # 300 pairs
│   │   ├── vpa_source_pairs.jsonl        # 69 pairs
│   │   └── synthetic_pairs.jsonl         # 4,500 pairs
│   ├── collect_k8s_docs.py      # K8s docs → instruction pairs
│   ├── collect_gh_issues.py     # GitHub issues → instruction pairs
│   ├── collect_so.py            # Stack Overflow → instruction pairs
│   ├── generate_synthetic.py    # Rules engine → synthetic pairs
│   ├── format_instruct.py       # Validate, dedup, merge → final dataset
│   ├── data/                    # Final merged dataset
│   │   └── training_data.jsonl  # 6,982 validated pairs
│   └── reports/                 # Dataset analytics
│       ├── volume_assessment.md
│       └── final_dataset_report.md
├── training/
│   ├── finetune_lora.py         # QLoRA fine-tuning (SFTTrainer)
│   ├── merge_and_quantize.py    # Merge LoRA + GGUF Q4_K_M conversion
│   ├── eval.py                  # Right-sizing accuracy, safety compliance
│   └── configs/
│       └── default.yaml         # Jamba 3B QLoRA config
└── serving/
    ├── Modelfile                # llama.cpp model parameters
    ├── run_llama_cpp.sh         # Launch script
    └── test_inference.py        # 5-scenario smoke test
```

## Quick Start

### 1. Generate + Build Dataset

```bash
python3 ml/dataset/generate_synthetic.py --count 4500 --seed 42
python3 ml/dataset/format_instruct.py --output ml/dataset/data/training_data.jsonl
```

### 2. Train

```bash
# One-command training (installs deps, validates dataset, runs QLoRA)
./scripts/train.sh

# Or dry-run first (no GPU needed)
DRY_RUN=1 ./scripts/train.sh
```

### 3. Merge + Quantise + Evaluate

```bash
./scripts/eval_and_deploy.sh
```

### 4. Serve

```bash
./ml/serving/run_llama_cpp.sh output/k8s-sage-q4.gguf
python3 ml/serving/test_inference.py --url http://localhost:8080
```

## Current Status

- [x] Dataset schema defined
- [x] Expert seed examples (191 pairs)
- [x] K8s docs collection (300 pairs)
- [x] GitHub issues collection (530 pairs)
- [x] Stack Overflow collection (2,409 pairs)
- [x] VPA source extraction (69 pairs)
- [x] Synthetic generation (4,500 pairs)
- [x] Dataset validation pipeline (6,982 final pairs)
- [x] QLoRA fine-tuning script (dry-run validated)
- [x] Merge + GGUF quantisation script
- [x] Evaluation script (accuracy, safety, pattern metrics)
- [x] llama.cpp serving config + smoke tests
- [x] Go SLM integration (client, prompts, parser — 21 tests)
- [x] L1+L2 analyzer orchestrator with safety fallback
- [x] Helm chart for llama.cpp deployment
- [ ] Fine-tune on GPU (ready — run `./scripts/train.sh`)
- [ ] GGUF quantisation (ready — run post-training)
- [ ] Evaluation benchmarks (ready — run post-training)
- [ ] Deploy to cluster with L2 enabled

## Hardware Requirements

| Stage | Hardware | Time |
|-------|----------|------|
| Dataset generation | Any (CPU) | ~1 second |
| Dry-run validation | Any (CPU) | ~30 seconds |
| QLoRA training | 2x RTX 3090 (24GB) | 3-6 hours |
| Merge + quantise | 32GB+ RAM | ~30 minutes |
| Inference (serving) | CPU only | ~2.5Gi RAM |
