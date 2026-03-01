# k8s-sage ML Pipeline

Training pipeline for the k8s-sage small language model — a Kubernetes resource efficiency specialist.

## Overview

The pipeline collects training data from multiple sources, validates and deduplicates it, fine-tunes a Phi-3 Mini 4K Instruct model with LoRA, and quantises to GGUF for CPU-only serving via Ollama.

See `docs/TRAINING_DATA.md` for the dataset strategy and `docs/MODEL_DESIGN.md` for model selection and training configuration.

## Directory Structure

```
ml/
├── README.md              # This file
├── dataset/
│   ├── schema.json        # Instruction-tuning JSONL schema
│   ├── sources.md         # Data source documentation and licensing
│   ├── examples/          # Hand-written expert training pairs (seed)
│   │   └── expert_pairs.jsonl
│   ├── collect_k8s_docs.py    # K8s docs → instruction pairs
│   ├── collect_gh_issues.py   # GitHub issues → instruction pairs
│   ├── collect_so.py          # Stack Overflow → instruction pairs
│   ├── generate_synthetic.py  # Rules engine → synthetic pairs
│   ├── format_instruct.py     # Validate, dedup, merge → final dataset
│   └── data/                  # Output datasets (gitignored)
├── training/
│   ├── finetune_lora.py       # LoRA fine-tuning script
│   ├── merge_and_quantize.py  # Merge adapters + GGUF quantisation
│   ├── eval.py                # Benchmark against base and GPT-4
│   └── configs/               # Hyperparameter configs
└── serving/
    ├── Modelfile              # Ollama Modelfile for k8s-sage
    └── test_inference.py      # Smoke tests for model quality
```

## Quick Start

### 1. Collect Data

```bash
# Each script outputs JSONL to ml/dataset/data/
python ml/dataset/collect_k8s_docs.py
python ml/dataset/collect_gh_issues.py
python ml/dataset/collect_so.py
python ml/dataset/generate_synthetic.py
```

### 2. Build Dataset

```bash
# Validates against schema, deduplicates, outputs final training set
python ml/dataset/format_instruct.py
```

### 3. Train

```bash
python ml/training/finetune_lora.py --config ml/training/configs/default.yaml
```

### 4. Quantise and Serve

```bash
python ml/training/merge_and_quantize.py
ollama create k8s-sage -f ml/serving/Modelfile
ollama run k8s-sage
```

## Current Status

- [x] Dataset schema defined
- [x] Expert seed examples (20 pairs)
- [x] Collection script scaffolds
- [ ] K8s docs collection
- [ ] GitHub issues collection
- [ ] Stack Overflow collection
- [ ] Synthetic generation
- [ ] Dataset validation pipeline
- [ ] LoRA fine-tuning
- [ ] Quantisation
- [ ] Evaluation benchmarks
