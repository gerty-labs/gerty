package models

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ContainerMetrics ---

func TestContainerMetrics_JSONRoundTrip(t *testing.T) {
	ts := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	cm := ContainerMetrics{
		PodName:               "web-abc123",
		PodNamespace:          "default",
		ContainerName:         "nginx",
		Timestamp:             ts,
		CPUUsageNanoCores:     250_000_000,
		CPURequestMillis:      500,
		CPULimitMillis:        1000,
		MemoryUsageBytes:      134_217_728,
		MemoryWorkingSetBytes: 100_000_000,
		MemoryRequestBytes:    268_435_456,
		MemoryLimitBytes:      536_870_912,
		RestartCount:          2,
		QoSClass:              "Burstable",
		OwnerKind:             "Deployment",
		OwnerName:             "web",
	}

	data, err := json.Marshal(cm)
	require.NoError(t, err)

	var decoded ContainerMetrics
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, cm, decoded)
}

func TestContainerMetrics_SnakeCaseFields(t *testing.T) {
	cm := ContainerMetrics{
		PodName:       "test-pod",
		PodNamespace:  "ns",
		ContainerName: "app",
	}

	data, err := json.Marshal(cm)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	expectedFields := []string{
		"podName", "podNamespace", "containerName", "timestamp",
		"cpuUsageNanoCores", "cpuRequestMillis", "cpuLimitMillis",
		"memoryUsageBytes", "memoryWorkingSetBytes", "memoryRequestBytes", "memoryLimitBytes",
		"restartCount", "qosClass",
	}
	for _, f := range expectedFields {
		_, ok := raw[f]
		assert.True(t, ok, "expected JSON field %q", f)
	}
}

func TestContainerMetrics_OmitemptyOwner(t *testing.T) {
	cm := ContainerMetrics{PodName: "standalone"}
	data, err := json.Marshal(cm)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	_, hasOwnerKind := raw["ownerKind"]
	_, hasOwnerName := raw["ownerName"]
	assert.False(t, hasOwnerKind, "ownerKind should be omitted when empty")
	assert.False(t, hasOwnerName, "ownerName should be omitted when empty")
}

// --- MetricAggregate ---

func TestMetricAggregate_JSONRoundTrip(t *testing.T) {
	ma := MetricAggregate{P50: 100.5, P95: 250.3, P99: 400.7, Max: 500.0}

	data, err := json.Marshal(ma)
	require.NoError(t, err)

	var decoded MetricAggregate
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ma, decoded)
}

func TestMetricAggregate_ZeroValues(t *testing.T) {
	ma := MetricAggregate{}
	data, err := json.Marshal(ma)
	require.NoError(t, err)

	var decoded MetricAggregate
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, float64(0), decoded.P50)
	assert.Equal(t, float64(0), decoded.Max)
}

// --- AggregatedMetrics ---

func TestAggregatedMetrics_JSONRoundTrip(t *testing.T) {
	am := AggregatedMetrics{
		PodName:       "worker-xyz",
		PodNamespace:  "batch",
		ContainerName: "processor",
		BucketStart:   time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
		BucketEnd:     time.Date(2025, 1, 15, 10, 5, 0, 0, time.UTC),
		SampleCount:   30,
		CPUNanoCores:  MetricAggregate{P50: 1e9, P95: 2e9, P99: 3e9, Max: 4e9},
		MemoryUsageBytes: MetricAggregate{P50: 1e8, P95: 2e8, P99: 3e8, Max: 4e8},
		MemoryWorkingSet: MetricAggregate{P50: 8e7, P95: 1.5e8, P99: 2.5e8, Max: 3.5e8},
	}

	data, err := json.Marshal(am)
	require.NoError(t, err)

	var decoded AggregatedMetrics
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, am, decoded)
}

// --- Recommendation ---

func TestRecommendation_JSONRoundTrip(t *testing.T) {
	rec := Recommendation{
		Target:           OwnerReference{Kind: "Deployment", Name: "api", Namespace: "prod"},
		Container:        "main",
		Resource:         "cpu",
		CurrentRequest:   1000,
		CurrentLimit:     2000,
		RecommendedReq:   500,
		RecommendedLimit: 1000,
		Pattern:          PatternSteady,
		Confidence:       0.85,
		Reasoning:        "P95 is well below request",
		EstSavings:       500,
		Risk:             RiskLow,
		DataWindow:       24 * time.Hour,
	}

	data, err := json.Marshal(rec)
	require.NoError(t, err)

	var decoded Recommendation
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, rec, decoded)
}

func TestRecommendation_FieldNames(t *testing.T) {
	rec := Recommendation{Resource: "memory"}
	data, err := json.Marshal(rec)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	expectedFields := []string{
		"target", "container", "resource",
		"currentRequest", "currentLimit",
		"recommendedRequest", "recommendedLimit",
		"pattern", "confidence", "reasoning",
		"estimatedSavings", "risk", "dataWindow",
	}
	for _, f := range expectedFields {
		_, ok := raw[f]
		assert.True(t, ok, "expected JSON field %q", f)
	}
}

// --- PodWaste ---

func TestPodWaste_JSONRoundTrip(t *testing.T) {
	pw := PodWaste{
		PodName:      "web-abc",
		PodNamespace: "default",
		QoSClass:     "Burstable",
		OwnerRef:     OwnerReference{Kind: "Deployment", Name: "web", Namespace: "default"},
		Containers: []ContainerWaste{
			{ContainerName: "nginx", CPUWasteMillis: 100, MemWasteBytes: 1024},
		},
		TotalCPUWasteMillis: 100,
		TotalMemWasteBytes:  1024,
	}

	data, err := json.Marshal(pw)
	require.NoError(t, err)

	var decoded PodWaste
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, pw, decoded)
}

func TestPodWaste_OwnerRefPresent(t *testing.T) {
	// Go's encoding/json emits zero-value structs even with omitempty
	// (only pointer/slice/map types are omitted). Verify the field is present.
	pw := PodWaste{PodName: "standalone-pod"}
	data, err := json.Marshal(pw)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	_, hasOwnerRef := raw["ownerRef"]
	assert.True(t, hasOwnerRef, "ownerRef struct should be present even when zero-value")
}

func TestPodWaste_EmptyContainersSlice(t *testing.T) {
	pw := PodWaste{PodName: "empty", Containers: []ContainerWaste{}}
	data, err := json.Marshal(pw)
	require.NoError(t, err)

	var decoded PodWaste
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Empty(t, decoded.Containers)
}

// --- ContainerWaste ---

func TestContainerWaste_JSONRoundTrip(t *testing.T) {
	cw := ContainerWaste{
		ContainerName:      "app",
		CPURequestMillis:   500,
		CPUUsage:           MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 400},
		CPUWasteMillis:     300,
		CPUWastePercent:    60.0,
		MemoryRequestBytes: 1 << 30,
		MemoryUsage:        MetricAggregate{P50: 500e6, P95: 700e6, P99: 800e6, Max: 900e6},
		MemWasteBytes:      300e6,
		MemWastePercent:    28.0,
		RestartCount:       1,
		DataWindow:         2 * time.Hour,
	}

	data, err := json.Marshal(cw)
	require.NoError(t, err)

	var decoded ContainerWaste
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, cw, decoded)
}

// --- NodeReport ---

func TestNodeReport_JSONRoundTrip(t *testing.T) {
	nr := NodeReport{
		NodeName:            "node-1",
		ReportTime:          time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Pods:                []PodWaste{{PodName: "p1", PodNamespace: "ns"}},
		TotalCPUWasteMillis: 500,
		TotalMemWasteBytes:  2048,
	}

	data, err := json.Marshal(nr)
	require.NoError(t, err)

	var decoded NodeReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, nr, decoded)
}

// --- ClusterReport ---

func TestClusterReport_JSONRoundTrip(t *testing.T) {
	cr := ClusterReport{
		ReportTime: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		NodeCount:  3,
		PodCount:   10,
		Namespaces: map[string]*NamespaceReport{
			"default": {
				Namespace:           "default",
				TotalCPUWasteMillis: 100,
				TotalMemWasteBytes:  200,
			},
		},
		TotalCPUWasteMillis: 100,
		TotalMemWasteBytes:  200,
	}

	data, err := json.Marshal(cr)
	require.NoError(t, err)

	var decoded ClusterReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, cr, decoded)
}

func TestClusterReport_NilNamespacesMap(t *testing.T) {
	cr := ClusterReport{}
	data, err := json.Marshal(cr)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	// namespaces is not omitempty, so it should be present as null
	_, hasNS := raw["namespaces"]
	assert.True(t, hasNS, "namespaces field should be present even when nil")
}

func TestClusterReport_OmitemptyRecommendations(t *testing.T) {
	cr := ClusterReport{}
	data, err := json.Marshal(cr)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	_, hasRecs := raw["recommendations"]
	assert.False(t, hasRecs, "empty recommendations should be omitted")
}

// --- NamespaceReport ---

func TestNamespaceReport_JSONRoundTrip(t *testing.T) {
	nr := NamespaceReport{
		Namespace: "production",
		Pods:      []PodWaste{{PodName: "api-1", PodNamespace: "production"}},
		Owners: []OwnerWaste{
			{Owner: OwnerReference{Kind: "Deployment", Name: "api", Namespace: "production"}, PodCount: 1},
		},
		TotalCPUWasteMillis: 250,
		TotalMemWasteBytes:  1024,
	}

	data, err := json.Marshal(nr)
	require.NoError(t, err)

	var decoded NamespaceReport
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, nr, decoded)
}

func TestNamespaceReport_OmitemptyOwners(t *testing.T) {
	nr := NamespaceReport{Namespace: "test"}
	data, err := json.Marshal(nr)
	require.NoError(t, err)

	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))

	_, hasOwners := raw["owners"]
	assert.False(t, hasOwners, "empty owners should be omitted")
}

// --- OwnerWaste ---

func TestOwnerWaste_JSONRoundTrip(t *testing.T) {
	ow := OwnerWaste{
		Owner:    OwnerReference{Kind: "StatefulSet", Name: "redis", Namespace: "cache"},
		PodCount: 3,
		Containers: []ContainerWaste{
			{ContainerName: "redis", CPUWasteMillis: 50},
		},
		TotalCPUWasteMillis: 150,
		TotalMemWasteBytes:  4096,
	}

	data, err := json.Marshal(ow)
	require.NoError(t, err)

	var decoded OwnerWaste
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ow, decoded)
}

// --- OwnerReference ---

func TestOwnerReference_JSONRoundTrip(t *testing.T) {
	ref := OwnerReference{Kind: "Deployment", Name: "payment-service", Namespace: "payments"}

	data, err := json.Marshal(ref)
	require.NoError(t, err)

	var decoded OwnerReference
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ref, decoded)
}

// --- APIResponse ---

func TestAPIResponse_JSONRoundTrip(t *testing.T) {
	resp := APIResponse{
		Status:    "ok",
		Data:      map[string]string{"key": "value"},
		Timestamp: "2025-06-01T12:00:00Z",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded APIResponse
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, "ok", decoded.Status)
	assert.Equal(t, "2025-06-01T12:00:00Z", decoded.Timestamp)
}

func TestAPIResponse_OmitemptyBehaviour(t *testing.T) {
	tests := []struct {
		name     string
		resp     APIResponse
		wantData bool
		wantErr  bool
	}{
		{
			name:     "ok response omits error",
			resp:     APIResponse{Status: "ok", Data: "hello", Timestamp: "2025-01-01T00:00:00Z"},
			wantData: true,
			wantErr:  false,
		},
		{
			name:     "error response omits data",
			resp:     APIResponse{Status: "error", Error: "not found", Timestamp: "2025-01-01T00:00:00Z"},
			wantData: false,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.resp)
			require.NoError(t, err)

			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal(data, &raw))

			_, hasData := raw["data"]
			_, hasErr := raw["error"]
			assert.Equal(t, tt.wantData, hasData, "data field presence")
			assert.Equal(t, tt.wantErr, hasErr, "error field presence")
		})
	}
}

func TestNewOKResponse(t *testing.T) {
	resp := NewOKResponse(map[string]int{"count": 42})

	assert.Equal(t, "ok", resp.Status)
	assert.NotNil(t, resp.Data)
	assert.Empty(t, resp.Error)

	// Validate timestamp is RFC3339
	_, err := time.Parse(time.RFC3339, resp.Timestamp)
	assert.NoError(t, err, "timestamp should be valid RFC3339")
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("something broke")

	assert.Equal(t, "error", resp.Status)
	assert.Nil(t, resp.Data)
	assert.Equal(t, "something broke", resp.Error)

	_, err := time.Parse(time.RFC3339, resp.Timestamp)
	assert.NoError(t, err, "timestamp should be valid RFC3339")
}

func TestAPIResponse_TimestampIsRFC3339UTC(t *testing.T) {
	resp := NewOKResponse(nil)
	parsed, err := time.Parse(time.RFC3339, resp.Timestamp)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, parsed.Location())
}

// --- ContainerKey ---

func TestContainerKey(t *testing.T) {
	tests := []struct {
		ns, pod, container string
		want               string
	}{
		{"default", "web-abc", "nginx", "default/web-abc/nginx"},
		{"kube-system", "coredns-xyz", "coredns", "kube-system/coredns-xyz/coredns"},
		{"", "", "", "//"},
	}

	for _, tt := range tests {
		got := ContainerKey(tt.ns, tt.pod, tt.container)
		assert.Equal(t, tt.want, got)
	}
}

// --- WorkloadPattern constants ---

func TestWorkloadPatternStringValues(t *testing.T) {
	assert.Equal(t, WorkloadPattern("steady"), PatternSteady)
	assert.Equal(t, WorkloadPattern("burstable"), PatternBurstable)
	assert.Equal(t, WorkloadPattern("batch"), PatternBatch)
	assert.Equal(t, WorkloadPattern("idle"), PatternIdle)
	assert.Equal(t, WorkloadPattern("anomalous"), PatternAnomalous)
}

// --- RiskLevel constants ---

func TestRiskLevelStringValues(t *testing.T) {
	assert.Equal(t, RiskLevel("LOW"), RiskLow)
	assert.Equal(t, RiskLevel("MEDIUM"), RiskMedium)
	assert.Equal(t, RiskLevel("HIGH"), RiskHigh)
}

// --- Edge cases ---

func TestMaxInt64Values(t *testing.T) {
	cm := ContainerMetrics{
		CPURequestMillis:  math.MaxInt64,
		MemoryLimitBytes:  math.MaxInt64,
		CPUUsageNanoCores: math.MaxUint64,
	}

	data, err := json.Marshal(cm)
	require.NoError(t, err)

	var decoded ContainerMetrics
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, int64(math.MaxInt64), decoded.CPURequestMillis)
	assert.Equal(t, int64(math.MaxInt64), decoded.MemoryLimitBytes)
	assert.Equal(t, uint64(math.MaxUint64), decoded.CPUUsageNanoCores)
}

func TestZeroValueStructs(t *testing.T) {
	// All zero-value structs should marshal/unmarshal cleanly
	structs := []interface{}{
		ContainerMetrics{},
		MetricAggregate{},
		AggregatedMetrics{},
		Recommendation{},
		PodWaste{},
		ContainerWaste{},
		NodeReport{},
		ClusterReport{},
		NamespaceReport{},
		OwnerWaste{},
		OwnerReference{},
		APIResponse{},
	}

	for _, s := range structs {
		data, err := json.Marshal(s)
		require.NoError(t, err, "marshal failed for %T", s)
		assert.NotEmpty(t, data, "empty marshal for %T", s)
	}
}
