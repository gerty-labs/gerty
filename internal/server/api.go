package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

const (
	// maxIngestBodyBytes is the maximum allowed request body for the ingest endpoint.
	// Agent reports for 300 pods at ~500 bytes each ≈ 150KB; 1MB gives ample headroom.
	maxIngestBodyBytes = 1 * 1024 * 1024 // 1 MB

	// maxNodeNameLen is the maximum length of a node name accepted in a report.
	maxNodeNameLen = 253 // K8s DNS subdomain max

	// maxPodNameLen is the maximum length of a pod name.
	maxPodNameLen = 253

	// maxNamespaceLen is the maximum length of a namespace name.
	maxNamespaceLen = 63 // K8s namespace label max

	// maxContainerNameLen is the maximum length of a container name.
	maxContainerNameLen = 63

	// maxPodsPerReport is the maximum number of pods accepted in a single ingest.
	maxPodsPerReport = 500

	// maxContainersPerPod is the maximum number of containers in a pod report.
	maxContainersPerPod = 50
)

// API holds the HTTP handlers for sage-server.
type API struct {
	aggregator *Aggregator
}

// NewAPI creates a new API with the given Aggregator.
func NewAPI(aggregator *Aggregator) *API {
	return &API{aggregator: aggregator}
}

// RegisterRoutes registers all API routes on the given ServeMux.
func (api *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", api.HandleHealthz)
	mux.HandleFunc("/api/v1/ingest", api.HandleIngest)
	mux.HandleFunc("/api/v1/report", api.HandleReport)
}

// HandleHealthz responds with a simple health check.
func (api *API) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleIngest receives a NodeReport from an agent via POST.
func (api *API) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Enforce payload size limit.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			slog.Warn("ingest payload too large", "remoteAddr", r.RemoteAddr)
			http.Error(w, `{"error":"payload too large"}`, http.StatusRequestEntityTooLarge)
			return
		}
		slog.Error("reading ingest body", "error", err)
		http.Error(w, `{"error":"failed to read request body"}`, http.StatusBadRequest)
		return
	}

	var report models.NodeReport
	if err := json.Unmarshal(body, &report); err != nil {
		slog.Warn("malformed ingest JSON", "error", err, "remoteAddr", r.RemoteAddr)
		http.Error(w, fmt.Sprintf(`{"error":"malformed JSON: %s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if err := validateNodeReport(&report); err != nil {
		slog.Warn("invalid node report", "error", err, "node", report.NodeName)
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	ingested, atCapacity := api.aggregator.Ingest(report)

	w.Header().Set("Content-Type", "application/json")
	status := http.StatusOK
	if atCapacity {
		status = http.StatusTooManyRequests
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ingested":   ingested,
		"atCapacity": atCapacity,
	})
}

// HandleReport returns a cluster-wide or namespace-filtered waste report.
func (api *API) HandleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	w.Header().Set("Content-Type", "application/json")

	if namespace != "" {
		if len(namespace) > maxNamespaceLen {
			http.Error(w, `{"error":"namespace name too long"}`, http.StatusBadRequest)
			return
		}

		nsReport := api.aggregator.NamespaceReport(namespace)
		if nsReport == nil {
			// Return an empty namespace report rather than 404 — the namespace
			// may exist but have no waste data yet.
			nsReport = &models.NamespaceReport{
				Namespace: namespace,
				Pods:      []models.PodWaste{},
			}
		}
		json.NewEncoder(w).Encode(nsReport)
		return
	}

	report := api.aggregator.ClusterReport()
	json.NewEncoder(w).Encode(report)
}

// validateNodeReport checks that a NodeReport has all required fields and
// doesn't contain obviously invalid data.
func validateNodeReport(report *models.NodeReport) error {
	if report.NodeName == "" {
		return fmt.Errorf("nodeName is required")
	}
	if len(report.NodeName) > maxNodeNameLen {
		return fmt.Errorf("nodeName exceeds maximum length of %d", maxNodeNameLen)
	}
	if report.ReportTime.IsZero() {
		return fmt.Errorf("reportTime is required")
	}

	if len(report.Pods) > maxPodsPerReport {
		return fmt.Errorf("too many pods in report: %d (max %d)", len(report.Pods), maxPodsPerReport)
	}

	for i, pod := range report.Pods {
		if pod.PodName == "" {
			return fmt.Errorf("pod[%d]: podName is required", i)
		}
		if len(pod.PodName) > maxPodNameLen {
			return fmt.Errorf("pod[%d]: podName exceeds maximum length of %d", i, maxPodNameLen)
		}
		if pod.PodNamespace == "" {
			return fmt.Errorf("pod[%d]: podNamespace is required", i)
		}
		if len(pod.PodNamespace) > maxNamespaceLen {
			return fmt.Errorf("pod[%d]: podNamespace exceeds maximum length of %d", i, maxNamespaceLen)
		}
		if len(pod.Containers) > maxContainersPerPod {
			return fmt.Errorf("pod[%d]: too many containers: %d (max %d)", i, len(pod.Containers), maxContainersPerPod)
		}

		for j, c := range pod.Containers {
			if c.ContainerName == "" {
				return fmt.Errorf("pod[%d].container[%d]: containerName is required", i, j)
			}
			if len(c.ContainerName) > maxContainerNameLen {
				return fmt.Errorf("pod[%d].container[%d]: containerName exceeds maximum length of %d", i, j, maxContainerNameLen)
			}
		}
	}

	return nil
}
