# k8s-sage — Day 4 Plan

## Objective

Complete all remaining development work in this session. Commit everything. You clone on the testing rig (Threadripper + dual 3090) and run training, dogfooding, evaluation from there.

**After this session:** `git clone` → `scripts/train.sh` → trained model. No further code changes needed.

---

## Starting Point

60 commits. All Go infrastructure complete. ML pipeline wired end-to-end (training, serving, Go SLM integration, L1+L2 analyzer, Helm). L1 fixes landed. Strategic docs finalised. 3,459 real training pairs collected.

**Single blocker:** `generate_synthetic.py` does not exist. Without it, the dataset is 3,459 pairs — well short of the ~8,000 target.

---

## Commit Plan

### Commit 1: `ml/dataset/generate_synthetic.py` (Critical Path)

The big one. Programmatic generation of ~4,500 metric→recommendation training pairs from the rules engine logic.

**File:** `ml/dataset/generate_synthetic.py` (~500-600 LOC)

**Scenario matrix:**

| Category | Target | What it teaches the model |
|----------|--------|--------------------------|
| Right-sizing (overprovisioned) | ~1,500 | Request >> P95, varying waste 20%-95%, pattern-appropriate headroom |
| Right-sizing (underprovisioned) | ~500 | Request < P95, throttling/OOM risk, upscale recommendations |
| Runtime-specific | ~800 | JVM heap (-Xmx), Go GOMEMLIMIT, Node event loop, Python GIL, .NET, Ruby |
| Edge cases | ~700 | Near-zero usage, extreme spikes, short data windows, memory leaks, anomalous patterns |
| Classification | ~500 | Metrics at pattern boundaries (steady↔burstable, burstable↔batch, near-idle) |
| Well-sized | ~300 | "No changes needed" — the model must learn to leave things alone |
| Multi-container | ~200 | Sidecars (istio-proxy, fluentd), init containers skewing metrics |

**Key design decisions:**

- **Metric generation mirrors rules engine logic exactly.** Pattern profiles define P50/P95/P99/Max relationships that produce the expected classification. Right-sizing headroom matches `internal/rules/recommendations.go` (CPU: P95 × 1.20, Memory: P99 WS × 1.10).
- **Explanation quality is the entire point.** 15-20 explanation templates per category with runtime-specific variants. Not "reduce memory" but "the JVM's -Xmx512m reserves the full heap on startup; 768Mi provides 256Mi headroom above heap max for metaspace, thread stacks, and OS overhead."
- **Safety invariants enforced at generation time.** Every pair: `P50 <= P95 <= P99 <= Max`, memory rec >= P99 WS × 1.10, CPU rec >= P95 × headroom, no zero recommendations.
- **Deterministic.** `--seed 42` (default), `--count 4500` (default). Same output every run.
- **Output format matches `schema.json` exactly.** `source: "synthetic"`, unique sequential IDs, canonical system prompt.

**Output:** `ml/dataset/raw/synthetic_pairs.jsonl`

**Verify:** `python3 ml/dataset/generate_synthetic.py --count 4500 --seed 42 && wc -l ml/dataset/raw/synthetic_pairs.jsonl`

---

### Commit 2: Update `format_instruct.py`, rebuild dataset

**Changes to `ml/dataset/format_instruct.py`:**
- Add `ml/dataset/raw/synthetic_pairs.jsonl` to `INPUT_FILES`
- Remove or raise the `MAX_SYNTHETIC_RATIO = 0.15` cap — volume assessment already acknowledges 56% synthetic is acceptable given real data provides language diversity and synthetic provides scenario coverage

**Verify:** `python3 ml/dataset/format_instruct.py --output ml/dataset/data/training_data.jsonl` → expect ~8,000 pairs

---

### Commit 3: Fix test stability

Three packages fail with `signal: killed` when run concurrently:
- `internal/server`
- `test/backtest`
- `test/safety`

**Fix approach:**
- Run each individually to identify actual failures vs resource kills
- Update Makefile `test` target to use `-p 1` (sequential) or increase timeout
- Fix any backtest/safety assertion failures caused by the L1 rules changes (floors, caps, anomaly detection changed expected outputs)

---

### Commit 4: Training launch script + testing rig instructions

**File:** `scripts/train.sh` — one-command training:
```bash
#!/bin/bash
set -euo pipefail
pip install -e ".[train]"
python3 ml/training/finetune_lora.py --config ml/training/configs/default.yaml
```

**File:** `scripts/eval_and_deploy.sh` — post-training pipeline:
```bash
#!/bin/bash
set -euo pipefail
python3 ml/training/merge_and_quantize.py --adapter-path output/k8s-sage-lora
python3 ml/training/eval.py --model-path output/k8s-sage-merged --test-file ml/dataset/data/training_data.jsonl
```

**File:** `docs/TESTING_RIG_RUNBOOK.md` — step-by-step for the Threadripper:
1. Clone repo
2. Install Python deps (`pip install -e ".[train]"`)
3. Generate dataset (if not committed): `python3 ml/dataset/generate_synthetic.py`
4. Rebuild dataset: `python3 ml/dataset/format_instruct.py`
5. Dry-run validation: `python3 ml/training/finetune_lora.py --dry-run`
6. Train: `scripts/train.sh` (3-6 hours, dual 3090)
7. Merge + quantize: `scripts/eval_and_deploy.sh`
8. Smoke test: `python3 ml/serving/test_inference.py`
9. Deploy to cluster: copy GGUF to Kind node, `make dev-deploy`
10. Dogfood v2: `make dogfood && sleep 1800 && make validate`

---

### Commit 5: Update `ml/README.md` status

Reflect current reality — all scripts complete, dataset ready, training ready to execute.

---

### Commit 6: Dataset quality report

**File:** `ml/dataset/reports/final_dataset_report.md`

Analysis of the full ~8,000-pair dataset:
- Source distribution breakdown
- Category distribution
- Average assistant response length by source
- Deduplication stats
- Train/eval split preview (90/10 = ~7,200 train / ~800 eval)

---

### Commit 7: Volume assessment + docs update

Update `ml/dataset/reports/volume_assessment.md` — mark synthetic as DONE, final totals.

---

### Commit 8: Day 4 status report

**File:** `docs/DAY4_STATUS_REPORT.md`

Session summary, commit log, final project state, what's ready for the testing rig.

---

## Commit Summary

| # | Type | Description | Effort |
|---|------|-------------|--------|
| 1 | `ml:` | Synthetic data generation script (~4,500 pairs) | 2-3 hours |
| 2 | `ml:` | Rebuild dataset pipeline with synthetic, remove cap | 15 min |
| 3 | `fix:` | Test stability (signal killed + assertion updates) | 30-60 min |
| 4 | `chore:` | Training scripts + testing rig runbook | 30 min |
| 5 | `docs:` | Update ml/README.md | 10 min |
| 6 | `docs:` | Dataset quality report | 20 min |
| 7 | `docs:` | Volume assessment update | 10 min |
| 8 | `docs:` | Day 4 status report | 20 min |

**Estimated total: 4-5 hours**

---

## What Happens on the Testing Rig (NOT this session)

| Step | Command | Time |
|------|---------|------|
| Dry-run validate | `python3 ml/training/finetune_lora.py --dry-run` | 1 min |
| Fine-tune Jamba 3B | `scripts/train.sh` | 3-6 hours |
| Merge LoRA + GGUF Q4 | `scripts/eval_and_deploy.sh` | 30 min |
| Evaluate held-out set | (included in eval_and_deploy.sh) | 30 min |
| Smoke test inference | `python3 ml/serving/test_inference.py` | 5 min |
| Deploy to Kind cluster | `make docker-build && make dev-deploy` | 10 min |
| Dogfood v2 (L1 fixes) | `make dogfood && sleep 1800 && make validate` | 30 min+ |
| Dogfood v3 (with L2) | Enable SLM in Helm, redeploy, validate | 30 min+ |

---

## What Is NOT In Scope (Post-Model, Post-Launch)

| Item | Status | When |
|------|--------|------|
| Slack integration | Designed (UX_RECOMMENDATION_FLOW.md) | After model validated |
| Grafana dashboards | Designed (DAY3_STATUS_REPORT.md) | After model validated |
| ArgoCD/Flux auto-detection | Designed (UX_RECOMMENDATION_FLOW.md) | After Slack |
| PR creation flow | Designed (UX_RECOMMENDATION_FLOW.md) | After GitOps |
| KWOK scale testing | Planned | Nice-to-have |
| Marketplace listing | Planned | Far future |

---

## Success Criteria

After this session, the repo should be clonable and the following should all work:

- [ ] `python3 ml/dataset/generate_synthetic.py` produces ~4,500 valid pairs
- [ ] `python3 ml/dataset/format_instruct.py` merges to ~8,000 total pairs
- [ ] `python3 ml/training/finetune_lora.py --dry-run` succeeds with full dataset
- [ ] `go test ./...` completes without signal kills
- [ ] `make lint` passes
- [ ] `helm lint deploy/helm/k8s-sage/` passes
- [ ] `scripts/train.sh` exists and is executable
- [ ] `docs/TESTING_RIG_RUNBOOK.md` has complete step-by-step instructions
- [ ] All changes committed with clean git history
