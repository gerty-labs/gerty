package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
	"github.com/gregorytcarroll/k8s-sage/internal/rules"
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

	// msgMethodNotAllowed is the error message for unsupported HTTP methods.
	msgMethodNotAllowed = "method not allowed"
)

// API holds the HTTP handlers for sage-server.
type API struct {
	aggregator *Aggregator
	engine     *rules.Engine
	analyzer   *Analyzer
}

// NewAPI creates a new API with the given Aggregator, rules Engine, and Analyzer.
func NewAPI(aggregator *Aggregator, engine *rules.Engine, analyzer *Analyzer) *API {
	return &API{aggregator: aggregator, engine: engine, analyzer: analyzer}
}

// RegisterRoutes registers all API routes on the given ServeMux.
func (api *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", api.HandleHealthz)
	mux.HandleFunc("/readyz", api.HandleReadyz)
	mux.HandleFunc("/api/v1/ingest", api.HandleIngest)
	mux.HandleFunc("/api/v1/report", api.HandleReport)
	mux.HandleFunc("/api/v1/workloads", api.HandleWorkloads)
	mux.HandleFunc("/api/v1/workloads/", api.HandleWorkloads)
	mux.HandleFunc("/api/v1/recommendations", api.HandleRecommendations)
	mux.HandleFunc("/api/v1/analyze", api.HandleAnalyze)
}

// writeJSON writes a success response wrapped in the APIResponse envelope.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(models.NewOKResponse(data)); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// writeError writes an error response wrapped in the APIResponse envelope.
func writeError(w http.ResponseWriter, statusCode int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(models.NewErrorResponse(msg)); err != nil {
		slog.Error("failed to encode error response", "error", err)
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code for logging.
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

// LoggingMiddleware logs method, path, status code, and duration for each request.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.code,
			"duration", time.Since(start).String(),
		)
	})
}

// HandleHealthz responds with a simple health check.
func (api *API) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReadyz returns 200 if the server has received at least one agent report,
// 503 otherwise.
func (api *API) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	if api.aggregator.PodCount() == 0 && api.aggregator.NodeCount() == 0 {
		writeError(w, http.StatusServiceUnavailable, "no agent data received")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// HandleIngest receives a NodeReport from an agent via POST.
func (api *API) HandleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		return
	}

	// Enforce payload size limit.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			slog.Warn("ingest payload too large", "remoteAddr", r.RemoteAddr)
			writeError(w, http.StatusRequestEntityTooLarge, "payload too large")
			return
		}
		slog.Error("reading ingest body", "error", err)
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	var report models.NodeReport
	if err := json.Unmarshal(body, &report); err != nil {
		slog.Warn("malformed ingest JSON", "error", err, "remoteAddr", r.RemoteAddr)
		writeError(w, http.StatusBadRequest, fmt.Sprintf("malformed JSON: %s", err.Error()))
		return
	}

	if err := validateNodeReport(&report); err != nil {
		slog.Warn("invalid node report", "error", err, "node", report.NodeName)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ingested, atCapacity := api.aggregator.Ingest(report)

	status := http.StatusOK
	if atCapacity {
		status = http.StatusTooManyRequests
	}
	writeJSON(w, status, map[string]interface{}{
		"ingested":   ingested,
		"atCapacity": atCapacity,
	})
}

// HandleReport returns a cluster-wide or namespace-filtered waste report.
func (api *API) HandleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		return
	}

	namespace := r.URL.Query().Get("namespace")

	if namespace != "" {
		if len(namespace) > maxNamespaceLen {
			writeError(w, http.StatusBadRequest, "namespace name too long")
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
		writeJSON(w, http.StatusOK, nsReport)
		return
	}

	report := api.aggregator.ClusterReport()
	writeJSON(w, http.StatusOK, report)
}

// HandleWorkloads lists all OwnerWaste entries or returns a single workload detail.
// GET /api/v1/workloads — list all workloads across namespaces
// GET /api/v1/workloads/{ns}/{kind}/{name} — single workload detail
func (api *API) HandleWorkloads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		return
	}

	// Check for path parameters after /api/v1/workloads/
	suffix := strings.TrimPrefix(r.URL.Path, "/api/v1/workloads")
	suffix = strings.TrimPrefix(suffix, "/")

	if suffix == "" {
		// List mode: return all OwnerWaste entries across namespaces.
		report := api.aggregator.ClusterReport()
		var all []models.OwnerWaste
		for _, nsReport := range report.Namespaces {
			all = append(all, nsReport.Owners...)
		}
		if all == nil {
			all = []models.OwnerWaste{}
		}
		writeJSON(w, http.StatusOK, all)
		return
	}

	// Detail mode: expect ns/kind/name
	parts := strings.SplitN(suffix, "/", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		writeError(w, http.StatusBadRequest, "path must be /api/v1/workloads/{namespace}/{kind}/{name}")
		return
	}

	ns, kind, name := parts[0], parts[1], parts[2]

	nsReport := api.aggregator.NamespaceReport(ns)
	if nsReport == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("namespace %q not found", ns))
		return
	}

	for _, ow := range nsReport.Owners {
		if strings.EqualFold(ow.Owner.Kind, kind) && ow.Owner.Name == name {
			writeJSON(w, http.StatusOK, ow)
			return
		}
	}

	writeError(w, http.StatusNotFound, fmt.Sprintf("workload %s/%s not found in namespace %q", kind, name, ns))
}

// HandleRecommendations returns recommendations from the rules engine.
// GET /api/v1/recommendations — all recommendations
// Supports ?risk= and ?namespace= query filters.
func (api *API) HandleRecommendations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		return
	}

	report := api.aggregator.ClusterReport()
	recs := api.engine.AnalyzeCluster(report)

	// Apply optional filters.
	riskFilter := r.URL.Query().Get("risk")
	nsFilter := r.URL.Query().Get("namespace")

	if riskFilter != "" || nsFilter != "" {
		filtered := make([]models.Recommendation, 0, len(recs))
		for _, rec := range recs {
			if riskFilter != "" && !strings.EqualFold(string(rec.Risk), riskFilter) {
				continue
			}
			if nsFilter != "" && rec.Target.Namespace != nsFilter {
				continue
			}
			filtered = append(filtered, rec)
		}
		recs = filtered
	}

	if recs == nil {
		recs = []models.Recommendation{}
	}
	writeJSON(w, http.StatusOK, recs)
}

// analyzeRequest is the expected JSON body for POST /api/v1/analyze.
type analyzeRequest struct {
	Namespace string `json:"namespace"`
}

// HandleAnalyze runs the rules engine on a specific namespace.
// POST /api/v1/analyze — accepts {"namespace":"..."} body
func (api *API) HandleAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, msgMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)
	var req analyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %s", err.Error()))
		return
	}

	if req.Namespace == "" {
		writeError(w, http.StatusBadRequest, "namespace is required")
		return
	}
	if len(req.Namespace) > maxNamespaceLen {
		writeError(w, http.StatusBadRequest, "namespace name too long")
		return
	}

	nsReport := api.aggregator.NamespaceReport(req.Namespace)
	if nsReport == nil {
		writeError(w, http.StatusNotFound, fmt.Sprintf("namespace %q not found", req.Namespace))
		return
	}

	// Build a single-namespace ClusterReport to feed the engine.
	clusterReport := models.ClusterReport{
		Namespaces: map[string]*models.NamespaceReport{
			req.Namespace: nsReport,
		},
	}

	// Use Analyzer if available (L1+L2), otherwise fall back to rules-only.
	var recs []models.Recommendation
	if api.analyzer != nil {
		recs = api.analyzeClusterWithSLM(r.Context(), clusterReport)
	} else {
		recs = api.engine.AnalyzeCluster(clusterReport)
	}

	if recs == nil {
		recs = []models.Recommendation{}
	}
	writeJSON(w, http.StatusOK, recs)
}

// analyzeClusterWithSLM runs the Analyzer (L1+L2) across all owners in a cluster report.
func (api *API) analyzeClusterWithSLM(ctx context.Context, report models.ClusterReport) []models.Recommendation {
	var recs []models.Recommendation

	for _, nsReport := range report.Namespaces {
		for _, owner := range nsReport.Owners {
			for _, cw := range owner.Containers {
				input := rules.AnalysisInput{
					Owner:             owner.Owner,
					ContainerName:     cw.ContainerName,
					CPUUsageMillis:    cw.CPUUsage,
					CPURequestMillis:  cw.CPURequestMillis,
					CPULimitMillis:    0,
					MemUsageBytes:     cw.MemoryUsage,
					MemRequestBytes:   cw.MemoryRequestBytes,
					MemLimitBytes:     0,
					DataWindowMinutes: cw.DataWindow.Minutes(),
				}

				result := api.analyzer.Analyze(ctx, input)

				if result.CPURecommendation != nil {
					recs = append(recs, *result.CPURecommendation)
				}
				if result.MemRecommendation != nil {
					recs = append(recs, *result.MemRecommendation)
				}
			}
		}
	}

	return recs
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
