package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestServer creates a real API server with aggregator + engine and returns
// the httptest.Server and aggregator for seeding data.
func newTestServer(t *testing.T) (*httptest.Server, *server.Aggregator) {
	t.Helper()
	agg := server.NewAggregator()
	engine := rules.NewEngine()
	analyzer := server.NewAnalyzer(engine, nil)
	api := server.NewAPI(agg, engine, analyzer)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, agg
}

func seedHighWasteData(t *testing.T, agg *server.Aggregator) {
	t.Helper()
	owner := models.OwnerReference{Kind: "Deployment", Name: "api-server", Namespace: "production"}
	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			{
				PodName:      "api-server-abc",
				PodNamespace: "production",
				QoSClass:     "Burstable",
				OwnerRef:     owner,
				Containers: []models.ContainerWaste{
					{
						ContainerName:      "main",
						CPURequestMillis:   2000,
						CPUUsage:           models.MetricAggregate{P50: 100, P95: 200, P99: 300, Max: 400},
						CPUWasteMillis:     1800,
						CPUWastePercent:    90,
						MemoryRequestBytes: 2_000_000_000,
						MemoryUsage:        models.MetricAggregate{P50: 100_000_000, P95: 200_000_000, P99: 300_000_000, Max: 400_000_000},
						MemWasteBytes:      1_800_000_000,
						MemWastePercent:    90,
						DataWindow:         7 * 24 * time.Hour,
					},
				},
				TotalCPUWasteMillis: 1800,
				TotalMemWasteBytes:  1_800_000_000,
			},
		},
	}
	agg.Ingest(report)
}

func TestClient_GetClusterReport_Empty(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClient(ts.URL)

	report, err := client.GetClusterReport()
	require.NoError(t, err)
	assert.Equal(t, 0, report.PodCount)
	assert.Equal(t, 0, report.NodeCount)
}

func TestClient_GetClusterReport_WithData(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	report, err := client.GetClusterReport()
	require.NoError(t, err)
	assert.Equal(t, 1, report.PodCount)
	assert.Equal(t, 1, report.NodeCount)
	assert.Contains(t, report.Namespaces, "production")
}

func TestClient_GetNamespaceReport(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	report, err := client.GetNamespaceReport("production")
	require.NoError(t, err)
	assert.Equal(t, "production", report.Namespace)
	assert.Len(t, report.Pods, 1)
}

func TestClient_GetRecommendations(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	recs, err := client.GetRecommendations("", "")
	require.NoError(t, err)
	assert.NotEmpty(t, recs)
}

func TestClient_GetWorkloads(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	workloads, err := client.GetWorkloads()
	require.NoError(t, err)
	assert.Len(t, workloads, 1)
	assert.Equal(t, "api-server", workloads[0].Owner.Name)
}

func TestClient_GetWorkload_Found(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	ow, err := client.GetWorkload("production", "Deployment", "api-server")
	require.NoError(t, err)
	assert.Equal(t, "api-server", ow.Owner.Name)
	assert.Equal(t, "Deployment", ow.Owner.Kind)
}

func TestClient_GetWorkload_NotFound(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClient(ts.URL)

	_, err := client.GetWorkload("nope", "Deployment", "nope")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "server error")
}

func TestClient_ServerUnreachable(t *testing.T) {
	client := NewClient("http://127.0.0.1:1") // nothing listening

	_, err := client.GetClusterReport()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "request failed")
}

// Verify the envelope is properly unwrapped -- the client should return
// domain objects, not raw APIResponse wrappers.
func TestClient_EnvelopeUnwrap(t *testing.T) {
	ts, agg := newTestServer(t)
	seedHighWasteData(t, agg)
	client := NewClient(ts.URL)

	report, err := client.GetClusterReport()
	require.NoError(t, err)

	// Ensure it's a real ClusterReport, not wrapped in envelope.
	raw, err := json.Marshal(report)
	require.NoError(t, err)
	// Should NOT have "status" at top level (that's the envelope).
	var m map[string]interface{}
	err = json.Unmarshal(raw, &m)
	require.NoError(t, err)

	// A ClusterReport struct does not have a "status":"ok" field.
	// If it does, the client is returning the raw envelope instead of unwrapping.
	statusVal, hasStatus := m["status"]
	if hasStatus {
		// ClusterReport might have a field that serializes as "status" but it
		// should never be the string "ok" (which is the APIResponse envelope marker).
		assert.NotEqual(t, "ok", statusVal, "client should unwrap envelope, not return raw APIResponse")
	}
	// Verify it has ClusterReport-specific fields instead.
	assert.Contains(t, m, "nodeCount", "unwrapped object should have ClusterReport fields")
	assert.Contains(t, m, "podCount", "unwrapped object should have ClusterReport fields")
}
