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
│  │  │ (llama.cpp)│  │               │
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
| Model serving | llama.cpp | GGUF Q4_K_M, CPU-only via llama.cpp server |
| Helm chart | YAML | Helm 3 |
| Training data | JSONL | Instruction-tuning format |

## Repo Structure

```
k8s-sage/
├── CLAUDE.md
├── README.md
├── COPYRIGHT                      # Proprietary — all rights reserved
├── Makefile
├── pyproject.toml                 # Python deps ([train], [eval], [lint])
├── go.mod / go.sum
│
├── cmd/
│   ├── agent/main.go              # DaemonSet entrypoint
│   ├── server/main.go             # Server entrypoint (+ Slack notifier bootstrap)
│   └── cli/                       # CLI: report, recommend, workloads, annotate, discover
│
├── internal/
│   ├── agent/                     # Collector, store, reporter, pusher
│   ├── server/
│   │   ├── aggregator.go          # Collect + aggregate by owner
│   │   ├── analyzer.go            # L1+L2 orchestrator with safety fallback
│   │   └── api.go                 # REST handlers
│   ├── rules/
│   │   ├── engine.go              # Orchestrates classification + recommendation
│   │   ├── patterns.go            # Classification (steady/burstable/batch/idle/anomalous)
│   │   └── recommendations.go     # Right-sizing, safety floors, reduction caps
│   ├── slm/
│   │   ├── client.go              # llama.cpp HTTP client
│   │   ├── prompts.go             # Prompt construction from workload metrics
│   │   └── parser.go              # JSON response parsing + validation
│   ├── slack/                     # Slack webhook notifier + Block Kit messages
│   ├── gitops/                    # ArgoCD + Flux workload discovery
│   └── models/                    # Shared types (metrics, reports, recommendations)
│
├── ml/
│   ├── dataset/
│   │   ├── data/training_data.jsonl     # 6,982 validated pairs
│   │   ├── raw/                         # Per-source JSONL files
│   │   ├── examples/expert_pairs.jsonl  # Hand-written expert pairs
│   │   ├── generate_synthetic.py        # Rules engine → synthetic pairs
│   │   ├── format_instruct.py           # Validate, dedup, merge → final dataset
│   │   ├── collect_*.py                 # Data collection scripts
│   │   ├── generate_*.py                # Data generation scripts
│   │   └── reports/                     # Dataset analytics
│   ├── training/
│   │   ├── finetune_lora.py       # QLoRA fine-tuning (SFTTrainer, --dry-run)
│   │   ├── merge_and_quantize.py  # Merge LoRA + GGUF Q4_K_M
│   │   ├── eval.py                # Accuracy, safety, pattern metrics
│   │   └── configs/default.yaml   # Jamba 3B QLoRA config
│   └── serving/
│       ├── run_llama_cpp.sh       # llama.cpp launch script
│       ├── Modelfile              # Model parameters
│       └── test_inference.py      # 5-scenario smoke test
│
├── deploy/
│   ├── helm/k8s-sage/             # Helm chart (agent, server, SLM, Grafana, Slack)
│   └── grafana/                   # Standalone Grafana dashboard
│
├── test/
│   ├── backtest/                  # 52 scenario regression tests
│   ├── safety/                    # 8 safety invariant tests
│   ├── integration/               # End-to-end tests
│   ├── dogfood/                   # 8 workload archetypes + validation
│   └── fixtures/                  # Backtest scenarios JSON
│
├── scripts/
│   ├── train.sh                   # One-command training (DRY_RUN support)
│   ├── eval_and_deploy.sh         # Merge + quantise + evaluate
│   ├── setup-dev.sh
│   └── kind-cluster.sh
│
├── docs/
│   ├── ARCHITECTURE.md            # System design (this is the source of truth)
│   ├── MODEL_DESIGN.md            # Model selection + training config
│   ├── TRAINING_DATA.md           # Dataset methodology + provenance
│   ├── TESTING_RIG_RUNBOOK.md     # Clone → train → deploy guide
│   └── UX_RECOMMENDATION_FLOW.md  # Slack integration design
│
└── .github/workflows/ci.yaml
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
make test             # Unit tests (go test -p 2 -timeout 120s ./...)
make lint             # Go vet + staticcheck + ruff + helm lint
make lint-python      # ruff check ml/
make lint-helm        # helm lint deploy/helm/k8s-sage/
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

### L1 rules engine is always the safety floor
The SLM (L2) can enhance recommendations but never override L1 safety invariants: 50m CPU floor, 64Mi memory floor, confidence-gated reduction caps (30%/50%/75%), anomaly detection. If L2 fails or violates safety, L1 stands.

### SLM runs via llama.cpp as a single central deployment
Not per-node. One instance serves the whole cluster. The model is invoked on-demand or on a schedule, not continuously. Base: AI21 Jamba Reasoning 3B, QLoRA fine-tuned, GGUF Q4_K_M, ~2.5GB RAM, CPU-only.

### Training data is as valuable as the model
The curated K8s efficiency dataset doesn't exist anywhere. Sources include: K8s docs, VPA recommender logic, GitHub issues, Stack Overflow, cloud provider best practices, and synthetically generated metric→recommendation pairs. This dataset is core IP.

## Project Status

73 commits. See `docs/ARCHITECTURE.md` for detailed status.

### Complete
- Agent, server, rules engine (with L1 safety fixes), CLI, Helm chart, CI/CD
- ML pipeline: 6,982 training pairs, QLoRA script, merge/quantise, eval, serving
- Go SLM integration: client, prompts, parser (21 tests), L1+L2 analyzer
- Dogfood workloads (8 archetypes) with validation scripts
- CI: ruff + helm lint in pipeline; Makefile lint-python/lint-helm targets
- Grafana dashboard (Infinity datasource) + Helm ConfigMap
- `sage annotate` + `sage discover` CLI subcommands
- Slack notifier scaffold (webhook, Block Kit, severity/dedup)
- GitOps discovery (ArgoCD + Flux)
- Models package tests (30 functions)

### Next: Training on GPU rig
- `./scripts/train.sh` — fine-tune Jamba 3B (3-6h on dual 3090)
- `./scripts/eval_and_deploy.sh` — merge, quantise, evaluate
- Dogfood v2 (L1) and v3 (with L2)

### Remaining
- PR creation flow, KWOK scale testing, marketplace listing

## Context for Claude Code

When working on this project:
- Prioritise working code over perfection — we have 16 days
- Write tests alongside implementation, not after
- The agent Go code should be boring and reliable — no cleverness
- The ML Python code can be more experimental
- Always check resource implications — if something could bloat the agent, flag it
- When generating training data formats, follow HuggingFace instruction-tuning conventions
- The model's job is K8s resource efficiency, not general K8s troubleshooting (that's K8sGPT's space)
