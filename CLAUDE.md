# CLAUDE.md — k8s-sage

## What Is This Project

k8s-sage is the world's first Kubernetes-specialist small language model and efficiency platform. There is currently no fine-tuned LLM that natively understands Kubernetes resource patterns, right-sizing decisions, or operational best practices. This project fills that gap.

The product has two parts:
1. **A lightweight in-cluster agent** that collects resource metrics and identifies waste
2. **A fine-tuned small language model (SLM)** trained specifically on K8s operational knowledge that provides intelligent, context-aware recommendations

The SLM is the core differentiator. Tools like K8sGPT exist but rely on general-purpose LLMs (GPT-4, Llama, etc.) with no K8s-specific training. They're slow, expensive, and often give generic advice. k8s-sage's model is small enough to run CPU-only inside a cluster, purpose-built for K8s, and dramatically more useful for resource optimization than a general model.

## IP & Ownership

This project is personal IP created by Gregory Carroll before employment at Prolific Academic Ltd (start date: 16 March 2026). All commits must have accurate timestamps. This is critical — do not backdate or manipulate commit dates.

## Architecture

See `docs/ARCHITECTURE.md` for the full system design. Summary:

```
┌─────────────────────────────────────┐
│         Kubernetes Cluster          │
│                                     │
│  ┌─────────┐ ┌─────────┐           │
│  │ Agent   │ │ Agent   │  (DaemonSet│
│  │ Node 1  │ │ Node N  │  per node) │
│  └────┬────┘ └────┬────┘           │
│       └─────┬─────┘                 │
│             ▼                       │
│  ┌──────────────────┐               │
│  │   sage-server    │               │
│  │                  │               │
│  │  ┌────────────┐  │               │
│  │  │Rules Engine│  │               │
│  │  └─────┬──────┘  │               │
│  │        ▼         │               │
│  │  ┌────────────┐  │               │
│  │  │ K8s SLM    │  │  (Optional)   │
│  │  │ (Ollama)   │  │               │
│  │  └────────────┘  │               │
│  └────────┬─────────┘               │
│           ▼                         │
│       REST API                      │
└───────────┬─────────────────────────┘
            ▼
     ┌────────────┐
     │  sage-cli   │
     └────────────┘
```

## Tech Stack

| Component | Language | Notes |
|-----------|----------|-------|
| Agent (DaemonSet) | Go 1.22+ | client-go, must stay under 50MB RAM / 0.05 CPU |
| Server | Go 1.22+ | chi router, aggregation + rules engine |
| CLI | Go 1.22+ | cobra |
| ML training pipeline | Python 3.11+ | transformers, peft, datasets (HuggingFace) |
| Model serving | Python / Ollama | GGUF quantized model via Ollama |
| Helm chart | YAML | Helm 3 |
| Training data | JSONL | Instruction-tuning format |

## Repo Structure

```
k8s-sage/
├── CLAUDE.md
├── README.md
├── COPYRIGHT                      # Proprietary — all rights reserved
├── Makefile
├── go.mod / go.sum
│
├── cmd/
│   ├── agent/main.go              # DaemonSet entrypoint
│   ├── server/main.go             # Server entrypoint
│   └── cli/main.go                # CLI entrypoint
│
├── internal/
│   ├── agent/
│   │   ├── collector.go           # Kubelet/cAdvisor metric scraping
│   │   ├── store.go               # Rolling window in-memory store
│   │   └── reporter.go            # /report endpoint + push to server
│   ├── server/
│   │   ├── aggregator.go          # Collect reports from all agents
│   │   ├── analyzer.go            # Orchestrate rules + optional SLM
│   │   └── api.go                 # REST handlers
│   ├── rules/
│   │   ├── engine.go              # Deterministic right-sizing rules
│   │   ├── patterns.go            # Workload classification (steady/burst/batch)
│   │   └── recommendations.go     # Generate structured recommendations
│   ├── slm/
│   │   ├── client.go              # Client for local Ollama/model endpoint
│   │   ├── prompts.go             # Prompt templates for K8s analysis
│   │   └── parser.go              # Parse structured responses from model
│   └── models/
│       ├── metrics.go             # Core metric types
│       ├── report.go              # Report structures
│       └── recommendation.go      # Recommendation types
│
├── ml/
│   ├── README.md                  # ML roadmap and methodology
│   ├── dataset/
│   │   ├── sources.md             # Documented data sources
│   │   ├── collect_k8s_docs.py    # Scrape K8s official docs
│   │   ├── collect_gh_issues.py   # K8s GitHub issues related to resources
│   │   ├── collect_so.py          # Stack Overflow K8s resource questions
│   │   ├── generate_synthetic.py  # Generate metric→recommendation pairs
│   │   ├── format_instruct.py     # Convert all sources to JSONL
│   │   └── data/                  # Output datasets
│   ├── training/
│   │   ├── finetune_lora.py       # LoRA fine-tuning script
│   │   ├── merge_and_quantize.py  # Merge adapters + GGUF quantization
│   │   ├── eval.py                # Benchmark against general models
│   │   └── configs/               # Hyperparameter configs
│   └── serving/
│       ├── Modelfile              # Ollama Modelfile for k8s-sage
│       └── test_inference.py      # Smoke tests for model quality
│
├── deploy/
│   └── helm/k8s-sage/
│       ├── Chart.yaml
│       ├── values.yaml
│       └── templates/
│
├── docs/
│   ├── ARCHITECTURE.md
│   ├── TRAINING_DATA.md           # Dataset methodology
│   └── MODEL_DESIGN.md            # Model selection and fine-tuning approach
│
├── test/
│   ├── unit/
│   ├── integration/
│   └── fixtures/                  # Mock kubelet responses, sample metrics
│
├── scripts/
│   ├── setup-dev.sh
│   └── kind-cluster.sh
│
├── Dockerfile.agent
├── Dockerfile.server
└── .github/workflows/
    ├── ci.yaml
    └── release.yaml
```

## Coding Standards

### Go
- Standard project layout (`cmd/`, `internal/`, `pkg/`)
- Errors: always wrap with context — `fmt.Errorf("collecting metrics for node %s: %w", node, err)`
- Logging: `slog` (structured, stdlib)
- Tests: table-driven, use `testify/assert`
- No global state — inject dependencies via structs
- Interfaces for external boundaries (kubelet client, model client, HTTP)

### Python (ML)
- Python 3.11+, `ruff` for linting/formatting
- Type hints on all function signatures
- `pyproject.toml` for config
- All datasets documented with provenance in `sources.md`

### General
- Conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `chore:`, `ml:`
- No secrets in code ever
- Favour readability over cleverness

## Build & Test

```bash
make build            # Build all Go binaries
make test             # Unit tests
make lint             # Go vet + staticcheck + ruff
make docker-build     # Build container images
make dev-cluster      # Spin up kind cluster
make dev-deploy       # Deploy to kind via Helm
make test-integration # Requires running cluster
```

## Key Design Decisions

### Agent must be invisible
50MB RAM, 0.05 CPU. If the efficiency tool uses meaningful resources, it has failed. The agent does NOT run any AI — it collects metrics and applies simple math. All intelligence lives in the server.

### Rules engine is the MVP, SLM is the differentiator
The rules engine provides deterministic right-sizing (e.g., "pod requests 4 CPU, P95 usage is 0.3, recommend 0.5 with headroom"). This works without any model. The SLM adds nuance: pattern recognition, natural language explanations, workload classification that rules can't capture.

### SLM runs via Ollama as a single central deployment
Not per-node. One instance serves the whole cluster. The model is invoked on-demand or on a schedule, not continuously. Target: Phi-3 Mini or TinyLlama base, Q4 GGUF, ~2.5GB RAM, CPU-only.

### Training data is as valuable as the model
The curated K8s efficiency dataset doesn't exist anywhere. Sources include: K8s docs, VPA recommender logic, GitHub issues, Stack Overflow, cloud provider best practices, and synthetically generated metric→recommendation pairs. This dataset is core IP.

## Sprint Plan (Pre-March 16)

### Week 1 (Feb 28 – Mar 7): Foundation
- [ ] Repo init, Go module, project scaffolding
- [ ] Agent: kubelet summary API collector
- [ ] Agent: rolling window in-memory store with downsampling
- [ ] Agent: /report endpoint (JSON waste per pod)
- [ ] Server: basic aggregation from agents
- [ ] Rules engine: simple right-sizing (request vs P95 usage)
- [ ] Unit tests for collector, store, rules

### Week 2 (Mar 8 – Mar 15): Intelligence & Packaging
- [ ] Server: full REST API
- [ ] CLI: cluster report, namespace drill-down
- [ ] Workload pattern classification (steady/burstable/batch)
- [ ] ML dataset: begin curation pipeline
- [ ] ML dataset: K8s docs extraction
- [ ] ML dataset: synthetic metric→recommendation pair generation
- [ ] Helm chart with working defaults
- [ ] Dockerfiles for agent and server
- [ ] kind-based integration test
- [ ] README, ARCHITECTURE.md, TRAINING_DATA.md
- [ ] Dataset format spec (instruction-tuning JSONL)

### Post-Start (personal time): Model Training
- [ ] Complete dataset curation (target: 10k+ instruction pairs)
- [ ] Fine-tune Phi-3 Mini with LoRA
- [ ] Quantize to GGUF Q4
- [ ] Create Ollama Modelfile
- [ ] Integrate model into server via slm/ package
- [ ] Benchmark against GPT-4 / base Phi-3 on K8s tasks
- [ ] Publish model to HuggingFace

## Context for Claude Code

When working on this project:
- Prioritise working code over perfection — we have 16 days
- Write tests alongside implementation, not after
- The agent Go code should be boring and reliable — no cleverness
- The ML Python code can be more experimental
- Always check resource implications — if something could bloat the agent, flag it
- When generating training data formats, follow HuggingFace instruction-tuning conventions
- The model's job is K8s resource efficiency, not general K8s troubleshooting (that's K8sGPT's space)
