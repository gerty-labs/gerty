package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeResponse decodes the APIResponse envelope from a recorder. If target
// is non-nil, the .Data field is re-marshaled into it.
func decodeResponse(t *testing.T, w *httptest.ResponseRecorder, target interface{}) models.APIResponse {
	t.Helper()
	var envelope models.APIResponse
	err := json.NewDecoder(w.Body).Decode(&envelope)
	require.NoError(t, err, "failed to decode APIResponse envelope")

	if target != nil && envelope.Data != nil {
		raw, err := json.Marshal(envelope.Data)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, target))
	}
	return envelope
}

func newTestAPI() (*Aggregator, *API) {
	agg := NewAggregator()
	engine := rules.NewEngine()
	analyzer := NewAnalyzer(engine, nil)
	api := NewAPI(agg, engine, analyzer)
	return agg, api
}

func validReport() models.NodeReport {
	return models.NodeReport{
		NodeName:   "test-node",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			{
				PodName:      "nginx-abc",
				PodNamespace: "default",
				QoSClass:     "Burstable",
				Containers: []models.ContainerWaste{
					{
						ContainerName:    "nginx",
						CPURequestMillis: 1000,
						CPUWasteMillis:   800,
						CPUWastePercent:  80,
					},
				},
				TotalCPUWasteMillis: 800,
			},
		},
		TotalCPUWasteMillis: 800,
	}
}

func postIngest(t *testing.T, api *API, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.HandleIngest(w, req)
	return w
}

func TestAPI_HandleIngest_ValidReport(t *testing.T) {
	agg, api := newTestAPI()

	body, _ := json.Marshal(validReport())
	w := postIngest(t, api, body)

	assert.Equal(t, http.StatusOK, w.Code)

	var data map[string]interface{}
	env := decodeResponse(t, w, &data)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, float64(1), data["ingested"])
	assert.Equal(t, false, data["atCapacity"])
	assert.Equal(t, 1, agg.PodCount())
}

func TestAPI_HandleIngest_MalformedJSON(t *testing.T) {
	agg, api := newTestAPI()

	w := postIngest(t, api, []byte(`{invalid json}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "malformed JSON")
	assert.Equal(t, 0, agg.PodCount())
}

func TestAPI_HandleIngest_EmptyBody(t *testing.T) {
	_, api := newTestAPI()

	w := postIngest(t, api, []byte(`{}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "nodeName is required")
}

func TestAPI_HandleIngest_MissingNodeName(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.NodeName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "nodeName is required")
}

func TestAPI_HandleIngest_MissingReportTime(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.ReportTime = time.Time{}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "reportTime is required")
}

func TestAPI_HandleIngest_MissingPodName(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.Pods[0].PodName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "podName is required")
}

func TestAPI_HandleIngest_MissingPodNamespace(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.Pods[0].PodNamespace = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "podNamespace is required")
}

func TestAPI_HandleIngest_MissingContainerName(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.Pods[0].Containers[0].ContainerName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "containerName is required")
}

func TestAPI_HandleIngest_NodeNameTooLong(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.NodeName = strings.Repeat("a", maxNodeNameLen+1)
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "exceeds maximum length")
}

func TestAPI_HandleIngest_TooManyPods(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.Pods = make([]models.PodWaste, maxPodsPerReport+1)
	for i := range report.Pods {
		report.Pods[i] = makePod("ns", "pod-"+strings.Repeat("x", 5), 0, 0)
	}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "too many pods")
}

func TestAPI_HandleIngest_TooManyContainers(t *testing.T) {
	_, api := newTestAPI()

	report := validReport()
	report.Pods[0].Containers = make([]models.ContainerWaste, maxContainersPerPod+1)
	for i := range report.Pods[0].Containers {
		report.Pods[0].Containers[i] = models.ContainerWaste{ContainerName: "c"}
	}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "too many containers")
}

func TestAPI_HandleIngest_PayloadTooLarge(t *testing.T) {
	_, api := newTestAPI()

	// Create a body larger than maxIngestBodyBytes.
	bigBody := make([]byte, maxIngestBodyBytes+100)
	for i := range bigBody {
		bigBody[i] = 'a'
	}

	w := postIngest(t, api, bigBody)
	// The MaxBytesReader triggers a "request body too large" error which is
	// caught and returned as 413 Request Entity Too Large.
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "payload too large")
}

func TestAPI_HandleIngest_MethodNotAllowed(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest", nil)
	w := httptest.NewRecorder()
	api.HandleIngest(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "method not allowed")
}

func TestAPI_HandleReport_EmptyCluster(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.ClusterReport
	env := decodeResponse(t, w, &report)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, 0, report.PodCount)
	assert.Equal(t, 0, report.NodeCount)
}

func TestAPI_HandleReport_ClusterWide(t *testing.T) {
	_, api := newTestAPI()

	body, _ := json.Marshal(validReport())
	postIngest(t, api, body)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.ClusterReport
	decodeResponse(t, w, &report)
	assert.Equal(t, 1, report.PodCount)
	assert.Equal(t, 1, report.NodeCount)
	assert.Contains(t, report.Namespaces, "default")
}

func TestAPI_HandleReport_NamespaceFilter(t *testing.T) {
	_, api := newTestAPI()

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makePod("default", "pod-a", 100, 100),
			makePod("production", "pod-b", 200, 200),
		},
	}
	body, _ := json.Marshal(report)
	postIngest(t, api, body)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report?namespace=production", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var nsReport models.NamespaceReport
	decodeResponse(t, w, &nsReport)
	assert.Equal(t, "production", nsReport.Namespace)
	assert.Len(t, nsReport.Pods, 1)
	assert.Equal(t, "pod-b", nsReport.Pods[0].PodName)
}

func TestAPI_HandleReport_NamespaceNotFound(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report?namespace=nonexistent", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var nsReport models.NamespaceReport
	decodeResponse(t, w, &nsReport)
	assert.Equal(t, "nonexistent", nsReport.Namespace)
	assert.Empty(t, nsReport.Pods)
}

func TestAPI_HandleReport_NamespaceTooLong(t *testing.T) {
	_, api := newTestAPI()

	longNS := strings.Repeat("x", maxNamespaceLen+1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/report?namespace="+longNS, nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_HandleReport_MethodNotAllowed(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestAPI_HandleHealthz(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	api.HandleHealthz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "ok", env.Status)
}

func TestAPI_HandleIngest_ValidEmptyPodList(t *testing.T) {
	agg, api := newTestAPI()

	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods:       []models.PodWaste{},
	}
	body, _ := json.Marshal(report)
	w := postIngest(t, api, body)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 0, agg.PodCount())
}

func TestAPI_RegisterRoutes(t *testing.T) {
	_, api := newTestAPI()
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)

	// Test that routes are registered by making requests.
	tests := []struct {
		method string
		path   string
		want   int
	}{
		{http.MethodGet, "/healthz", http.StatusOK},
		{http.MethodGet, "/api/v1/report", http.StatusOK},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, tt.want, w.Code, "route %s %s", tt.method, tt.path)
	}
}

func TestAPI_HandleReadyz_NoData(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	api.HandleReadyz(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "no agent data")
}

func TestAPI_HandleReadyz_WithData(t *testing.T) {
	_, api := newTestAPI()

	// Ingest a report so the server has data.
	body, _ := json.Marshal(validReport())
	postIngest(t, api, body)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	api.HandleReadyz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "ok", env.Status)
}

// --- Helpers for high-waste test data ---

// makeHighWastePod creates a pod with high waste that will trigger the rules engine
// to produce recommendations (request: 2000m, P95 usage: 200m = 90% waste).
func makeHighWastePod(ns, name string, owner models.OwnerReference) models.PodWaste {
	return models.PodWaste{
		PodName:      name,
		PodNamespace: ns,
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
	}
}

func ingestHighWasteData(t *testing.T, api *API) {
	t.Helper()
	owner := models.OwnerReference{Kind: "Deployment", Name: "api-server", Namespace: "production"}
	report := models.NodeReport{
		NodeName:   "node-1",
		ReportTime: time.Now(),
		Pods: []models.PodWaste{
			makeHighWastePod("production", "api-server-abc", owner),
			makeHighWastePod("production", "api-server-def", owner),
		},
	}
	body, _ := json.Marshal(report)
	postIngest(t, api, body)
}

// --- Workloads endpoint tests ---

func TestAPI_HandleWorkloads_EmptyList(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var data []models.OwnerWaste
	env := decodeResponse(t, w, &data)
	assert.Equal(t, "ok", env.Status)
	assert.Empty(t, data)
}

func TestAPI_HandleWorkloads_ListWithData(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var data []models.OwnerWaste
	decodeResponse(t, w, &data)
	assert.Len(t, data, 1) // Two pods, same owner = 1 OwnerWaste
	assert.Equal(t, "api-server", data[0].Owner.Name)
	assert.Equal(t, 2, data[0].PodCount)
}

func TestAPI_HandleWorkloads_DetailFound(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/production/Deployment/api-server", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var data models.OwnerWaste
	env := decodeResponse(t, w, &data)
	assert.Equal(t, "ok", env.Status)
	assert.Equal(t, "api-server", data.Owner.Name)
	assert.Equal(t, "Deployment", data.Owner.Kind)
}

func TestAPI_HandleWorkloads_DetailCaseInsensitiveKind(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	// Lowercase "deployment" should match "Deployment".
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/production/deployment/api-server", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var data models.OwnerWaste
	decodeResponse(t, w, &data)
	assert.Equal(t, "api-server", data.Owner.Name)
}

func TestAPI_HandleWorkloads_DetailNotFound(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/production/Deployment/nonexistent", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Equal(t, "error", env.Status)
	assert.Contains(t, env.Error, "not found")
}

func TestAPI_HandleWorkloads_DetailBadPath(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/only-one-part", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "path must be")
}

func TestAPI_HandleWorkloads_NamespaceNotFound(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workloads/nonexistent/Deployment/api-server", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "namespace")
}

func TestAPI_HandleWorkloads_MethodNotAllowed(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/workloads", nil)
	w := httptest.NewRecorder()
	api.HandleWorkloads(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- Recommendations endpoint tests ---

func TestAPI_HandleRecommendations_Empty(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	env := decodeResponse(t, w, &recs)
	assert.Equal(t, "ok", env.Status)
	assert.Empty(t, recs)
}

func TestAPI_HandleRecommendations_WithData(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	decodeResponse(t, w, &recs)
	assert.NotEmpty(t, recs, "high-waste data should produce recommendations")

	// All recommendations should reference our target.
	for _, rec := range recs {
		assert.Equal(t, "production", rec.Target.Namespace)
	}
}

func TestAPI_HandleRecommendations_FilterByRisk(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?risk=LOW", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	decodeResponse(t, w, &recs)
	for _, rec := range recs {
		assert.Equal(t, models.RiskLow, rec.Risk)
	}
}

func TestAPI_HandleRecommendations_FilterByNamespace(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	// Add data in another namespace too.
	otherOwner := models.OwnerReference{Kind: "Deployment", Name: "web", Namespace: "staging"}
	report := models.NodeReport{
		NodeName:   "node-2",
		ReportTime: time.Now(),
		Pods:       []models.PodWaste{makeHighWastePod("staging", "web-abc", otherOwner)},
	}
	body, _ := json.Marshal(report)
	postIngest(t, api, body)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?namespace=staging", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	decodeResponse(t, w, &recs)
	for _, rec := range recs {
		assert.Equal(t, "staging", rec.Target.Namespace)
	}
}

func TestAPI_HandleRecommendations_CombinedFilters(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recommendations?namespace=production&risk=LOW", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	decodeResponse(t, w, &recs)
	for _, rec := range recs {
		assert.Equal(t, "production", rec.Target.Namespace)
		assert.Equal(t, models.RiskLow, rec.Risk)
	}
}

func TestAPI_HandleRecommendations_MethodNotAllowed(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", nil)
	w := httptest.NewRecorder()
	api.HandleRecommendations(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- Analyze endpoint tests ---

func TestAPI_HandleAnalyze_ValidNamespace(t *testing.T) {
	_, api := newTestAPI()
	ingestHighWasteData(t, api)

	body, _ := json.Marshal(map[string]string{"namespace": "production"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var recs []models.Recommendation
	env := decodeResponse(t, w, &recs)
	assert.Equal(t, "ok", env.Status)
	assert.NotEmpty(t, recs)
	for _, rec := range recs {
		assert.Equal(t, "production", rec.Target.Namespace)
	}
}

func TestAPI_HandleAnalyze_NamespaceNotFound(t *testing.T) {
	_, api := newTestAPI()

	body, _ := json.Marshal(map[string]string{"namespace": "nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "not found")
}

func TestAPI_HandleAnalyze_MissingNamespace(t *testing.T) {
	_, api := newTestAPI()

	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "namespace is required")
}

func TestAPI_HandleAnalyze_NamespaceTooLong(t *testing.T) {
	_, api := newTestAPI()

	longNS := strings.Repeat("x", maxNamespaceLen+1)
	body, _ := json.Marshal(map[string]string{"namespace": longNS})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewReader(body))
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "namespace name too long")
}

func TestAPI_HandleAnalyze_BadJSON(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/analyze", bytes.NewReader([]byte(`{bad`)))
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := decodeResponse(t, w, nil)
	assert.Contains(t, env.Error, "invalid JSON")
}

func TestAPI_HandleAnalyze_MethodNotAllowed(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analyze", nil)
	w := httptest.NewRecorder()
	api.HandleAnalyze(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

// --- Envelope format tests ---

func TestAPI_Envelope_SuccessStructure(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	api.HandleHealthz(w, req)

	var raw map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&raw)
	require.NoError(t, err)

	assert.Equal(t, "ok", raw["status"])
	assert.NotNil(t, raw["data"])
	assert.NotEmpty(t, raw["timestamp"])
	assert.Nil(t, raw["error"])
}

func TestAPI_Envelope_ErrorStructure(t *testing.T) {
	_, api := newTestAPI()

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	api.HandleReadyz(w, req)

	var raw map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&raw)
	require.NoError(t, err)

	assert.Equal(t, "error", raw["status"])
	assert.Nil(t, raw["data"])
	assert.NotEmpty(t, raw["error"])
	assert.NotEmpty(t, raw["timestamp"])
}
