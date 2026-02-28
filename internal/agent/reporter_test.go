package agent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReporter_HandleReport_EmptyStore(t *testing.T) {
	store := NewStore()
	reporter := NewReporter("test-node", store)

	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	w := httptest.NewRecorder()

	reporter.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var report models.NodeReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)

	assert.Equal(t, "test-node", report.NodeName)
	assert.Empty(t, report.Pods)
	assert.Equal(t, float64(0), report.TotalCPUWasteMillis)
}

func TestReporter_HandleReport_WithData(t *testing.T) {
	store := NewStore()

	// Record samples with requests so waste can be calculated.
	store.Record(models.ContainerMetrics{
		PodName:               "nginx-abc",
		PodNamespace:          "default",
		ContainerName:         "nginx",
		Timestamp:             time.Now(),
		CPUUsageNanoCores:     150_000_000,    // 150m actual
		CPURequestMillis:      1000,            // 1000m requested
		MemoryWorkingSetBytes: 100_000_000,     // ~95Mi actual
		MemoryRequestBytes:    500_000_000,     // ~476Mi requested
		QoSClass:              "Burstable",
	})

	reporter := NewReporter("test-node", store)

	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	w := httptest.NewRecorder()

	reporter.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.NodeReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)

	assert.Equal(t, "test-node", report.NodeName)
	require.Len(t, report.Pods, 1)

	pod := report.Pods[0]
	assert.Equal(t, "nginx-abc", pod.PodName)
	assert.Equal(t, "default", pod.PodNamespace)
	assert.Equal(t, "Burstable", pod.QoSClass)
	require.Len(t, pod.Containers, 1)

	container := pod.Containers[0]
	assert.Equal(t, "nginx", container.ContainerName)
	assert.Equal(t, int64(1000), container.CPURequestMillis)

	// CPU: request 1000m, usage 150m → waste 850m (85%).
	assert.Equal(t, float64(850), container.CPUWasteMillis)
	assert.InDelta(t, 85.0, container.CPUWastePercent, 0.1)

	// Memory: request 500MB, working set 100MB → waste 400MB (80%).
	assert.Equal(t, float64(400_000_000), container.MemWasteBytes)
	assert.InDelta(t, 80.0, container.MemWastePercent, 0.1)
}

func TestReporter_HandleReport_MultipleContainersPerPod(t *testing.T) {
	store := NewStore()
	now := time.Now()

	store.Record(models.ContainerMetrics{
		PodName: "api-pod", PodNamespace: "prod", ContainerName: "api",
		Timestamp: now, CPUUsageNanoCores: 500_000_000, CPURequestMillis: 2000,
		MemoryWorkingSetBytes: 400_000_000, MemoryRequestBytes: 1_000_000_000,
	})
	store.Record(models.ContainerMetrics{
		PodName: "api-pod", PodNamespace: "prod", ContainerName: "sidecar",
		Timestamp: now, CPUUsageNanoCores: 10_000_000, CPURequestMillis: 200,
		MemoryWorkingSetBytes: 20_000_000, MemoryRequestBytes: 100_000_000,
	})

	reporter := NewReporter("node-1", store)
	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	w := httptest.NewRecorder()

	reporter.HandleReport(w, req)

	var report models.NodeReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)

	require.Len(t, report.Pods, 1)
	assert.Len(t, report.Pods[0].Containers, 2)

	// Total pod waste should be sum of container wastes.
	assert.Greater(t, report.Pods[0].TotalCPUWasteMillis, float64(0))
	assert.Greater(t, report.Pods[0].TotalMemWasteBytes, float64(0))
}

func TestReporter_HandleReport_MethodNotAllowed(t *testing.T) {
	store := NewStore()
	reporter := NewReporter("test-node", store)

	req := httptest.NewRequest(http.MethodPost, "/report", nil)
	w := httptest.NewRecorder()

	reporter.HandleReport(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestReporter_HandleReport_NoWasteWhenNoRequests(t *testing.T) {
	store := NewStore()

	// Record sample with zero requests — waste should be zero.
	store.Record(models.ContainerMetrics{
		PodName: "besteffort-pod", PodNamespace: "default", ContainerName: "app",
		Timestamp: time.Now(), CPUUsageNanoCores: 100_000_000,
		CPURequestMillis: 0, MemoryRequestBytes: 0,
	})

	reporter := NewReporter("test-node", store)
	req := httptest.NewRequest(http.MethodGet, "/report", nil)
	w := httptest.NewRecorder()

	reporter.HandleReport(w, req)

	var report models.NodeReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)

	require.Len(t, report.Pods, 1)
	container := report.Pods[0].Containers[0]
	assert.Equal(t, float64(0), container.CPUWasteMillis)
	assert.Equal(t, float64(0), container.CPUWastePercent)
	assert.Equal(t, float64(0), container.MemWasteBytes)
}

func TestComputeContainerWaste(t *testing.T) {
	tests := []struct {
		name            string
		cpuRequestMilli int64
		cpuP95Nano      float64
		memRequestBytes int64
		memP95WS        float64
		wantCPUWaste    float64
		wantMemWaste    float64
	}{
		{
			name:            "significant waste",
			cpuRequestMilli: 2000,
			cpuP95Nano:      300_000_000, // 300m
			memRequestBytes: 1_000_000_000,
			memP95WS:        200_000_000,
			wantCPUWaste:    1700,
			wantMemWaste:    800_000_000,
		},
		{
			name:            "no waste — usage exceeds request",
			cpuRequestMilli: 100,
			cpuP95Nano:      200_000_000, // 200m > 100m request
			memRequestBytes: 100_000_000,
			memP95WS:        150_000_000,
			wantCPUWaste:    0,
			wantMemWaste:    0,
		},
		{
			name:            "zero requests",
			cpuRequestMilli: 0,
			cpuP95Nano:      100_000_000,
			memRequestBytes: 0,
			memP95WS:        50_000_000,
			wantCPUWaste:    0,
			wantMemWaste:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := models.AggregatedMetrics{
				CPUNanoCores: models.MetricAggregate{
					P50: tt.cpuP95Nano * 0.5,
					P95: tt.cpuP95Nano,
					P99: tt.cpuP95Nano * 1.1,
					Max: tt.cpuP95Nano * 1.2,
				},
				MemoryWorkingSet: models.MetricAggregate{
					P50: tt.memP95WS * 0.5,
					P95: tt.memP95WS,
					P99: tt.memP95WS * 1.1,
					Max: tt.memP95WS * 1.2,
				},
			}
			meta := ContainerMeta{
				ContainerName:    "test",
				CPURequestMillis: tt.cpuRequestMilli,
				MemRequestBytes:  tt.memRequestBytes,
			}

			cw := computeContainerWaste(summary, meta, 24*time.Hour)
			assert.Equal(t, tt.wantCPUWaste, cw.CPUWasteMillis)
			assert.Equal(t, tt.wantMemWaste, cw.MemWasteBytes)
		})
	}
}
