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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	agg := NewAggregator()
	api := NewAPI(agg)

	body, _ := json.Marshal(validReport())
	w := postIngest(t, api, body)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	assert.Equal(t, float64(1), resp["ingested"])
	assert.Equal(t, false, resp["atCapacity"])
	assert.Equal(t, 1, agg.PodCount())
}

func TestAPI_HandleIngest_MalformedJSON(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	w := postIngest(t, api, []byte(`{invalid json}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "malformed JSON")
	assert.Equal(t, 0, agg.PodCount())
}

func TestAPI_HandleIngest_EmptyBody(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	w := postIngest(t, api, []byte(`{}`))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "nodeName is required")
}

func TestAPI_HandleIngest_MissingNodeName(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.NodeName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "nodeName is required")
}

func TestAPI_HandleIngest_MissingReportTime(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.ReportTime = time.Time{}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "reportTime is required")
}

func TestAPI_HandleIngest_MissingPodName(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.Pods[0].PodName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "podName is required")
}

func TestAPI_HandleIngest_MissingPodNamespace(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.Pods[0].PodNamespace = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "podNamespace is required")
}

func TestAPI_HandleIngest_MissingContainerName(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.Pods[0].Containers[0].ContainerName = ""
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "containerName is required")
}

func TestAPI_HandleIngest_NodeNameTooLong(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.NodeName = strings.Repeat("a", maxNodeNameLen+1)
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "exceeds maximum length")
}

func TestAPI_HandleIngest_TooManyPods(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.Pods = make([]models.PodWaste, maxPodsPerReport+1)
	for i := range report.Pods {
		report.Pods[i] = makePod("ns", "pod-"+strings.Repeat("x", 5), 0, 0)
	}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "too many pods")
}

func TestAPI_HandleIngest_TooManyContainers(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	report := validReport()
	report.Pods[0].Containers = make([]models.ContainerWaste, maxContainersPerPod+1)
	for i := range report.Pods[0].Containers {
		report.Pods[0].Containers[i] = models.ContainerWaste{ContainerName: "c"}
	}
	body, _ := json.Marshal(report)

	w := postIngest(t, api, body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "too many containers")
}

func TestAPI_HandleIngest_PayloadTooLarge(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	// Create a body larger than maxIngestBodyBytes.
	bigBody := make([]byte, maxIngestBodyBytes+100)
	for i := range bigBody {
		bigBody[i] = 'a'
	}

	w := postIngest(t, api, bigBody)
	// Should be 413 or 400 depending on where it's caught.
	assert.True(t, w.Code == http.StatusRequestEntityTooLarge || w.Code == http.StatusBadRequest)
}

func TestAPI_HandleIngest_MethodNotAllowed(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ingest", nil)
	w := httptest.NewRecorder()
	api.HandleIngest(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestAPI_HandleReport_EmptyCluster(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.ClusterReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)
	assert.Equal(t, 0, report.PodCount)
	assert.Equal(t, 0, report.NodeCount)
}

func TestAPI_HandleReport_ClusterWide(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	body, _ := json.Marshal(validReport())
	postIngest(t, api, body)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var report models.ClusterReport
	err := json.NewDecoder(w.Body).Decode(&report)
	require.NoError(t, err)
	assert.Equal(t, 1, report.PodCount)
	assert.Equal(t, 1, report.NodeCount)
	assert.Contains(t, report.Namespaces, "default")
}

func TestAPI_HandleReport_NamespaceFilter(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

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
	err := json.NewDecoder(w.Body).Decode(&nsReport)
	require.NoError(t, err)
	assert.Equal(t, "production", nsReport.Namespace)
	assert.Len(t, nsReport.Pods, 1)
	assert.Equal(t, "pod-b", nsReport.Pods[0].PodName)
}

func TestAPI_HandleReport_NamespaceNotFound(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/report?namespace=nonexistent", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var nsReport models.NamespaceReport
	err := json.NewDecoder(w.Body).Decode(&nsReport)
	require.NoError(t, err)
	assert.Equal(t, "nonexistent", nsReport.Namespace)
	assert.Empty(t, nsReport.Pods)
}

func TestAPI_HandleReport_NamespaceTooLong(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	longNS := strings.Repeat("x", maxNamespaceLen+1)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/report?namespace="+longNS, nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAPI_HandleReport_MethodNotAllowed(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", nil)
	w := httptest.NewRecorder()
	api.HandleReport(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestAPI_HandleHealthz(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	api.HandleHealthz(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

func TestAPI_HandleIngest_ValidEmptyPodList(t *testing.T) {
	agg := NewAggregator()
	api := NewAPI(agg)

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
	agg := NewAggregator()
	api := NewAPI(agg)
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
