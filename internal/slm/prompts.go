package slm

import (
	"fmt"
	"strings"

	"github.com/gregorytcarroll/k8s-sage/internal/rules"
)

const systemPrompt = `You are k8s-sage, a Kubernetes resource efficiency specialist. Analyse the provided workload metrics and respond with a JSON object containing exactly these fields:
- cpu_request: recommended CPU request (e.g. "250m")
- cpu_limit: recommended CPU limit (e.g. "500m")
- memory_request: recommended memory request (e.g. "256Mi")
- memory_limit: recommended memory limit (e.g. "512Mi")
- pattern: workload pattern ("steady", "burstable", "batch", or "idle")
- confidence: float 0-1
- reasoning_code: short code for the reasoning (e.g. "OVER_PROVISIONED", "RIGHT_SIZED")
- explanation: human-readable explanation of the recommendation
- risk: risk level ("LOW", "MEDIUM", or "HIGH")

Respond ONLY with the JSON object, no additional text.`

// truncate limits a string to maxLen characters for safe prompt interpolation.
func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// BuildPrompt constructs the full prompt for the SLM from workload data.
// The format matches the instruction-tuning template used during training.
func BuildPrompt(input rules.AnalysisInput) string {
	var b strings.Builder

	b.WriteString("<|system|>\n")
	b.WriteString(systemPrompt)
	b.WriteString("\n<|user|>\n")

	// Workload identification — truncate to K8s max lengths for defense-in-depth.
	fmt.Fprintf(&b, "Workload: %s/%s in namespace %s\n",
		truncate(input.Owner.Kind, 63), truncate(input.Owner.Name, 253), truncate(input.Owner.Namespace, 63))
	fmt.Fprintf(&b, "Container: %s\n", truncate(input.ContainerName, 63))

	// CPU metrics
	fmt.Fprintf(&b, "CPU: Request=%dm, Limit=%dm\n",
		input.CPURequestMillis, input.CPULimitMillis)
	fmt.Fprintf(&b, "  Usage: P50=%.0fm, P95=%.0fm, P99=%.0fm, Max=%.0fm\n",
		input.CPUUsageMillis.P50, input.CPUUsageMillis.P95,
		input.CPUUsageMillis.P99, input.CPUUsageMillis.Max)

	// Memory metrics
	memReqMi := input.MemRequestBytes / (1024 * 1024)
	memLimMi := input.MemLimitBytes / (1024 * 1024)
	fmt.Fprintf(&b, "Memory: Request=%dMi, Limit=%dMi\n", memReqMi, memLimMi)
	fmt.Fprintf(&b, "  Usage: P50=%.0fMi, P95=%.0fMi, P99=%.0fMi, Max=%.0fMi\n",
		input.MemUsageBytes.P50/(1024*1024),
		input.MemUsageBytes.P95/(1024*1024),
		input.MemUsageBytes.P99/(1024*1024),
		input.MemUsageBytes.Max/(1024*1024))

	// Data window
	hours := input.DataWindowMinutes / 60
	if hours >= 24 {
		fmt.Fprintf(&b, "Data window: %.0f days\n", hours/24)
	} else {
		fmt.Fprintf(&b, "Data window: %.0f hours\n", hours)
	}

	b.WriteString("<|assistant|>\n")

	return b.String()
}
