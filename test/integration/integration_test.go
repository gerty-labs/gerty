//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/gregorytcarroll/k8s-sage/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Helpers ---

func setupTestServer(t *testing.T) (*httptest.Server, *server.Aggregator) {
	t.Helper()
	agg := server.NewAggregator()
	engine := rules.NewEngine()
	api := server.NewAPI(agg, engine)

	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	return ts, agg
}

func makeHighWastePod(ns, name, container string) models.PodWaste {
	return models.PodWaste{
		PodName:      name,
		PodNamespace: ns,
		QoSClass:     "Burstable",
		OwnerRef: models.OwnerReference{
			Kind:      "Deployment",
			Name:      name,
			Namespace: ns,
		},
		Containers: []models.ContainerWaste{
			{
				ContainerName:      container,
				CPURequestMillis:   1000,
				CPUUsage:           models.MetricAggregate{P50: 50, P95: 100, P99: 150, Max: 200},
				CPUWasteMillis:     900,
				CPUWastePercent:    90,
				MemoryRequestBytes: 1_000_000_000,
				MemoryUsage:        models.MetricAggregate{P50: 50_000_000, P95: 100_000_000, P99: 150_000_000, Max: 200_000_000},
				MemWasteBytes:      900_000_000,
				MemWastePercent:    90,
				DataWindow:         7 * 24 * time.Hour,
			},
		},
		TotalCPUWasteMillis: 900,
		TotalMemWasteBytes:  900_000_000,
	}
}

func postIngest(t *testing.T, serverURL string, report models.NodeReport) {
	t.Helper()
	body, err := json.Marshal(report)
	require.NoError(t, err)

	resp, err := http.Post(serverURL+"/api/v1/ingest", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func decodeAPIResponse(t *testing.T, resp *http.Response, target interface{}) models.APIResponse {
	t.Helper()
	var envelope models.APIResponse
	err := json.NewDecoder(resp.Body).Decode(&envelope)
	require.NoError(t, err)

	if target != nil && envelope.Data != nil {
		raw, err := json.Marshal(envelope.Data)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, target))
	}
	return envelope
}

// --- Tests ---

func TestIngestAndReport(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("default", "pod-a", "main"),
			makeHighWastePod("default", "pod-b", "main"),
			makeHighWastePod("kube-system", "pod-c", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	resp, err := http.Get(ts.URL + "/api/v1/report")
	require.NoError(t, err)
	defer resp.Body.Close()

	var clusterReport models.ClusterReport
	env := decodeAPIResponse(t, resp, &clusterReport)

	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, 1, clusterReport.NodeCount)
	assert.Equal(t, 3, clusterReport.PodCount)
}

func TestMultiAgentConcurrent(t *testing.T) {
	ts, agg := setupTestServer(t)

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(nodeIdx int) {
			defer wg.Done()
			nodeName := "node-" + string(rune('a'+nodeIdx))
			report := models.NodeReport{
				NodeName:   nodeName,
				ReportTime: time.Now(),
				Pods: []models.PodWaste{
					makeHighWastePod("default", nodeName+"-pod", "main"),
				},
			}
			postIngest(t, ts.URL, report)
		}(i)
	}
	wg.Wait()

	resp, err := http.Get(ts.URL + "/api/v1/report")
	require.NoError(t, err)
	defer resp.Body.Close()

	var clusterReport models.ClusterReport
	decodeAPIResponse(t, resp, &clusterReport)

	assert.Equal(t, 3, clusterReport.NodeCount, "all 3 agent nodes should be present")
	assert.Equal(t, 3, clusterReport.PodCount, "all 3 pods should be present")
	assert.Equal(t, 3, agg.NodeCount())
}

func TestStalePodEviction(t *testing.T) {
	ts, agg := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("default", "fresh-pod", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	// Call PruneStalePods immediately — fresh pod should NOT be pruned.
	pruned := agg.PruneStalePods()
	assert.Equal(t, 0, pruned, "just-ingested pod should not be pruned")
	assert.Equal(t, 1, agg.PodCount(), "pod should still be present after prune")
}

func TestReportEndpointWithNamespace(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("ns1", "pod-a", "main"),
			makeHighWastePod("ns1", "pod-b", "main"),
			makeHighWastePod("ns2", "pod-c", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	resp, err := http.Get(ts.URL + "/api/v1/report?namespace=ns1")
	require.NoError(t, err)
	defer resp.Body.Close()

	var nsReport models.NamespaceReport
	decodeAPIResponse(t, resp, &nsReport)

	assert.Equal(t, "ns1", nsReport.Namespace)
	assert.Len(t, nsReport.Pods, 2, "only ns1 pods should be returned")
}

func TestRecommendationsEndpoint(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("production", "api-server-abc", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	resp, err := http.Get(ts.URL + "/api/v1/recommendations")
	require.NoError(t, err)
	defer resp.Body.Close()

	var recs []models.Recommendation
	env := decodeAPIResponse(t, resp, &recs)

	assert.Equal(t, "ok", env.Status)
	assert.NotEmpty(t, recs, "high-waste pods should produce recommendations")

	for _, rec := range recs {
		assert.Greater(t, rec.RecommendedReq, int64(0), "recommendedRequest should be positive")
		assert.GreaterOrEqual(t, rec.EstSavings, int64(0), "estimatedSavings should be non-negative")
	}
}

func TestRecommendationsNamespaceFilter(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("ns1", "deploy-a", "main"),
			makeHighWastePod("ns2", "deploy-b", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	resp, err := http.Get(ts.URL + "/api/v1/recommendations?namespace=ns1")
	require.NoError(t, err)
	defer resp.Body.Close()

	var recs []models.Recommendation
	decodeAPIResponse(t, resp, &recs)

	for _, rec := range recs {
		assert.Equal(t, "ns1", rec.Target.Namespace, "only ns1 recs should be returned")
	}
}

func TestWorkloadsListAndDetail(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("production", "web-server", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	// Test list endpoint.
	resp, err := http.Get(ts.URL + "/api/v1/workloads")
	require.NoError(t, err)
	defer resp.Body.Close()

	var workloads []models.OwnerWaste
	decodeAPIResponse(t, resp, &workloads)
	assert.NotEmpty(t, workloads, "workloads list should not be empty")

	// Test detail endpoint.
	resp2, err := http.Get(ts.URL + "/api/v1/workloads/production/Deployment/web-server")
	require.NoError(t, err)
	defer resp2.Body.Close()

	var detail models.OwnerWaste
	env := decodeAPIResponse(t, resp2, &detail)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "web-server", detail.Owner.Name)
	assert.Equal(t, "Deployment", detail.Owner.Kind)
	assert.Equal(t, "production", detail.Owner.Namespace)
}

func TestCLIReportCommand(t *testing.T) {
	ts, _ := setupTestServer(t)

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("default", "test-pod", "main"),
		},
	}
	postIngest(t, ts.URL, report)

	cmd := exec.Command("go", "run", "./cmd/cli", "report", "--server", ts.URL)
	cmd.Dir = findProjectRoot(t)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "CLI report command failed: %s", string(output))
	assert.NotEmpty(t, output, "CLI report output should not be empty")
}

// findProjectRoot walks up from the test directory to find the module root.
func findProjectRoot(t *testing.T) string {
	t.Helper()
	// The test file is at test/integration/integration_test.go,
	// so the project root is two directories up.
	cmd := exec.Command("go", "list", "-m", "-f", "{{.Dir}}")
	output, err := cmd.Output()
	require.NoError(t, err, "failed to find project root")
	return string(bytes.TrimSpace(output))
}
