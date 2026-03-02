package server

import (
	"context"
	"log/slog"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/slm"
)

const (
	// defaultSLMTimeout is the maximum time to wait for an SLM response.
	defaultSLMTimeout = 10 * time.Second

	// safetyMemoryMultiplier is the minimum memory headroom above P99 WS.
	safetyMemoryMultiplier = 1.10

	// safetyCPUMultiplier is the minimum CPU headroom above P95.
	safetyCPUMultiplier = 1.10
)

// Analyzer orchestrates the L1 rules engine and optional L2 SLM for
// generating right-sizing recommendations.
type Analyzer struct {
	rules  *rules.Engine
	slm    *slm.Client // nil if SLM disabled
}

// NewAnalyzer creates a new Analyzer with the given rules engine and
// optional SLM client. Pass nil for slmClient to run L1-only.
func NewAnalyzer(engine *rules.Engine, slmClient *slm.Client) *Analyzer {
	return &Analyzer{
		rules: engine,
		slm:   slmClient,
	}
}

// Analyze runs L1 rules engine and optionally L2 SLM for a single container.
func (a *Analyzer) Analyze(ctx context.Context, input rules.AnalysisInput) rules.AnalysisResult {
	// L1: always run rules engine (< 1ms)
	l1Result := a.rules.Analyze(input)

	// L2: skip if SLM not configured
	if a.slm == nil {
		return l1Result
	}

	// Build prompt and call SLM
	prompt := slm.BuildPrompt(input)

	slmCtx, cancel := context.WithTimeout(ctx, defaultSLMTimeout)
	defer cancel()

	raw, err := a.slm.Complete(slmCtx, slm.CompletionRequest{
		Prompt:      prompt,
		MaxTokens:   512,
		Temperature: 0.1,
		Stop:        []string{"<|end|>", "</s>", "<|im_end|>"},
	})
	if err != nil {
		slog.Warn("SLM call failed, using L1 result",
			"owner", input.Owner.Kind+"/"+input.Owner.Name,
			"error", err)
		return l1Result
	}

	// Parse SLM output
	slmOutput, err := slm.ParseRecommendation(raw)
	if err != nil {
		slog.Warn("SLM output parse failed, using L1 result",
			"owner", input.Owner.Kind+"/"+input.Owner.Name,
			"error", err)
		return l1Result
	}

	// Validate L2 against safety invariants
	violations := checkSafetyInvariants(slmOutput, input)
	if len(violations) > 0 {
		slog.Warn("SLM recommendation failed safety checks, using L1 result",
			"owner", input.Owner.Kind+"/"+input.Owner.Name,
			"violations", violations)
		return l1Result
	}

	// Merge L2 recommendations with L1 safety floors
	return mergeL2Result(l1Result, slmOutput, input)
}

// checkSafetyInvariants validates SLM output against hard safety constraints.
func checkSafetyInvariants(output *slm.SLMOutput, input rules.AnalysisInput) []string {
	var violations []string

	// Parse SLM resource values
	cpuReqMillis, err := slm.ParseResourceMillis(output.CPURequest)
	if err != nil {
		violations = append(violations, "invalid cpu_request: "+err.Error())
		return violations
	}

	memReqBytes, err := slm.ParseResourceBytes(output.MemoryRequest)
	if err != nil {
		violations = append(violations, "invalid memory_request: "+err.Error())
		return violations
	}

	// No zero recommendations
	if cpuReqMillis <= 0 {
		violations = append(violations, "cpu_request is zero or negative")
	}
	if memReqBytes <= 0 {
		violations = append(violations, "memory_request is zero or negative")
	}

	// Memory >= P99 WS × 1.10
	memFloor := int64(input.MemUsageBytes.P99 * safetyMemoryMultiplier)
	if memReqBytes < memFloor {
		violations = append(violations, "memory_request below P99 working set safety floor")
	}

	// CPU >= P95 × headroom
	cpuFloor := int64(input.CPUUsageMillis.P95 * safetyCPUMultiplier)
	if cpuReqMillis < cpuFloor {
		violations = append(violations, "cpu_request below P95 usage safety floor")
	}

	return violations
}

// mergeL2Result merges SLM recommendations into the L1 result, using L1
// values as safety floors.
func mergeL2Result(l1 rules.AnalysisResult, l2 *slm.SLMOutput, input rules.AnalysisInput) rules.AnalysisResult {
	result := l1

	// Deep copy pointer fields to avoid mutating the original L1 result.
	if l1.CPURecommendation != nil {
		cpuCopy := *l1.CPURecommendation
		result.CPURecommendation = &cpuCopy
	}
	if l1.MemRecommendation != nil {
		memCopy := *l1.MemRecommendation
		result.MemRecommendation = &memCopy
	}

	// Map SLM pattern to model pattern
	if l2.Pattern != "" {
		result.Pattern = patternFromString(l2.Pattern)
	}

	// Merge CPU recommendation
	if result.CPURecommendation != nil {
		cpuReq, err := slm.ParseResourceMillis(l2.CPURequest)
		if err == nil {
			cpuLim, _ := slm.ParseResourceMillis(l2.CPULimit)

			// Use L2 values but enforce L1 as floor
			if cpuReq > l1.CPURecommendation.RecommendedReq {
				result.CPURecommendation.RecommendedReq = cpuReq
			}
			if cpuLim > 0 && cpuLim > l1.CPURecommendation.RecommendedLimit {
				result.CPURecommendation.RecommendedLimit = cpuLim
			}

			result.CPURecommendation.Pattern = result.Pattern
			result.CPURecommendation.Confidence = l2.Confidence
			if l2.Explanation != "" {
				result.CPURecommendation.Reasoning = l2.Explanation
			}
			if l2.Risk != "" {
				result.CPURecommendation.Risk = riskFromString(l2.Risk)
			}
		}
	}

	// Merge memory recommendation
	if result.MemRecommendation != nil {
		memReq, err := slm.ParseResourceBytes(l2.MemoryRequest)
		if err == nil {
			memLim, _ := slm.ParseResourceBytes(l2.MemoryLimit)

			if memReq > l1.MemRecommendation.RecommendedReq {
				result.MemRecommendation.RecommendedReq = memReq
			}
			if memLim > 0 && memLim > l1.MemRecommendation.RecommendedLimit {
				result.MemRecommendation.RecommendedLimit = memLim
			}

			result.MemRecommendation.Pattern = result.Pattern
			result.MemRecommendation.Confidence = l2.Confidence
			if l2.Explanation != "" {
				result.MemRecommendation.Reasoning = l2.Explanation
			}
			if l2.Risk != "" {
				result.MemRecommendation.Risk = riskFromString(l2.Risk)
			}
		}
	}

	return result
}

func patternFromString(s string) models.WorkloadPattern {
	switch s {
	case "steady":
		return models.PatternSteady
	case "burstable":
		return models.PatternBurstable
	case "batch":
		return models.PatternBatch
	case "idle":
		return models.PatternIdle
	case "anomalous":
		return models.PatternAnomalous
	default:
		return models.PatternSteady
	}
}

func riskFromString(s string) models.RiskLevel {
	switch s {
	case "LOW":
		return models.RiskLow
	case "MEDIUM":
		return models.RiskMedium
	case "HIGH":
		return models.RiskHigh
	default:
		return models.RiskMedium
	}
}
