package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gerty-labs/gerty/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleClusterReport() *models.ClusterReport {
	return &models.ClusterReport{
		ReportTime: time.Now(),
		NodeCount:  2,
		PodCount:   3,
		Namespaces: map[string]*models.NamespaceReport{
			"default": {
				Namespace:           "default",
				TotalCPUWasteMillis: 500,
				TotalMemWasteBytes:  500_000_000,
				Pods:                []models.PodWaste{{PodName: "pod-a"}, {PodName: "pod-b"}},
				Owners: []models.OwnerWaste{
					{
						Owner:               models.OwnerReference{Kind: "Deployment", Name: "web", Namespace: "default"},
						PodCount:            2,
						TotalCPUWasteMillis: 500,
						TotalMemWasteBytes:  500_000_000,
					},
				},
			},
		},
	}
}

func sampleRecommendations() []models.Recommendation {
	return []models.Recommendation{
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"},
			Container:      "main",
			Resource:       "cpu",
			CurrentRequest: 2000,
			RecommendedReq: 300,
			EstSavings:     1700,
			Risk:           models.RiskLow,
			Confidence:     0.95,
		},
		{
			Target:         models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"},
			Container:      "main",
			Resource:       "memory",
			CurrentRequest: 2_000_000_000,
			RecommendedReq: 400_000_000,
			EstSavings:     1_600_000_000,
			Risk:           models.RiskMedium,
			Confidence:     0.85,
		},
	}
}

func sampleWorkloads() []models.OwnerWaste {
	return []models.OwnerWaste{
		{
			Owner:               models.OwnerReference{Kind: "Deployment", Name: "web", Namespace: "default"},
			PodCount:            3,
			TotalCPUWasteMillis: 900,
			TotalMemWasteBytes:  900_000_000,
		},
	}
}

func TestWriteClusterReportTable(t *testing.T) {
	var buf bytes.Buffer
	writeClusterReportTable(&buf, sampleClusterReport())

	output := buf.String()
	assert.Contains(t, output, "NAMESPACE")
	assert.Contains(t, output, "OWNERS")
	assert.Contains(t, output, "PODS")
	assert.Contains(t, output, "CPU WASTE")
	assert.Contains(t, output, "MEM WASTE")
	assert.Contains(t, output, "default")
}

func TestWriteNamespaceReportTable(t *testing.T) {
	report := sampleClusterReport().Namespaces["default"]
	var buf bytes.Buffer
	writeNamespaceReportTable(&buf, report)

	output := buf.String()
	assert.Contains(t, output, "OWNER")
	assert.Contains(t, output, "KIND")
	assert.Contains(t, output, "web")
	assert.Contains(t, output, "Deployment")
}

func TestWriteRecommendationsTable(t *testing.T) {
	var buf bytes.Buffer
	writeRecommendationsTable(&buf, sampleRecommendations())

	output := buf.String()
	assert.Contains(t, output, "TARGET")
	assert.Contains(t, output, "CONTAINER")
	assert.Contains(t, output, "RESOURCE")
	assert.Contains(t, output, "CURRENT")
	assert.Contains(t, output, "RECOMMENDED")
	assert.Contains(t, output, "SAVINGS")
	assert.Contains(t, output, "RISK")
	assert.Contains(t, output, "CONFIDENCE")
	assert.Contains(t, output, "cpu")
	assert.Contains(t, output, "memory")

	// Memory rec has higher savings, should appear first.
	lines := strings.Split(output, "\n")
	var dataLines []string
	for _, l := range lines {
		if strings.Contains(l, "memory") || strings.Contains(l, "cpu") {
			dataLines = append(dataLines, l)
		}
	}
	require.Len(t, dataLines, 2)
	assert.Contains(t, dataLines[0], "memory", "higher savings should sort first")
}

func TestWriteWorkloadsTable(t *testing.T) {
	var buf bytes.Buffer
	writeWorkloadsTable(&buf, sampleWorkloads())

	output := buf.String()
	assert.Contains(t, output, "NAMESPACE")
	assert.Contains(t, output, "KIND")
	assert.Contains(t, output, "NAME")
	assert.Contains(t, output, "PODS")
	assert.Contains(t, output, "web")
}

func TestWriteWorkloadDetailTable(t *testing.T) {
	ow := &models.OwnerWaste{
		Owner:               models.OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"},
		PodCount:            2,
		TotalCPUWasteMillis: 1800,
		TotalMemWasteBytes:  1_800_000_000,
		Containers: []models.ContainerWaste{
			{
				ContainerName:      "main",
				CPURequestMillis:   2000,
				CPUUsage:           models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 400},
				CPUWastePercent:    90,
				MemoryRequestBytes: 2_000_000_000,
				MemoryUsage:        models.MetricAggregate{P50: 100_000_000, P95: 200_000_000, P99: 300_000_000, Max: 400_000_000},
				MemWastePercent:    90,
			},
		},
	}

	var buf bytes.Buffer
	writeWorkloadDetailTable(&buf, ow)

	output := buf.String()
	assert.Contains(t, output, "prod/api")
	assert.Contains(t, output, "Deployment")
	assert.Contains(t, output, "CONTAINER")
	assert.Contains(t, output, "main")
}

func TestPrintJSON_ValidOutput(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	err := printJSON(&buf, data)
	require.NoError(t, err)

	// Should be valid JSON.
	var parsed map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "value", parsed["key"])
}

func TestPrintJSON_IndentedOutput(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"key": "value"}
	err := printJSON(&buf, data)
	require.NoError(t, err)

	// Should be indented (contains newlines beyond the trailing one).
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Greater(t, len(lines), 1, "JSON should be pretty-printed with indentation")
}
