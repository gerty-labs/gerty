# Week 2.5 — Testing, Dogfooding & Validation Plan

**Timeline**: Mar 12 – Mar 15 (overlaps with end of Week 2 packaging)
**Goal**: Prove k8s-sage works correctly, gives accurate recommendations, and doesn't break the clusters it's meant to help.

---

## 0. Test Integrity Audit — Testing the Tests

**This comes first. Before any integration or dogfood testing, verify that the existing unit tests are actually testing what they claim to test.**

AI code assistants (including Claude Code) have a documented tendency to silently weaken test assertions or add special-case logic to make failing tests pass, rather than fixing the underlying implementation bug. This creates the illusion of coverage while masking real defects. Phase 0 catches this before it compounds.

### Audit Process

For every `*_test.go` file in the repo, manually review the following:

#### A. Assertion Strength

- [ ] Tests assert **specific expected values**, not just "no error" or "not nil"
- [ ] Floating-point comparisons use tight tolerances (not `assert.InDelta(t, expected, actual, 999)`)
- [ ] Slice/map assertions check **length AND contents**, not just one
- [ ] Error path tests assert the **specific error type or message**, not just `err != nil`
- [ ] No `t.Skip()` calls without a documented reason

#### B. Test Independence

- [ ] Each test case sets up its own state — no reliance on execution order
- [ ] No shared mutable state between test functions
- [ ] Table-driven tests cover boundary values, not just happy paths
- [ ] Subtests are named descriptively enough to identify failures without reading code

#### C. Anti-Patterns to Hunt For

| Anti-Pattern | What It Looks Like | Why It's Dangerous |
|-------------|-------------------|-------------------|
| Tautological test | `assert.Equal(t, result, result)` | Tests nothing |
| Assertion-free test | Function runs but never asserts | Passes by default |
| Over-mocked reality | Every dependency mocked, no real logic exercised | Tests the mocks, not the code |
| Copied implementation | Test reimplements the function under test, compares outputs | If both are wrong, test still passes |
| Magic tolerance | `assert.InDelta(t, 100, actual, 50)` | 50% tolerance is not a test |
| Swallowed errors | `result, _ := SomeFunc()` in test code | Ignoring the thing most likely to fail |
| Conditional assertions | `if result != nil { assert.Equal(...) }` | Silently skips assertion when result is nil |
| Test that matches implementation | Assertion values clearly derived from running the code, not from independent calculation | Circular reasoning — tests whatever the code does, not what it should do |

#### D. Rules Engine Specific Checks

The rules engine is the most critical component to audit because wrong recommendations erode trust:

- [ ] Classification thresholds are tested at **exact boundaries** (CV = 0.29, 0.30, 0.31)
- [ ] Recommendation values are verified against **hand-calculated expected outputs**, not code outputs
- [ ] Confidence scores are tested at every data window boundary (1 day, 3 days, 7 days)
- [ ] Headroom factors are tested per pattern type — steady gets less headroom than burstable
- [ ] Safety invariants are asserted: `recommended >= P95 * min_headroom` for every test case
- [ ] Edge cases have their own test functions, not buried in table-driven tests where they're easy to miss

#### E. Mutation Testing (Optional but Recommended)

Run a mutation testing pass to verify test effectiveness:

```bash
# Install go-mutesting
go install github.com/zimmski/go-mutesting/cmd/go-mutesting@latest

# Run against critical packages
go-mutesting ./internal/rules/...
go-mutesting ./internal/agent/...
```

**Target mutation score**: 70%+ for `rules/`, 60%+ for `agent/`. If mutants survive (code is changed and tests still pass), those are gaps.

### Remediation Rules

When a weak test is found:

1. **Fix the test first** — strengthen the assertion to catch the real behaviour
2. **Then check if the implementation is actually correct** — the test may have been weak because the code was wrong
3. **Never weaken a test to make it pass** — if a test fails, the implementation must change
4. **Document any intentional tolerance** — if a loose bound is genuinely correct, add a comment explaining why

### Claude Code Directive

Add this to any Claude Code prompt involving test fixes:

```
CRITICAL: If a test fails, fix the IMPLEMENTATION, not the test.
Do not weaken assertions, widen tolerances, add special cases to tests,
or skip tests to make the suite pass. If you believe a test expectation
is genuinely wrong, explain why BEFORE changing it and get confirmation.
```

### Sign-Off

- [ ] All `*_test.go` files reviewed against checklist above
- [ ] Zero tautological, assertion-free, or conditional-assertion tests
- [ ] All rules engine boundary tests verified against hand calculations
- [ ] Mutation testing score meets threshold (if run)
- [ ] Any test weaknesses found have been remediated

**Phase 0 must be complete before proceeding to Phase 1 (dogfooding).**

---

## 1. Local Dogfooding — kind Cluster

### Setup

Spin up a multi-node kind cluster with realistic workloads that cover all four classification patterns:

```bash
# kind cluster with 3 worker nodes
kind create cluster --name sage-test --config test/kind-config.yaml
```

### Synthetic Workloads to Deploy

| Workload | Type | What It Simulates | Expected Classification |
|----------|------|-------------------|------------------------|
| `nginx-overprovisioned` | Deployment (3 replicas) | Web server requesting 1 CPU / 1Gi but using ~50m / 80Mi | STEADY, high waste |
| `api-bursty` | Deployment (2 replicas) | Simulated API with periodic load spikes (hey/vegeta) | BURSTABLE |
| `cronjob-batch` | CronJob (every 5min) | CPU-intensive task that spins up, burns, dies | BATCH |
| `idle-dev` | Deployment (1 replica) | Sleep container with 500m CPU / 512Mi requested | IDLE |
| `java-app` | Deployment (1 replica) | JVM app with GC heap pressure, memory looks high but is normal | STEADY (edge case — must not over-trim memory) |
| `memory-leak` | Deployment (1 replica) | Simulated slow memory growth over time | Should flag as anomalous, low confidence |
| `right-sized` | Deployment (1 replica) | Workload already running close to requests | Should recommend no change or minimal change |
| `best-effort` | Deployment (1 replica) | No resource requests/limits set | Should report but not recommend (no baseline) |

### kind-config.yaml

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
  - role: worker
  - role: worker
  - role: worker
```

### Test Harness Scripts

All scripts live in `test/dogfood/`:

| Script | Purpose |
|--------|---------|
| `setup-workloads.sh` | Deploys all synthetic workloads with known resource requests |
| `generate-load.sh` | Runs hey/vegeta against bursty workloads to create realistic patterns |
| `validate-classifications.sh` | Waits for data window, then checks classifications match expectations |
| `validate-recommendations.sh` | Checks recommendations are sane (not recommending 0, not exceeding current) |
| `validate-savings.sh` | Checks savings math: `(current - recommended) * cost_per_unit` adds up |
| `teardown.sh` | Cleans up cluster |

---

## 2. Backtesting — Rules Engine Accuracy

### Approach

Feed the rules engine known metric patterns with predetermined correct answers, then measure accuracy.

### Test Dataset

Create `test/fixtures/backtest_scenarios.json` with 50+ scenarios covering:

**Steady workloads (15 scenarios)**
- Low utilisation (10-30% of request) → should recommend significant reduction
- Moderate utilisation (50-70%) → should recommend modest reduction
- High utilisation (80-95%) → should recommend minimal or no change
- Already right-sized (P95 within 10% of request) → should recommend no change

**Burstable workloads (10 scenarios)**
- Regular spikes to 80%+ with low baseline → should set request near baseline, limit near spike
- Irregular spikes → lower confidence score
- Spikes exceeding current limits → should recommend limit increase (this is a real risk catch)

**Batch workloads (10 scenarios)**
- Short-lived high-CPU jobs → should not over-provision for idle time between runs
- Long-running batch with consistent usage → treat more like steady

**Idle workloads (5 scenarios)**
- Zero usage for 48h+ → should flag as idle, recommend scale-to-zero or removal
- Near-zero but not zero → should still classify as idle

**Edge cases (10+ scenarios)**
- Single data point (just deployed) → confidence must be ≤0.5
- Exactly on classification threshold (CV = 0.3) → deterministic, no flapping
- Negative values (shouldn't happen but must not panic)
- BestEffort pods (no requests) → skip recommendation, report only
- Init container skewing metrics → should be excluded from steady-state analysis
- Pod in CrashLoopBackOff → erratic metrics, must not recommend based on restart spikes

### Validation Criteria

```
Pass criteria for each scenario:
  ✓ Classification matches expected pattern
  ✓ Recommended CPU is between P95 and current request (with headroom)
  ✓ Recommended memory is between P95_working_set and current request (with headroom)
  ✓ Confidence is within expected range for data window
  ✓ Risk level is correct for the gap between recommended and P99
  ✓ Savings estimate is mathematically correct
  ✓ Reasoning string is non-empty and references the pattern
```

### Running Backtests

```bash
make backtest        # Runs all 50+ scenarios, outputs pass/fail table
make backtest-report # Generates markdown report with accuracy metrics
```

**Target**: 95%+ classification accuracy, 100% on edge case safety (never recommend something that would OOMKill or throttle a correctly-running workload).

---

## 3. Integration Tests — End-to-End Pipeline

### What Gets Tested

| Test | Description | Pass Criteria |
|------|-------------|---------------|
| Agent → Server ingest | Agent pushes report, server receives and stores | Server store contains all pods from agent report |
| Multi-agent ingest | 3 agents push simultaneously | No data races, all nodes represented |
| Stale pod eviction | Stop reporting a pod, wait 15min | Pod pruned from server store |
| Report endpoint accuracy | Compare `/api/v1/report` output against known workload state | Waste percentages within 5% of expected |
| Recommendation endpoint | Hit `/api/v1/recommendations` after data window | Returns valid recommendations for all non-BestEffort workloads |
| Namespace filtering | Query `?namespace=overprovisioned` | Only returns pods in that namespace |
| CLI output | Run `sage report` and `sage recommend` | Parses without error, output matches API response |
| Helm install/uninstall | `helm install` then `helm uninstall` | Clean install, clean removal, no orphaned resources |
| Agent resource usage | Monitor agent pod during test run | Never exceeds 50Mi RAM or 50m CPU |

### Running Integration Tests

```bash
# Full suite (requires kind cluster)
make dev-cluster
make dev-deploy
make test-integration

# Individual
go test -v -tags=integration ./test/integration/...
```

---

## 4. Stress Testing

### Agent Limits

| Test | Method | Pass Criteria |
|------|--------|---------------|
| High pod density | Deploy 300 pods on a single node | Agent stays under 50Mi, no data loss |
| Rapid churn | Create/delete 50 pods per minute | Store handles tombstoning, no memory leak |
| Kubelet timeout | Inject 10s latency on kubelet API | Agent retries gracefully, doesn't pile up goroutines |
| Server unreachable | Kill server, agent keeps collecting | Agent buffers locally, pushes when server returns |

### Server Limits

| Test | Method | Pass Criteria |
|------|--------|---------------|
| Large cluster simulation | 50 agents pushing 100 pods each (5000 pods) | Server responds to /report within 2s |
| Concurrent API requests | 100 concurrent GET /api/v1/recommendations | No panics, no data races, p99 < 500ms |
| Memory ceiling | Run for 24h with pod churn | Server memory stable, eviction working |

---

## 5. Recommendation Safety Validation

This is the most critical section. A bad recommendation could OOMKill production workloads.

### Safety Rules (Must Never Violate)

1. **Never recommend memory below P99 working set** — OOMKill risk
2. **Never recommend CPU below P95 for steady workloads** — throttling risk
3. **Never recommend 0 for any resource** — even idle pods need a floor
4. **Headroom must scale with risk** — burstable workloads get more headroom than steady
5. **Confidence below 0.5 must include a warning** — insufficient data
6. **Batch workloads must not be sized based on idle periods** — peak usage matters

### Safety Test Suite

```bash
make test-safety   # Runs dedicated safety assertions
```

This suite runs every scenario through the recommendation engine and asserts:
- `recommended_cpu >= P95_cpu * headroom_factor` (headroom varies by pattern)
- `recommended_memory >= P99_working_set * 1.1` (10% minimum memory buffer, always)
- `recommended_cpu > 0 && recommended_memory > 0` (always)
- If `confidence < 0.5`, the `reasoning` field contains "insufficient data" or equivalent
- If `risk == HIGH`, the `reasoning` field contains an explicit warning

---

## 6. Output Validation — Human Review

Before declaring v0.1 ready:

### Checklist

- [ ] Deploy to kind cluster with all 8 synthetic workloads
- [ ] Let it collect for 2+ hours (enough for meaningful data window)
- [ ] Run `sage report` — does the waste summary look right?
- [ ] Run `sage recommend` — would you trust these recommendations?
- [ ] Manually apply one LOW-risk recommendation — does the workload survive?
- [ ] Check agent resource usage — actually under 50Mi/50m?
- [ ] Check server resource usage — reasonable for what it's doing?
- [ ] Run `helm uninstall` — is the cluster clean?
- [ ] Review all HIGH-risk recommendations — are the warnings clear?
- [ ] Review all low-confidence recommendations — is the reasoning honest about data gaps?

---

## 7. CI Integration

Add to `.github/workflows/ci.yaml`:

```yaml
jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: make lint
      - run: make test

  backtest:
    runs-on: ubuntu-latest
    needs: unit-tests
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: make backtest

  integration:
    runs-on: ubuntu-latest
    needs: backtest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - uses: helm/kind-action@v1
      - run: make dev-deploy
      - run: make test-integration
```

---

## Definition of Done — v0.1

All of the following must pass before tagging v0.1:

- [ ] `make test` — all unit tests pass with `-race`
- [ ] `make backtest` — 95%+ classification accuracy, 100% safety compliance
- [ ] `make test-integration` — full pipeline works in kind
- [ ] `make test-safety` — zero safety violations across all scenarios
- [ ] Agent resource usage verified under budget (50Mi / 50m)
- [ ] Manual dogfood review completed (checklist above)
- [ ] `go vet ./...` clean
- [ ] No TODO or FIXME in critical paths (rules engine, recommendations)
- [ ] README documents installation, usage, and limitations honestly
