package slm

import (
	"strings"
	"testing"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/stretchr/testify/assert"
)

func TestBuildPrompt_SteadyWorkload(t *testing.T) {
	input := rules.AnalysisInput{
		Owner: models.OwnerReference{
			Kind:      "Deployment",
			Name:      "nginx-web",
			Namespace: "production",
		},
		ContainerName: "nginx",
		CPUUsageMillis: models.MetricAggregate{
			P50: 120, P95: 180, P99: 250, Max: 400,
		},
		CPURequestMillis: 1000,
		CPULimitMillis:   2000,
		MemUsageBytes: models.MetricAggregate{
			P50: 180 * 1024 * 1024,
			P95: 200 * 1024 * 1024,
			P99: 220 * 1024 * 1024,
			Max: 250 * 1024 * 1024,
		},
		MemRequestBytes:   512 * 1024 * 1024,
		MemLimitBytes:     1024 * 1024 * 1024,
		DataWindowMinutes: 7 * 24 * 60, // 7 days
	}

	prompt := BuildPrompt(input)

	// Check structure
	assert.True(t, strings.HasPrefix(prompt, "<|system|>"))
	assert.Contains(t, prompt, "<|user|>")
	assert.True(t, strings.HasSuffix(prompt, "<|assistant|>\n"))

	// Check workload identification
	assert.Contains(t, prompt, "Deployment/nginx-web in namespace production")
	assert.Contains(t, prompt, "Container: nginx")

	// Check CPU metrics
	assert.Contains(t, prompt, "Request=1000m, Limit=2000m")
	assert.Contains(t, prompt, "P50=120m, P95=180m, P99=250m, Max=400m")

	// Check memory metrics
	assert.Contains(t, prompt, "Request=512Mi, Limit=1024Mi")
	assert.Contains(t, prompt, "P50=180Mi")

	// Check data window
	assert.Contains(t, prompt, "Data window: 7 days")
}

func TestBuildPrompt_ShortDataWindow(t *testing.T) {
	input := rules.AnalysisInput{
		Owner: models.OwnerReference{
			Kind:      "StatefulSet",
			Name:      "redis",
			Namespace: "cache",
		},
		ContainerName:     "redis",
		CPUUsageMillis:    models.MetricAggregate{P50: 50, P95: 80, P99: 100, Max: 120},
		CPURequestMillis:  200,
		CPULimitMillis:    400,
		MemUsageBytes:     models.MetricAggregate{P50: 100 * 1024 * 1024, P95: 110 * 1024 * 1024, P99: 115 * 1024 * 1024, Max: 120 * 1024 * 1024},
		MemRequestBytes:   256 * 1024 * 1024,
		MemLimitBytes:     512 * 1024 * 1024,
		DataWindowMinutes: 12 * 60, // 12 hours
	}

	prompt := BuildPrompt(input)
	assert.Contains(t, prompt, "Data window: 12 hours")
	assert.Contains(t, prompt, "StatefulSet/redis")
}

func TestBuildPrompt_ContainsSystemPrompt(t *testing.T) {
	input := rules.AnalysisInput{
		Owner:             models.OwnerReference{Kind: "Deployment", Name: "test", Namespace: "default"},
		ContainerName:     "app",
		CPUUsageMillis:    models.MetricAggregate{},
		MemUsageBytes:     models.MetricAggregate{},
		DataWindowMinutes: 60,
	}

	prompt := BuildPrompt(input)
	assert.Contains(t, prompt, "k8s-sage")
	assert.Contains(t, prompt, "cpu_request")
	assert.Contains(t, prompt, "memory_request")
	assert.Contains(t, prompt, "JSON")
}
