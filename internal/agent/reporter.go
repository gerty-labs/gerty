package agent

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gregorytcarroll/k8s-sage/internal/models"
)

// Reporter serves the /report endpoint with waste-per-pod JSON analysis.
type Reporter struct {
	nodeName string
	store    *Store
}

// NewReporter creates a Reporter.
func NewReporter(nodeName string, store *Store) *Reporter {
	return &Reporter{
		nodeName: nodeName,
		store:    store,
	}
}

// HandleReport is the HTTP handler for GET /report.
// It returns a JSON NodeReport with waste analysis for every pod on this node.
func (r *Reporter) HandleReport(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	report := r.buildReport()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		slog.Error("failed to encode report", "error", err)
	}
}

// BuildReport constructs the full waste report from the store's data.
// It is used by the Pusher to obtain the current NodeReport for pushing to the server.
func (r *Reporter) BuildReport() models.NodeReport {
	return r.buildReport()
}

// buildReport constructs the full waste report from the store's data.
func (r *Reporter) buildReport() models.NodeReport {
	keys := r.store.ContainerKeys()

	// Group containers by pod (namespace/pod).
	type podKey struct {
		namespace string
		pod       string
	}
	podContainers := make(map[podKey][]string)
	for _, key := range keys {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) != 3 {
			continue
		}
		pk := podKey{namespace: parts[0], pod: parts[1]}
		podContainers[pk] = append(podContainers[pk], key)
	}

	var pods []models.PodWaste
	var totalCPUWaste, totalMemWaste float64

	for pk, containerKeys := range podContainers {
		podWaste := models.PodWaste{
			PodName:      pk.pod,
			PodNamespace: pk.namespace,
		}

		for _, key := range containerKeys {
			snap := r.store.GetContainerSnapshot(key)
			if !snap.OK {
				continue
			}

			if podWaste.QoSClass == "" {
				podWaste.QoSClass = snap.Meta.QoSClass
			}

			// Set owner reference from container metadata (same for all containers in a pod).
			if podWaste.OwnerRef.Kind == "" && snap.Meta.OwnerKind != "" {
				podWaste.OwnerRef = models.OwnerReference{
					Kind:      snap.Meta.OwnerKind,
					Name:      snap.Meta.OwnerName,
					Namespace: pk.namespace,
				}
			}

			cw := computeContainerWaste(snap.Summary, snap.Meta, snap.DataWindow)
			podWaste.Containers = append(podWaste.Containers, cw)
			podWaste.TotalCPUWasteMillis += cw.CPUWasteMillis
			podWaste.TotalMemWasteBytes += cw.MemWasteBytes
		}

		totalCPUWaste += podWaste.TotalCPUWasteMillis
		totalMemWaste += podWaste.TotalMemWasteBytes
		pods = append(pods, podWaste)
	}

	return models.NodeReport{
		NodeName:            r.nodeName,
		ReportTime:          time.Now(),
		Pods:                pods,
		TotalCPUWasteMillis: totalCPUWaste,
		TotalMemWasteBytes:  totalMemWaste,
	}
}

// computeContainerWaste calculates waste metrics for a single container.
// Waste = request - P95 usage (if request > P95, the difference is wasted).
func computeContainerWaste(summary models.AggregatedMetrics, meta ContainerMeta, dataWindow time.Duration) models.ContainerWaste {
	// Convert CPU nanocores P95 to millicores for comparison with requests.
	cpuP95Millis := summary.CPUNanoCores.P95 / 1_000_000

	var cpuWasteMillis, cpuWastePercent float64
	if meta.CPURequestMillis > 0 {
		cpuWasteMillis = float64(meta.CPURequestMillis) - cpuP95Millis
		if cpuWasteMillis < 0 {
			cpuWasteMillis = 0
		}
		cpuWastePercent = (cpuWasteMillis / float64(meta.CPURequestMillis)) * 100
	}

	// Memory waste: request - P95 working set usage.
	memP95 := summary.MemoryWorkingSet.P95

	var memWasteBytes, memWastePercent float64
	if meta.MemRequestBytes > 0 {
		memWasteBytes = float64(meta.MemRequestBytes) - memP95
		if memWasteBytes < 0 {
			memWasteBytes = 0
		}
		memWastePercent = (memWasteBytes / float64(meta.MemRequestBytes)) * 100
	}

	// Build CPU usage aggregate in millicores for the report.
	cpuUsageMillis := models.MetricAggregate{
		P50: summary.CPUNanoCores.P50 / 1_000_000,
		P95: cpuP95Millis,
		P99: summary.CPUNanoCores.P99 / 1_000_000,
		Max: summary.CPUNanoCores.Max / 1_000_000,
	}

	return models.ContainerWaste{
		ContainerName:    meta.ContainerName,
		CPURequestMillis: meta.CPURequestMillis,
		CPUUsage:         cpuUsageMillis,
		CPUWasteMillis:   cpuWasteMillis,
		CPUWastePercent:  cpuWastePercent,
		MemoryRequestBytes: meta.MemRequestBytes,
		MemoryUsage: models.MetricAggregate{
			P50: summary.MemoryWorkingSet.P50,
			P95: memP95,
			P99: summary.MemoryWorkingSet.P99,
			Max: summary.MemoryWorkingSet.Max,
		},
		MemWasteBytes:   memWasteBytes,
		MemWastePercent: memWastePercent,
		RestartCount:    meta.RestartCount,
		DataWindow:      dataWindow,
	}
}
